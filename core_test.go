package main

import (
	"fmt"
	"regexp"
	"testing"
)

// 以下期望值来自浏览器版（JavaScript）在同一结构样例上的实际输出，用于逐项对拍。

func TestBuildRules_ReportedSuffixCase(t *testing.T) {
	path := `X:\Trace\2026-05-04\GluePath.x9600a.enhancement.1\ABC12345W0001KLS+DEF67890GHIJ_v3_1303_0_75908_1_10__20260501022200.png`
	target := "ABC12345W0001KLS+DEF67890GHIJ"
	want := "ABC12345W0001KLS+DEF67890GHIJ"

	res := buildRules(path, target)
	if res.Error != "" {
		t.Fatalf("buildRules error: %s", res.Error)
	}
	if res.Anchor != "_v3_" {
		t.Errorf("anchor = %q, want _v3_", res.Anchor)
	}

	wantPy := []string{
		`.*[\\\/](?P<sn>[^+]+\+[^+]+)_v3_.*`,
		`.*[\\\/](?P<sn>[^+]+\+[^+]+?)_v3_.*`,
		`.*[\\\/](?P<sn>[^+]+\+[^+]+?)_v[^+]*?_[^+]*?_[^+]*?_[^+]*?_[^+]*?_[^+]*?__[^+]*?\.png$`,
		`.*[\\\/](?:[^+]+\+){0}(?P<sn>[^+]+\+[^+]+)(?:\+[^+]+)*$`,
	}
	if len(res.Rules) != len(wantPy) {
		t.Fatalf("rule count = %d, want %d", len(res.Rules), len(wantPy))
	}
	for i, rule := range res.Rules {
		if rule.Py != wantPy[i] {
			t.Errorf("rule[%d] py = %q, want %q", i, rule.Py, wantPy[i])
		}
	}

	// 前三条规则应正确提取，不再吞掉后缀。
	for i := 0; i < 3; i++ {
		values, ok, err := validate(res.Rules[i].Py, path)
		if err != nil || !ok {
			t.Errorf("rule[%d] validate failed: ok=%v err=%v", i, ok, err)
			continue
		}
		if values["sn"] != want {
			t.Errorf("rule[%d] sn = %q, want %q", i, values["sn"], want)
		}
	}

	// 纯段位版在此场景会连后缀一起提取，属于设计内局限。
	values, ok, err := validate(res.Rules[3].Py, path)
	if err != nil || !ok || values["sn"] == want {
		t.Errorf("rule[3] should include suffix, got %q ok=%v err=%v", values["sn"], ok, err)
	}
}

