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

// locateInfo 描述目标在样例文件名中的结构位置。
type locateInfo struct {
	sep   string // 目标起始之前最近的分隔符字符
	count int    // 该分隔符在目标之前出现的次数
	after string // 目标结束之后紧跟的分隔符字符
}

// locate 计算目标在文件名中的定位信息。idx、end 为字节索引。
func locate(file string, idx, end int) locateInfo {
	rs := []rune(file)
	ri := utf8.RuneCountInString(file[:idx])
	re := utf8.RuneCountInString(file[:end])

	info := locateInfo{}
	for i := ri - 1; i >= 0; i-- {
		if isSep(rs[i]) {
			info.sep = string(rs[i])
			break
		}
	}
	if info.sep != "" {
		for i := 0; i < ri; i++ {
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

// buildRules 根据样例路径与目标值生成四条候选规则。
func buildRules(pathText, target string) GenerateResult {
	path := strings.TrimSpace(pathText)
	tgt := strings.TrimSpace(target)
	if path == "" {
		return GenerateResult{Error: "请先填入样例路径"}
	}
	if tgt == "" {
		return GenerateResult{Error: "请填入要提取的字符"}
	}

	parts := reSep.Split(path, -1)
	file := parts[len(parts)-1]
	dirRe := ""
	if len(parts) > 1 {
		dirRe = `.*[\\\/]`
	}

	idx := strings.Index(file, tgt)
	if idx < 0 {
		return GenerateResult{Error: "指定字符不在文件名中，请检查是否与路径一致"}
	}
	loc := locate(file, idx, idx+len(tgt))

	template := buildTemplate(tgt, fieldToken)
	strictTemplate := buildTemplate(tgt, strictToken)
	prefix := buildPrefix(loc.sep, loc.count)
	afterRe := ""
	if loc.after != "" {
		afterRe = escapeRegex(loc.after)
	}

	anchorDesc := "（无）"
	if loc.sep != "" {
		anchorDesc = fmt.Sprintf("字段分隔「%s」，跳过 %d 段", loc.sep, loc.count)
	}

	capLoose := `(?P<sn>` + template + `)`
	capStrict := `(?P<sn>` + strictTemplate + `)`

	rules := []Rule{
		{
			Name: "极简版",
			Rec:  true,
			Desc: "按字段结构定位并宽松结尾，适用于绝大多数场景",
			Py:   dirRe + prefix + capLoose + `.*`,
		},
		{
			Name: "边界版",
			Desc: "用目标后紧跟的分隔符锚定右边界，避免吞掉后缀",
			Py:   dirRe + prefix + capLoose + afterRe + `.*`,
		},
		{
			Name: "精确版",
			Desc: "匹配到目标字段即止，不强制匹配后续内容",
			Py:   dirRe + prefix + capLoose,
		},
		{
			Name: "严格版",
			Desc: "字段仅限字母数字，排除中文及特殊字符，结构最严格",
			Py:   dirRe + prefix + capStrict + `.*`,
		},
	}

	return GenerateResult{Rules: rules, Anchor: anchorDesc, File: file}
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

// joinedValues 按 key 排序后拼接所有捕获组值，供展示使用。
func joinedValues(values map[string]string) string {
	if v, ok := values["sn"]; ok {
		return v
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, values[k])
	}
	return strings.Join(parts, "")
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
