package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Version 为程序版本号，可在构建时通过 -ldflags "-X main.Version=x.y.z" 覆盖。
var Version = "1.0.0"

func init() {
	// Go 输出 UTF-8，Windows 控制台默认 GBK，切到 65001 避免中文乱码。
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	kernel32.NewProc("SetConsoleOutputCP").Call(65001)
}

// appTitle 返回带版本号的窗口标题。
func appTitle() string {
	return "标签提取规则生成器 v" + Version
}

// hideConsole 在 GUI 模式下隐藏控制台窗口（双击启动时只一闪）。
func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	hwnd, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
	if hwnd != 0 {
		user32 := syscall.NewLazyDLL("user32.dll")
		user32.NewProc("ShowWindow").Call(hwnd, 0) // SW_HIDE
	}
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

func runCLI(path, target string, tests []string) {
	res := buildRules(path, target)
	if res.Error != "" {
		fmt.Println(res.Error)
		return
	}
	fmt.Printf("锚点：%s\n", anchorText(res.Anchor))
	fmt.Printf("文件名：%s\n\n", res.File)
	for i, rule := range res.Rules {
		recommend := ""
		if rule.Rec {
			recommend = "（推荐）"
		}
		fmt.Printf("[%d] %s%s\n", i+1, rule.Name, recommend)
		fmt.Printf("    说明：%s\n", rule.Desc)
		fmt.Printf("    深舟规则：%s\n", rule.Py)
		fmt.Printf("    JS 验证版：%s\n", toJsRegex(rule.Py))
		vd := makeVerdict("样例", rule.Py, path)
		fmt.Printf("    样例验证：%s %s\n", mark(vd.Ok), vd.Value)
		for j, t := range tests {
			vd = makeVerdict(fmt.Sprintf("测试%d", j+1), rule.Py, t)
			fmt.Printf("    测试%d验证：%s %s\n", j+1, mark(vd.Ok), vd.Value)
		}
		fmt.Println()
	}
}

func main() {
	path := flag.String("path", "", "样例路径；填写后进入命令行模式")
	target := flag.String("target", "", "要提取的变量值示例")
	tests := flag.String("test", "", "测试路径，多个以竖线 | 分隔")
	showVer := flag.Bool("version", false, "显示版本号")
	flag.Parse()

	if *showVer {
		fmt.Println("标签提取规则生成器 v" + Version)
		return
	}

	if *path != "" || *target != "" {
		var testList []string
		if *tests != "" {
			for _, t := range splitTests(*tests) {
				testList = append(testList, t)
			}
		}
		runCLI(*path, *target, testList)
		return
	}

	// GUI 模式：隐藏随双击弹出的控制台。
	hideConsole()

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

// walkFatal 用系统对话框显示致命错误（GUI 模式下控制台已隐藏）。
func walkFatal(err error) {
	title := utf16Ptr("启动失败")
	msg := utf16Ptr(err.Error())
	user32 := syscall.NewLazyDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(msg)),
		uintptr(unsafe.Pointer(title)),
		0x10) // MB_ICONHAND
}

func utf16Ptr(s string) *uint16 {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		u = []uint16{0}
	}
	return &u[0]
}
