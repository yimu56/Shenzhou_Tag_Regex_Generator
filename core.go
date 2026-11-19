package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
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

var (
	rePureDigits  = regexp.MustCompile(`^[0-9]{4,}$`)
	reUUID        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reDate        = regexp.MustCompile(`^\d{4}[-_]\d{2}[-_]\d{2}$`)
	reLongDigits  = regexp.MustCompile(`^\d{6,}$`)
	reAlphaDigits = regexp.MustCompile(`^[A-Za-z]+\d{4,}$`)

	// 与浏览器版本一致的变量识别顺序：UUID、日期、数字。
	reNumberish = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|\d{4}[-_]\d{2}[-_]\d{2}|\d+`)

	reDigits2 = regexp.MustCompile(`\d{2,}`)
	reSep     = regexp.MustCompile(`[\\/]`)

	// UUID 前缀片段：如 7C4E9A21-5B6D-4E8F-9A0C-。目标值从 UUID 尾段
	// 开始时，这一段必须以尾部连字符为锚点整体通配，否则会写死其中的字母。
	reUUIDPrefix = regexp.MustCompile(`^[0-9a-fA-F]{2,}(-[0-9a-fA-F]{2,})+-$`)
)

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

// looksVariable 判断一段文本是否整体呈现为变量特征。
func looksVariable(seg string) bool {
	if rePureDigits.MatchString(seg) {
		return true
	}
	if reUUID.MatchString(seg) {
		return true
	}
	if reDate.MatchString(seg) {
		return true
	}
	if reLongDigits.MatchString(seg) {
		return true
	}
	if reAlphaDigits.MatchString(seg) {
		return true
	}
	return false
}

// textToPattern 把固定文本转换成正则片段；按 + 分段，变量段通配。
func textToPattern(s string) string {
	if s == "" {
		return ""
	}
	segs := strings.Split(s, "+")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg == "" {
			out = append(out, "")
			continue
		}
		if looksVariable(seg) {
			out = append(out, "[^+]*?")
			continue
		}
		// UUID 前缀片段整体通配，保留尾部连字符作为定位锚点。
		// 用贪婪的 [^+]* 确保匹配到最后一个连字符，使捕获组从尾段开始。
		if reUUIDPrefix.MatchString(seg) {
			out = append(out, `[^+]*-`)
			continue
		}
		locs := reNumberish.FindAllStringIndex(seg, -1)
		var b strings.Builder
		last := 0
		for _, loc := range locs {
			b.WriteString(escapeRegex(seg[last:loc[0]]))
			b.WriteString("[^+]*?")
			last = loc[1]
		}
		b.WriteString(escapeRegex(seg[last:]))
		out = append(out, b.String())
	}
	return strings.Join(out, `\+`)
}

// shortAnchor 从目标之后的文本中，提取紧邻数字串之前的短字面量作为锚点。
func shortAnchor(after string) string {
	if after == "" {
		return ""
	}
	loc := reDigits2.FindStringIndex(after)
	if loc == nil || loc[0] <= 1 {
		return ""
	}
	anchor := after[:loc[0]]
	if len(anchor) > 12 {
		return ""
	}
	return anchor
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
	prefixDirs := parts[:len(parts)-1]
	idx := strings.Index(file, tgt)
	if idx < 0 {
		return GenerateResult{Error: "指定字符不在文件名中，请检查是否与路径一致"}
	}

	before := file[:idx]
	after := file[idx+len(tgt):]
	tSegs := strings.Split(tgt, "+")

	dirRe := ""
	if len(prefixDirs) > 0 {
		dirRe = `.*[\\\/]`
	}

	beforeRe := textToPattern(before)
	afterRe := textToPattern(after)
	anchor := shortAnchor(after)
	anchorRe := ""
	if anchor != "" {
		anchorRe = escapeRegex(anchor)
	}

	targetGreedy := joinFilled(len(tSegs), "[^+]+")
	lazyParts := make([]string, len(tSegs))
	for i := range lazyParts {
		if i < len(tSegs)-1 {
			lazyParts[i] = "[^+]+"
		} else {
			lazyParts[i] = "[^+]+?"
		}
	}
	targetLazyTail := strings.Join(lazyParts, `\+`)

	beforeCount := 0
	for _, s := range strings.Split(before, "+") {
		if s != "" {
			beforeCount++
		}
	}

	anchorDesc := "（无）"
	if anchor != "" {
		anchorDesc = "「" + anchor + "」"
	}

	rules := make([]Rule, 0, 4)
	rules = append(rules, Rule{
		Name: "极简版",
		Rec:  true,
		Desc: "贪婪目标段 + 短锚点" + anchorDesc + " + 宽松结尾，贴近手写习惯",
		Py:   dirRe + beforeRe + `(?P<sn>` + targetGreedy + `)` + anchorRe + `.*`,
	})

	lazyPy := targetLazyTail
	lazyDesc := "目标末段遇短锚点「" + anchor + "」即停，定位更精确"
	if anchor == "" {
		lazyPy = targetGreedy
		lazyDesc = "无可用锚点，自动退化为贪婪匹配，避免只截到首字符"
	}
	rules = append(rules, Rule{
		Name: "非贪婪版",
		Desc: lazyDesc,
		Py:   dirRe + beforeRe + `(?P<sn>` + lazyPy + `)` + anchorRe + `.*`,
	})
	rules = append(rules, Rule{
		Name: "完整结构版",
		Desc: "后缀结构化通配并锚定结尾，对结构一致性要求高",
		Py:   dirRe + beforeRe + `(?P<sn>` + targetLazyTail + `)` + afterRe + `$`,
	})
	rules = append(rules, Rule{
		Name: "纯段位版",
		Desc: "只按 + 分段数定位目标，不依赖锚点或后缀内容",
		Py:   dirRe + `(?:[^+]+\+){` + strconv.Itoa(beforeCount) + `}(?P<sn>` + targetGreedy + `)(?:\+[^+]+)*$`,
	})

	return GenerateResult{Rules: rules, Anchor: anchor, File: file}
}

// joinFilled 生成 n 段相同文本并用 + 连接。
func joinFilled(n int, unit string) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = unit
	}
	return strings.Join(parts, `\+`)
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
