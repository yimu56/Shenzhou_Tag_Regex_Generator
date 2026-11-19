package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Rule 表示一条候选正则规则。
type Rule struct {
	Name string
	Rec  bool
	Desc string
	Py   string
}

// GenerateResult 表示一次生成的完整结果。
type GenerateResult struct {
	Rules  []Rule
	Anchor string
	File   string
	Error  string
}

const (
	// 字段 = 连续的非标点、非空白字符（字母数字、中文等）。
	// 采用 POSIX 字符类：任何标点或空白都能成为字段边界，跨业务结构通用，
	// 不再依赖 UUID、"_v3_"、"+" 等具体结构。
	fieldAtom  = `[^[:punct:][:space:]]`
	fieldToken = fieldAtom + `*`
	// 严格字段：仅限字母数字，供"严格版"候选使用。
	strictAtom  = `[0-9A-Za-z]`
	strictToken = strictAtom + `*`
)

var reSep = regexp.MustCompile(`[\\/]`)

var escapePairs = strings.NewReplacer(
	`\`, `\\`,
	`.`, `\.`,
	`*`, `\*`,
	`+`, `\+`,
	`?`, `\?`,
	`^`, `\^`,
	`$`, `\$`,
	`{`, `\{`,
	`}`, `\}`,
	`(`, `\(`,
	`)`, `\)`,
	`|`, `\|`,
	`[`, `\[`,
	`]`, `\]`,
)

// escapeRegex 转义正则元字符。
func escapeRegex(s string) string {
	return escapePairs.Replace(s)
}

// isSep 判断字符是否为字段分隔符。
// 标点（如 - . _ /）与符号（如 + = | ^）都视为分隔符，
// 因为 Unicode 中 "+" 属于符号而非标点，仅判断 IsPunct 会漏掉它。
func isSep(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r)
}

// buildTemplate 把目标串转成字段结构模板：字段替换为通配类，分隔符原样保留。
// 例如 "7C4E9A21-5B6D" -> "<field>-<field>"；"A+B" -> "<field>\+<field>"。
func buildTemplate(target, field string) string {
	var b strings.Builder
	rs := []rune(target)
	for i := 0; i < len(rs); {
		if isSep(rs[i]) {
			j := i
			for j < len(rs) && isSep(rs[j]) {
				j++
			}
			b.WriteString(escapeRegex(string(rs[i:j])))
			i = j
			continue
		}
		j := i
		for j < len(rs) && !isSep(rs[j]) {
			j++
		}
		b.WriteString(field)
		i = j
	}
	return b.String()
}

// buildTemplateFixed 把目标串转成"结构定长"模板：每个字段替换为带长度约束的
// 通配类，分隔符原样保留。字段长度按 rune 计数，中文同样适用。
// 例如 "7C4E9A21-5B6D" -> "<field>{8}-<field>{4}"；"20261119" -> "<field>{8}"。
// 相比无长度的 fieldToken，定长模板只取与目标等宽的内容，不会吞掉相邻字段。
func buildTemplateFixed(target, atom string) string {
	var b strings.Builder
	rs := []rune(target)
	for i := 0; i < len(rs); {
		if isSep(rs[i]) {
			j := i
			for j < len(rs) && isSep(rs[j]) {
				j++
			}
			b.WriteString(escapeRegex(string(rs[i:j])))
			i = j
			continue
		}
		j := i
		for j < len(rs) && !isSep(rs[j]) {
			j++
		}
		fmt.Fprintf(&b, `%s{%d}`, atom, j-i)
		i = j
	}
	return b.String()
}

// locateInfo 描述目标在样例文件名中的结构位置。
type locateInfo struct {
	sep   string // 目标起始之前最近的分隔符字符
	count int    // 该分隔符在目标之前出现的次数
	after string // 目标结束之后紧跟的分隔符字符
}

// locate 计算目标在文件名中的定位信息，等价于从文件名开头起算的 locateFrom。
func locate(file string, idx, end int) locateInfo {
	return locateFrom(file, idx, end, 0)
}

// locateFrom 计算定位信息，但分隔符只在 [from, idx) 范围内统计。
// 多关键词合并时，每个关键词的定位前缀只需覆盖它与前一个关键词之间的部分，
// 不能从文件名开头重复计算，否则前缀会跨过已匹配的关键词导致失配。
func locateFrom(file string, idx, end, from int) locateInfo {
	rs := []rune(file)
	rf := utf8.RuneCountInString(file[:from])
	ri := utf8.RuneCountInString(file[:idx])
	re := utf8.RuneCountInString(file[:end])

	info := locateInfo{}
	for i := ri - 1; i >= rf; i-- {
		if isSep(rs[i]) {
			info.sep = string(rs[i])
			break
		}
	}
	if info.sep != "" {
		for i := rf; i < ri; i++ {
			if string(rs[i]) == info.sep {
				info.count++
			}
		}
	}
	if re < len(rs) && isSep(rs[re]) {
		info.after = string(rs[re])
	}
	return info
}

// buildPrefix 生成"跳过 N 个定位分隔符"的前缀，其间内容宽松匹配。
func buildPrefix(sep string, count int) string {
	if sep == "" || count <= 0 {
		return ""
	}
	return fmt.Sprintf(`(?:.*?%s){%d}`, escapeRegex(sep), count)
}

// ruleParts 保存一条规则所需的全部结构片段，供单目标与多目标模式复用。
type ruleParts struct {
	dirRe     string // 目录前缀
	file      string // 样例文件名
	anchor    string // 锚点描述
	prefix    string // 跳过定位分隔符的前缀
	capLoose  string // 宽松字段捕获
	capStrict string // 严格字段捕获
	capFixed  string // 结构定长捕获
	afterRe   string // 目标后紧跟的分隔符
	start     int    // 目标在 file 中的字节起始
	end       int    // 目标在 file 中的字节结束
}

// ruleVariant 描述一种规则变体的命名组与后缀。
type ruleVariant struct {
	name   string
	desc   string
	rec    bool
	cap    func(ruleParts) string
	suffix func(ruleParts) string
}

// ruleVariants 为两种模式共用的五种规则变体。前四种保持原有行为，
// 第五种「定长版」按目标字段长度约束提取宽度，解决相邻字段被吞并的问题。
var ruleVariants = []ruleVariant{
	{
		name: "极简版", rec: true,
		desc:   "按字段结构定位并宽松结尾，适用于绝大多数场景",
		cap:    func(p ruleParts) string { return p.capLoose },
		suffix: func(p ruleParts) string { return `.*` },
	},
	{
		name: "边界版",
		desc:   "用目标后紧跟的分隔符锚定右边界，避免吞掉后缀",
		cap:    func(p ruleParts) string { return p.capLoose },
		suffix: func(p ruleParts) string { return p.afterRe + `.*` },
	},
	{
		name: "精确版",
		desc:   "匹配到目标字段即止，不强制匹配后续内容",
		cap:    func(p ruleParts) string { return p.capLoose },
		suffix: func(p ruleParts) string { return `` },
	},
	{
		name: "严格版",
		desc:   "字段仅限字母数字，排除中文及特殊字符，结构最严格",
		cap:    func(p ruleParts) string { return p.capStrict },
		suffix: func(p ruleParts) string { return `.*` },
	},
	{
		name: "定长版",
		desc:   "每个字段长度与目标一致，只取等宽内容，不会吞掉相邻字段",
		cap:    func(p ruleParts) string { return p.capFixed },
		suffix: func(p ruleParts) string { return `.*` },
	},
}

// buildRuleParts 计算单条目标的结构片段。from 为相对 file 的查找起点，
// 单目标模式传 0；多目标模式传前一个目标的结束位置。
func buildRuleParts(pathText, target string, from int) (ruleParts, string) {
	path := strings.TrimSpace(pathText)
	tgt := strings.TrimSpace(target)
	if path == "" {
		return ruleParts{}, "请先填入样例路径"
	}
	if tgt == "" {
		return ruleParts{}, "请填入要提取的字符"
	}

	parts := reSep.Split(path, -1)
	file := parts[len(parts)-1]
	dirRe := ""
	if len(parts) > 1 {
		dirRe = `.*[\\\/]`
	}

	// from 是相对样例文件名 file 的字节偏移；单目标模式传 0，
	// 多目标模式传前一个目标在 file 中的结束位置。
	if from < 0 {
		from = 0
	}
	if from > len(file) {
		from = len(file)
	}

	rel := strings.Index(file[from:], tgt)
	if rel < 0 {
		return ruleParts{}, "指定字符不在文件名中，请检查是否与路径一致"
	}
	idx := from + rel
	end := idx + len(tgt)
	loc := locateFrom(file, idx, end, from)

	prefix := buildPrefix(loc.sep, loc.count)
	afterRe := ""
	if loc.after != "" {
		afterRe = escapeRegex(loc.after)
	}
	anchorDesc := "（无）"
	if loc.sep != "" {
		anchorDesc = fmt.Sprintf("字段分隔「%s」，跳过 %d 段", loc.sep, loc.count)
	}

	return ruleParts{
		dirRe:     dirRe,
		file:      file,
		anchor:    anchorDesc,
		prefix:    prefix,
		capLoose:  `(?P<sn>` + buildTemplate(tgt, fieldToken) + `)`,
		capStrict: `(?P<sn>` + buildTemplate(tgt, strictToken) + `)`,
		capFixed:  `(?P<sn>` + buildTemplateFixed(tgt, fieldAtom) + `)`,
		afterRe:   afterRe,
		start:     idx,
		end:       end,
	}, ""
}

// buildRules 根据样例路径与目标值生成五条候选规则（含定长版）。
func buildRules(pathText, target string) GenerateResult {
	p, errMsg := buildRuleParts(pathText, target, 0)
	if errMsg != "" {
		return GenerateResult{Error: errMsg}
	}

	rules := make([]Rule, 0, len(ruleVariants))
	for _, v := range ruleVariants {
		rules = append(rules, Rule{
			Name: v.name,
			Rec:  v.rec,
			Desc: v.desc,
			Py:   p.dirRe + p.prefix + v.cap(p) + v.suffix(p),
		})
	}
	return GenerateResult{Rules: rules, Anchor: p.anchor, File: p.file}
}

// buildAdvancedRules 按关键词在文件名中的出现顺序，把多个关键词合并成
// 一条含 sn1..snN 命名组的规则。每个关键词的定位只覆盖它与前一个
// 关键词之间的部分，避免前缀跨过已匹配的关键词。
func buildAdvancedRules(pathText string, targets []string) GenerateResult {
	clean := make([]string, 0, len(targets))
	for _, t := range targets {
		if s := strings.TrimSpace(t); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return GenerateResult{Error: "高级模式请至少填入一个关键词"}
	}

	path := strings.TrimSpace(pathText)
	partsList := make([]ruleParts, 0, len(clean))
	from := 0
	for _, t := range clean {
		p, errMsg := buildRuleParts(path, t, from)
		if errMsg != "" {
			return GenerateResult{Error: errMsg}
		}
		partsList = append(partsList, p)
		from += p.end
	}

	rules := make([]Rule, 0, len(ruleVariants))
	for _, v := range ruleVariants {
		var b strings.Builder
		b.WriteString(partsList[0].dirRe)
		for j, p := range partsList {
			if j > 0 {
				b.WriteString(`.*?`)
			}
			b.WriteString(p.prefix)
			cap := strings.Replace(v.cap(p), `(?P<sn>`,
				fmt.Sprintf(`(?P<sn%d>`, j+1), 1)
			b.WriteString(cap)
		}
		b.WriteString(v.suffix(partsList[len(partsList)-1]))

		name := v.name + "·多关键词"
		desc := v.desc + "；按输入顺序同时提取 " + fmt.Sprint(len(clean)) + " 个关键词"
		rules = append(rules, Rule{Name: name, Rec: v.rec, Desc: desc, Py: b.String()})
	}

	anchor := "按输入顺序定位 " + fmt.Sprint(len(clean)) + " 个关键词"
	return GenerateResult{Rules: rules, Anchor: anchor, File: partsList[0].file}
}

// toJsRegex 把 Python 风格命名组转换成 JavaScript 风格命名组。
func toJsRegex(py string) string {
	return strings.ReplaceAll(py, "(?P<", "(?<")
}

// validate 使用 Go 正则引擎验证规则，并返回命名捕获组的值。
func validate(py, pathText string) (map[string]string, bool, error) {
	if py == "" {
		return nil, false, nil
	}
	re, err := regexp.Compile(py)
	if err != nil {
		return nil, false, err
	}
	m := re.FindStringSubmatch(strings.TrimSpace(pathText))
	if m == nil {
		return nil, false, nil
	}
	values := make(map[string]string)
	for i, name := range re.SubexpNames() {
		if name != "" && i < len(m) {
			values[name] = m[i]
		}
	}
	return values, true, nil
}

// joinedValues 把捕获组值拼成展示文本。单组（sn）直接返回值；
// 高级模式的多组（sn1..snN）按组名排序输出 "sn1=值, sn2=值"，便于逐项核对。
func joinedValues(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	if v, ok := values["sn"]; ok && len(values) == 1 {
		return v
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values[k])
	}
	return strings.Join(parts, ", ")
}

// verdict 表示一条规则对某条路径的验证结果。
type verdict struct {
	Label string
	Ok    bool
	Value string
}

// formatVerdict 把一条验证结果格式化为单行展示文本。
func formatVerdict(v verdict) string {
	if !v.Ok {
		return v.Label + "：✗ " + v.Value
	}
	return v.Label + "：✓ " + v.Value
}

// buildVerifyText 构造单条规则的验证文本：先样例，再按输入顺序逐条列出测试路径。
// 每条结果独占一行，返回的行数恒为 1 + len(tests)。
func buildVerifyText(py, samplePath string, tests []string) string {
	var b strings.Builder
	b.WriteString(formatVerdict(makeVerdict("样例", py, samplePath)))
	for j, t := range tests {
		b.WriteString("\r\n")
		b.WriteString(formatVerdict(makeVerdict(fmt.Sprintf("测试%d", j+1), py, t)))
	}
	return b.String()
}

// makeVerdict 验证一条规则在指定路径上的提取结果。
func makeVerdict(label, py, path string) verdict {
	values, ok, err := validate(py, path)
	if err != nil {
		return verdict{Label: label, Ok: false, Value: "规则语法错误"}
	}
	if !ok {
		return verdict{Label: label, Ok: false, Value: "未匹配"}
	}
	return verdict{Label: label, Ok: true, Value: joinedValues(values)}
}
