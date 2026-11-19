package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const (
	samplePath    = `E:\AOI追溯\产线3\2026_11_19\7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`
	sampleTest    = `E:\AOI追溯\产线3\2026_11_20\FAFADFFF-FFFF-FFFF-AVCD-FFFFFFFFFFFF+LOT20261120999+STATION07_rev2_20261120104533.png`
	traceSample   = `X:\Trace\2026-05-04\GluePath.x9600a.enhancement.1\ABC12345W0001KLS+DEF67890GHIJ_v3_1303_0_75908_1_10__20260501022200.png`
	traceVariant  = `X:\Trace\2099-01-01\GluePath.x9600a.enhancement.9\ZZZZZ99999W9999KLS+YYYYY88888GHIJ_v3_9999_2_88888_3_4__20990101010101.png`
	noAnchorPath  = `D:\线A\2026_08_04\AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE+XSN0001KLS+YSEC0001+7`
)

// checkRules 对样例路径与测试路径逐条规则断言提取结果。
func checkRules(t *testing.T, path, target, wantSample, testPath, wantTest string) {
	t.Helper()
	res := buildRules(path, target)
	if res.Error != "" {
		t.Fatalf("buildRules error: %s", res.Error)
	}
	if len(res.Rules) != 4 {
		t.Fatalf("rule count = %d, want 4", len(res.Rules))
	}
	for i, rule := range res.Rules {
		vs, ok, err := validate(rule.Py, path)
		if err != nil || !ok {
			t.Errorf("rule[%d](%s) sample not matched: ok=%v err=%v py=%q", i, rule.Name, ok, err, rule.Py)
			continue
		}
		if got := joinedValues(vs); got != wantSample {
			t.Errorf("rule[%d](%s) sample = %q, want %q", i, rule.Name, got, wantSample)
		}
		vt, ok, err := validate(rule.Py, testPath)
		if err != nil || !ok {
			t.Errorf("rule[%d](%s) test not matched: ok=%v err=%v py=%q", i, rule.Name, ok, err, rule.Py)
			continue
		}
		if got := joinedValues(vt); got != wantTest {
			t.Errorf("rule[%d](%s) test = %q, want %q", i, rule.Name, got, wantTest)
		}
	}
}

// 用户最新反馈的三组案例：指定字段的不同位置，测试路径提取对应位置字段。
func TestUserFieldCases(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		wantSample string
		wantTest   string
	}{
		{"首段两字段", "7C4E9A21-5B6D", "7C4E9A21-5B6D", "FAFADFFF-FFFF"},
		{"中间两字段", "5B6D-4E8F", "5B6D-4E8F", "FFFF-FFFF"},
		{"末段两字段", "9A0C-2D3E4F5A6B7C", "9A0C-2D3E4F5A6B7C", "AVCD-FFFFFFFFFFFF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkRules(t, samplePath, tc.target, tc.wantSample, sampleTest, tc.wantTest)
		})
	}
}

// 目标为 UUID 尾段 + 后续字段，验证不再写死前缀字母。
func TestUUIDTailSegment(t *testing.T) {
	target := "2D3E4F5A6B7C+LOT20261119008"
	checkRules(t, samplePath, target, target, sampleTest, "FFFFFFFFFFFF+LOT20261120999")
}

// 目标为加号分隔的字段段。
func TestPlusSegment(t *testing.T) {
	target := "LOT20261119008+STATION12"
	checkRules(t, samplePath, target, target, sampleTest, "LOT20261120999+STATION07")
}

// 目标后带后缀（_v3_...），验证不吞后缀、且规则不写死 _v3_。
func TestSuffixNotSwallowed(t *testing.T) {
	target := "ABC12345W0001KLS+DEF67890GHIJ"
	checkRules(t, traceSample, target, target, traceVariant, "ZZZZZ99999W9999KLS+YYYYY88888GHIJ")
	res := buildRules(traceSample, target)
	for _, rule := range res.Rules {
		if stringsContains(rule.Py, "_v3_") {
			t.Errorf("rule %s hardcodes _v3_: %q", rule.Name, rule.Py)
		}
	}
}

// 无短锚点场景，退化为宽松结尾仍应正确。
func TestNoAnchorPath(t *testing.T) {
	target := "XSN0001KLS+YSEC0001"
	checkRules(t, noAnchorPath, target, target, noAnchorPath, target)
}