func TestBuildRules_NewSample(t *testing.T) {
	path := `E:\AOI追溯\产线3\2026_11_19\7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`
	target := "LOT20261119008+STATION12"
	want := "LOT20261119008+STATION12"
	testPath := `E:\AOI追溯\产线3\2026_11_20\FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF+LOT20261120999+STATION07_rev2_20261120104533.png`
	wantTest := "LOT20261120999+STATION07"

	res := buildRules(path, target)
	if res.Anchor != "_rev2_" {
		t.Errorf("anchor = %q, want _rev2_", res.Anchor)
	}

	wantPy := []string{
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+)_rev2_.*`,
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+?)_rev2_.*`,
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+?)_rev[^+]*?_[^+]*?\.png$`,
		`.*[\\\/](?:[^+]+\+){1}(?P<sn>[^+]+\+[^+]+)(?:\+[^+]+)*$`,
	}
	for i, rule := range res.Rules {
		if rule.Py != wantPy[i] {
			t.Errorf("rule[%d] py = %q, want %q", i, rule.Py, wantPy[i])
		}
	}

	for i := 0; i < 3; i++ {
		for _, tc := range []struct {
			path string
			want string
		}{{path, want}, {testPath, wantTest}} {
			values, ok, err := validate(res.Rules[i].Py, tc.path)
			if err != nil || !ok || values["sn"] != tc.want {
				t.Errorf("rule[%d] on %q = %q ok=%v err=%v", i, tc.path, values["sn"], ok, err)
			}
		}
	}
}

func TestBuildRules_NoAnchor(t *testing.T) {
	path := `D:\线A\2026_08_04\AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE+XSN0001KLS+YSEC0001+7`
	target := "XSN0001KLS+YSEC0001"
	want := "XSN0001KLS+YSEC0001"

	res := buildRules(path, target)
	if res.Anchor != "" {
		t.Errorf("anchor = %q, want empty", res.Anchor)
	}

	wantPy := []string{
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+).*`,
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+).*`,
		`.*[\\\/][^+]*?\+(?P<sn>[^+]+\+[^+]+?)\+[^+]*?$`,
		`.*[\\\/](?:[^+]+\+){1}(?P<sn>[^+]+\+[^+]+)(?:\+[^+]+)*$`,
	}
	for i, rule := range res.Rules {
		if rule.Py != wantPy[i] {
			t.Errorf("rule[%d] py = %q, want %q", i, rule.Py, wantPy[i])
		}
	}

	// 修复点：无锚点时非贪婪版不得退化为只截首字符。
	for i, rule := range res.Rules {
		values, ok, err := validate(rule.Py, path)
		if err != nil || !ok {
			t.Errorf("rule[%d] validate failed: ok=%v err=%v", i, ok, err)
			continue
		}
		if values["sn"] != want {
			t.Errorf("rule[%d] sn = %q, want %q", i, values["sn"], want)
		}
	}
}

func TestBuildRules_VariantSuffix(t *testing.T) {
	path := `X:\Trace\2026-05-04\GluePath.x9600a.enhancement.1\ABC12345W0001KLS+DEF67890GHIJ_v3_1303_0_75908_1_10__20260501022200.png`
	target := "ABC12345W0001KLS+DEF67890GHIJ"
	variant := `X:\Trace\2099-01-01\GluePath.x9600a.enhancement.9\ZZZZZ99999W9999KLS+YYYYY88888GHIJ_v3_9999_2_88888_3_4__20990101010101.png`
	want := "ZZZZZ99999W9999KLS+YYYYY88888GHIJ"

	res := buildRules(path, target)
	for i := 0; i < 3; i++ {
		values, ok, err := validate(res.Rules[i].Py, variant)
		if err != nil || !ok || values["sn"] != want {
			t.Errorf("rule[%d] variant sn = %q, want %q, ok=%v err=%v", i, values["sn"], want, ok, err)
		}
	}
}

func TestBuildRules_Errors(t *testing.T) {
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

func TestAllGeneratedRulesCompileInGo(t *testing.T) {
	paths := []struct {
		p string
		t string
	}{
		{`X:\Trace\2026-05-04\GluePath.x9600a.enhancement.1\ABC12345W0001KLS+DEF67890GHIJ_v3_1303_0_75908_1_10__20260501022200.png`, "ABC12345W0001KLS+DEF67890GHIJ"},
		{`E:\AOI追溯\产线3\2026_11_19\7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`, "LOT20261119008+STATION12"},
		{`D:\线A\2026_08_04\AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE+XSN0001KLS+YSEC0001+7`, "XSN0001KLS+YSEC0001"},
	}
	for _, tc := range paths {
		res := buildRules(tc.p, tc.t)
		if res.Error != "" {
			t.Fatalf("buildRules error: %s", res.Error)
		}
		for i, rule := range res.Rules {
			if _, err := regexp.Compile(rule.Py); err != nil {
				t.Errorf("rule[%d] cannot compile in Go: %q: %v", i, rule.Py, err)
			}
			if _, err := regexp.Compile(toJsRegex(rule.Py)); err != nil {
				t.Errorf("rule[%d] JS version cannot compile: %q: %v", i, toJsRegex(rule.Py), err)
			}
		}
	}
}

func TestShortAnchor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"_v3_1303.png", "_v3_"},
		{"_rev2_20261119.png", "_rev2_"},
		{"+7", ""},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ123456.png", ""}, // 过长，放弃
		{"v3_1303.png", "v3_"},                          // 数字出现在索引 1 之后，仍可作为锚点
	}
	for _, tc := range cases {
		if got := shortAnchor(tc.in); got != tc.want {
			t.Errorf("shortAnchor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTextToPattern(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE+", `[^+]*?\+`},
		{"2026_11_19", `[^+]*?`}, // 整体呈现日期特征，按变量处理
		{"ABC", `ABC`},
	}
	for _, tc := range cases {
		if got := textToPattern(tc.in); got != tc.want {
			t.Errorf("textToPattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMakeVerdicts(t *testing.T) {
	path := `E:\AOI追溯\产线3\2026_11_19\7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`
	tests := []string{`E:\AOI追溯\产线3\2026_11_20\FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF+LOT20261120999+STATION07_rev2_20261120104533.png`}
	res := buildRules(path, "LOT20261119008+STATION12")
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if len(res.Rules) != 4 {
		t.Fatalf("rule count = %d", len(res.Rules))
	}
	for i, rule := range res.Rules {
		sample := makeVerdict("样例", rule.Py, path)
		if !sample.Ok {
			t.Errorf("rule %s sample not matched", rule.Name)
		}
		// 纯段位版在该场景会连后缀一起提取，属于设计内局限。
		if i < 3 && sample.Value != "LOT20261119008+STATION12" {
			t.Errorf("rule %s sample verdict = %q", rule.Name, sample.Value)
		}
		for j, tp := range tests {
			v := makeVerdict(fmt.Sprintf("测试%d", j+1), rule.Py, tp)
			if !v.Ok {
				t.Errorf("rule %s test%d not matched", rule.Name, j+1)
			}
		}
		if toJsRegex(rule.Py) == rule.Py {
			t.Errorf("rule %s js version not converted", rule.Name)
		}
	}
}

// 目标值从 UUID 尾段开始时，UUID 前缀片段必须整体通配，
// 否则前缀里的字母会被写死，测试路径的 UUID 不同便无法匹配。
func TestUUIDPrefixTarget(t *testing.T) {
	path := `E:/AOI追溯/产线3/2026_11_19/7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`
	target := "2D3E4F5A6B7C+LOT20261119008"
	testPath := `E:/AOI追溯/产线3/2026_11_20/FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF+LOT20261120999+STATION07_rev2_20261120104533.png`
	want := "2D3E4F5A6B7C+LOT20261119008"
	wantTest := "FFFFFFFFFFFF+LOT20261120999"

	res := buildRules(path, target)
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if len(res.Rules) != 4 {
		t.Fatalf("rule count = %d", len(res.Rules))
	}

	// 前三条规则应能在两条路径上分别提取到对应位置的值。
	for i := 0; i < 3; i++ {
		values, ok, err := validate(res.Rules[i].Py, path)
		if err != nil || !ok || values["sn"] != want {
			t.Errorf("rule[%d] sample sn = %q, want %q, ok=%v err=%v", i, values["sn"], want, ok, err)
		}
		values, ok, err = validate(res.Rules[i].Py, testPath)
		if err != nil || !ok || values["sn"] != wantTest {
			t.Errorf("rule[%d] test sn = %q, want %q, ok=%v err=%v", i, values["sn"], wantTest, ok, err)
		}
	}
}

func TestTextToPatternUUIDPrefix(t *testing.T) {
	in := "7C4E9A21-5B6D-4E8F-9A0C-"
	want := `[^+]*-`
	if got := textToPattern(in); got != want {
		t.Errorf("textToPattern(%q) = %q, want %q", in, got, want)
	}
}
