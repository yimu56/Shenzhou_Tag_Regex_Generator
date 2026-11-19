package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Version 为程序版本号，可在构建时通过 -ldflags "-X main.Version=x.y.z" 覆盖。
var Version = "1.1.1"

// appTitle 返回带版本号的窗口标题。
func appTitle() string {
	return "标签提取规则生成器 v" + Version
}

// msgBox 用系统消息框显示信息（GUI 版本无控制台）。
func msgBox(text, title string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		0x40) // MB_ICONINFORMATION
}

// walkFatal 用系统对话框显示致命错误。
func walkFatal(err error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(utf16Ptr(err.Error()))),
		uintptr(unsafe.Pointer(utf16Ptr("启动失败"))),
		0x10) // MB_ICONHAND
}

func utf16Ptr(s string) *uint16 {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		u = []uint16{0}
	}
	return &u[0]
}

func mark(ok bool) string {
	if ok {
		return "OK"
	}
	return "NG"
}

func anchorText(anchor string) string {
	if anchor == "" {
		return "（无）"
	}
	return anchor
}

// runCLI 生成结果文本并返回字符串，供写入文件，不依赖控制台。
func runCLI(path, target string, tests []string) string {
	var b strings.Builder
	res := buildRules(path, target)
	if res.Error != "" {
		return res.Error
	}
	fmt.Fprintf(&b, "锚点：%s\r\n", anchorText(res.Anchor))
	fmt.Fprintf(&b, "文件名：%s\r\n\r\n", res.File)
	for i, rule := range res.Rules {
		recommend := ""
		if rule.Rec {
			recommend = "（推荐）"
		}
		fmt.Fprintf(&b, "[%d] %s%s\r\n", i+1, rule.Name, recommend)
		fmt.Fprintf(&b, "    说明：%s\r\n", rule.Desc)
		fmt.Fprintf(&b, "    深舟规则：%s\r\n", rule.Py)
		fmt.Fprintf(&b, "    JS 验证版：%s\r\n", toJsRegex(rule.Py))
		vd := makeVerdict("样例", rule.Py, path)
		fmt.Fprintf(&b, "    样例验证：%s %s\r\n", mark(vd.Ok), vd.Value)
		for j, t := range tests {
			vd = makeVerdict(fmt.Sprintf("测试%d", j+1), rule.Py, t)
			fmt.Fprintf(&b, "    测试%d验证：%s %s\r\n", j+1, mark(vd.Ok), vd.Value)
		}
		b.WriteString("\r\n")
	}
	return b.String()
}

func main() {
	path := flag.String("path", "", "样例路径；填写后进入命令行模式")
	target := flag.String("target", "", "要提取的变量值示例")
	tests := flag.String("test", "", "测试路径，多个以竖线 | 分隔")
	showVer := flag.Bool("version", false, "显示版本号")
	flag.Parse()

	if *showVer {
		msgBox("标签提取规则生成器 "+appTitle(), appTitle())
		return
	}

	if *path != "" || *target != "" {
		var testList []string
		if *tests != "" {
			testList = splitTests(*tests)
		}
		out := runCLI(*path, *target, testList)
		if err := os.WriteFile("规则结果.txt", []byte(out), 0644); err != nil {
			msgBox("结果写入失败："+err.Error(), "错误")
			return
		}
		msgBox("规则已写入 规则结果.txt", appTitle())
		return
	}

	app := newApp()
	if err := app.build(); err != nil {
		walkFatal(err)
		return
	}
	os.Exit(app.Run())
}

func splitTests(s string) []string {
	out := make([]string, 0)
	cur := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '|' {
			out = append(out, string(cur))
			cur = cur[:0]
		} else {
			cur = append(cur, r)
		}
	}
	out = append(out, string(cur))
	return out
}