func TestErrors(t *testing.T) {
	cases := []struct {
		path   string
		target string
	}{
		{"", "ABC"},
		{`D:\a\b\c.png`, ""},
		{`D:\a\b\c.png`, "NOT_EXIST"},
	}
	for _, tc := range cases {
		if res := buildRules(tc.path, tc.target); res.Error == "" {
			t.Errorf("buildRules(%q, %q) expected error", tc.path, tc.target)
		}
	}
}

func TestAllRulesCompile(t *testing.T) {
	paths := []struct{ p, t string }{
		{samplePath, "7C4E9A21-5B6D"},
		{samplePath, "LOT20261119008+STATION12"},
		{traceSample, "ABC12345W0001KLS+DEF67890GHIJ"},
		{noAnchorPath, "XSN0001KLS+YSEC0001"},
	}
	for _, tc := range paths {
		res := buildRules(tc.p, tc.t)
		if res.Error != "" {
			t.Fatalf("buildRules error: %s", res.Error)
		}
		for i, rule := range res.Rules {
			if _, err := regexp.Compile(rule.Py); err != nil {
				t.Errorf("rule[%d] Go compile failed: %q: %v", i, rule.Py, err)
			}
			if _, err := regexp.Compile(toJsRegex(rule.Py)); err != nil {
				t.Errorf("rule[%d] JS compile failed: %q: %v", i, toJsRegex(rule.Py), err)
			}
		}
	}
}

func TestBuildTemplate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"7C4E9A21-5B6D", "F-F"},
		{"A+B", `F\+F`},
		{"ABC", "F"},
		{"A_B.C", `F_F\.F`},
	}
	for _, tc := range cases {
		if got := buildTemplate(tc.in, "F"); got != tc.want {
			t.Errorf("buildTemplate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildPrefix(t *testing.T) {
	cases := []struct {
		sep   string
		count int
		want  string
	}{
		{"-", 0, ""},
		{"-", 1, `(?:.*?-){1}`},
		{"+", 3, `(?:.*?\+){3}`},
	}
	for _, tc := range cases {
		if got := buildPrefix(tc.sep, tc.count); got != tc.want {
			t.Errorf("buildPrefix(%q,%d) = %q, want %q", tc.sep, tc.count, got, tc.want)
		}
	}
}

func TestLocate(t *testing.T) {
	file := "7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT"
	// 目标 "5B6D-4E8F" 在 file 中的字节位置。
	idx := 9
	end := idx + len("5B6D-4E8F")
	info := locate(file, idx, end)
	if info.sep != "-" || info.count != 1 || info.after != "-" {
		t.Errorf("locate = %+v, want sep=- count=1 after=-", info)
	}
}

func TestMakeVerdicts(t *testing.T) {
	res := buildRules(samplePath, "5B6D-4E8F")
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	for _, rule := range res.Rules {
		sv := makeVerdict("样例", rule.Py, samplePath)
		if !sv.Ok || sv.Value != "5B6D-4E8F" {
			t.Errorf("rule %s sample verdict = %+v", rule.Name, sv)
		}
		tv := makeVerdict("测试", rule.Py, sampleTest)
		if !tv.Ok || tv.Value != "FFFF-FFFF" {
			t.Errorf("rule %s test verdict = %+v", rule.Name, tv)
		}
		if toJsRegex(rule.Py) == rule.Py {
			t.Errorf("rule %s js version not converted", rule.Name)
		}
	}
}

// 多条测试路径时，验证文本必须逐条列出全部结果，不能遗漏。
func TestBuildVerifyTextMultipleTests(t *testing.T) {
	res := buildRules(samplePath, "5B6D-4E8F")
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	for _, rule := range res.Rules {
		tests := []string{sampleTest, traceSample, noAnchorPath, traceVariant}
		text := buildVerifyText(rule.Py, samplePath, tests)
		lines := strings.Split(text, "\r\n")
		if got, want := len(lines), len(tests)+1; got != want {
			t.Errorf("rule %s line count = %d, want %d; text=%q", rule.Name, got, want, text)
		}
		for j := range tests {
			label := fmt.Sprintf("测试%d：", j+1)
			if !strings.Contains(text, label) {
				t.Errorf("rule %s missing %s in text=%q", rule.Name, label, text)
			}
		}
		if !strings.Contains(text, "样例：") {
			t.Errorf("rule %s missing 样例 line in text=%q", rule.Name, text)
		}
	}
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
