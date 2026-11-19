package main

import (
	"fmt"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const (
	winW = 900
	winH = 700
	// 候选规则数：极简 / 边界 / 精确 / 严格 / 定长。
	cardCount = 5
)

type ruleCard struct {
	nameLabel *walk.Label
	descLabel *walk.Label
	ruleEdit  *walk.TextEdit
	verify    *walk.TextEdit
	pyBtn     *walk.PushButton
	jsBtn     *walk.PushButton
	py        string
	js        string
}

type App struct {
	mw          *walk.MainWindow
	pathEdit    *walk.TextEdit
	tgtEdit     *walk.LineEdit
	normalPanel *walk.Composite
	advanceChk  *walk.CheckBox
	multiPanel  *walk.Composite
	multiEdit   *walk.TextEdit
	testEdit    *walk.TextEdit
	cards       []*ruleCard
}

func newApp() *App {
	return &App{}
}

func (a *App) build() error {
	a.cards = make([]*ruleCard, cardCount)

	// 结果区：所有卡片放进滚动容器。
	resultChildren := make([]Widget, 0, cardCount)
	for i := 0; i < cardCount; i++ {
		card := &ruleCard{}
		a.cards[i] = card
		pyHandler := a.makeCopyHandler(i, true)
		jsHandler := a.makeCopyHandler(i, false)
		resultChildren = append(resultChildren,
			Composite{
				Layout:  VBox{Margins: Margins{10, 10, 10, 10}, Spacing: 5},
				MinSize: Size{0, 128},
				Children: []Widget{
					Label{AssignTo: &card.nameLabel, Text: "候选 " + fmt.Sprint(i+1), Font: Font{Bold: true}},
					Label{AssignTo: &card.descLabel, Text: "", TextColor: walk.Color(0x666666)},
					TextEdit{AssignTo: &card.ruleEdit, ReadOnly: true, MinSize: Size{0, 34},
						Font: Font{Family: "Consolas", PointSize: 9}},
					Composite{
						Layout: HBox{MarginsZero: true},
						Children: []Widget{
							PushButton{AssignTo: &card.pyBtn, Text: "复制深舟规则", OnClicked: pyHandler, MaxSize: Size{130, 0}},
							PushButton{AssignTo: &card.jsBtn, Text: "复制 JS 验证版", OnClicked: jsHandler, MaxSize: Size{140, 0}},
							HSpacer{},
						},
					},
					// 验证结果随测试路径条数增长，使用 CompactHeight 让控件按文本内容
					// 自动增高，避免多条测试路径时结果被截断（原固定高度仅约两行）。
					TextEdit{AssignTo: &card.verify, ReadOnly: true, CompactHeight: true},
				},
			},
		)
	}

	err := MainWindow{
		AssignTo: &a.mw,
		Title:    appTitle(),
		MinSize:  Size{760, 560},
		Layout:   VBox{Margins: Margins{16, 16, 16, 16}, Spacing: 10},
		Children: []Widget{
			Label{Text: "输入路径与目标值，生成多条候选规则并选择测试　（v" + Version + "）", TextColor: walk.Color(0x666666)},

			Composite{
				Layout: VBox{MarginsZero: true, Spacing: 4},
				Children: []Widget{
					Label{Text: "样例路径"},
					TextEdit{AssignTo: &a.pathEdit, MinSize: Size{0, 52}},
					Composite{
						AssignTo: &a.normalPanel,
						Layout:   VBox{MarginsZero: true, Spacing: 4},
						Children: []Widget{
							Label{Text: "要提取的变量值（示例）"},
							LineEdit{AssignTo: &a.tgtEdit, MaxLength: 512, MinSize: Size{0, 26}},
						},
					},
					CheckBox{AssignTo: &a.advanceChk,
						Text:      "高级模式：同时提取多个关键词（按路径中出现顺序，每行一个）",
						OnClicked: a.onToggleAdvanced},
					Composite{
						AssignTo: &a.multiPanel,
						Visible:  false,
						Layout:   VBox{MarginsZero: true, Spacing: 4},
						Children: []Widget{
							Label{Text: "多个关键词（每行一个；生成的规则含 sn1、sn2… 多组结果）"},
							TextEdit{AssignTo: &a.multiEdit, MinSize: Size{0, 64}},
						},
					},
					Label{Text: "测试路径（可选，每行一条，用于横向比较规则的泛化能力）"},
					TextEdit{AssignTo: &a.testEdit, MinSize: Size{0, 48}},
				},
			},

			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					PushButton{Text: "生成规则", OnClicked: a.onGenerate, MinSize: Size{110, 0}},
					PushButton{Text: "填入示例", OnClicked: a.onSample, MinSize: Size{100, 0}},
					PushButton{Text: "清空", OnClicked: a.onClear, MinSize: Size{90, 0}},
					HSpacer{},
				},
			},

			Label{Text: "候选规则（每条都把指定值按变量处理，附验证结果）", Font: Font{Bold: true}},
			ScrollView{
				Layout:   VBox{MarginsZero: true, Spacing: 10},
				MinSize:  Size{0, 300},
				Children: resultChildren,
			},
		},
	}.Create()

	if err != nil {
		return err
	}

	a.fitWindow()
	return nil
}

func (a *App) makeCopyHandler(i int, py bool) func() {
	return func() {
		if i < 0 || i >= len(a.cards) {
			return
		}
		card := a.cards[i]
		text := card.py
		if !py {
			text = card.js
		}
		if text == "" {
			a.warn("尚未生成规则")
			return
		}
		if err := walk.Clipboard().SetText(text); err != nil {
			a.warn("复制失败：" + err.Error())
		}
	}
}

// fitWindow 按显示器工作区收敛窗口尺寸，避免高 DPI 下超出屏幕。
func (a *App) fitWindow() {
	dpi := 96
	if d := a.mw.DPI(); d > 0 {
		dpi = d
	}
	screenW := 1920
	screenH := 1080
	w := winW
	h := winH
	if maxW := screenW*96/dpi - 24; w > maxW && maxW > 400 {
		w = maxW
	}
	if maxH := screenH*96/dpi - 60; h > maxH && maxH > 320 {
		h = maxH
	}
	_ = a.mw.SetSize(walk.Size{Width: w, Height: h})
}

func (a *App) warn(msg string) {
	walk.MsgBox(a.mw, "提示", msg, walk.MsgBoxOK|walk.MsgBoxIconWarning)
}

func (a *App) onSample() {
	a.pathEdit.SetText(`E:\AOI追溯\产线3\2026_11_19\7C4E9A21-5B6D-4E8F-9A0C-2D3E4F5A6B7C+LOT20261119008+STATION12_rev2_20261119093015.png`)
	a.tgtEdit.SetText("LOT20261119008+STATION12")
	a.testEdit.SetText(`E:\AOI追溯\产线3\2026_11_20\FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF+LOT20261120999+STATION07_rev2_20261120104533.png`)
	a.onGenerate()
}

func (a *App) onClear() {
	a.pathEdit.SetText("")
	a.tgtEdit.SetText("")
	a.multiEdit.SetText("")
	a.advanceChk.SetChecked(false)
	a.onToggleAdvanced()
	a.testEdit.SetText("")
	for _, c := range a.cards {
		c.nameLabel.SetText("")
		c.descLabel.SetText("")
		c.ruleEdit.SetText("")
		c.verify.SetText("")
		c.py, c.js = "", ""
	}
}

func splitLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		s := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) testPaths() []string {
	return splitLines(a.testEdit.Text())
}

// onToggleAdvanced 在普通模式与多关键词模式之间切换输入区。
func (a *App) onToggleAdvanced() {
	on := a.advanceChk.Checked()
	a.normalPanel.SetVisible(!on)
	a.multiPanel.SetVisible(on)
	a.mw.RequestLayout()
}

func (a *App) onGenerate() {
	var res GenerateResult
	if a.advanceChk.Checked() {
		res = buildAdvancedRules(a.pathEdit.Text(), splitLines(a.multiEdit.Text()))
	} else {
		res = buildRules(a.pathEdit.Text(), a.tgtEdit.Text())
	}
	if res.Error != "" {
		a.warn(res.Error)
		return
	}

	tests := a.testPaths()
	for i, rule := range res.Rules {
		card := a.cards[i]
		title := fmt.Sprintf("%d. %s", i+1, rule.Name)
		if rule.Rec {
			title += "（推荐）"
		}
		card.nameLabel.SetText(title)
		card.descLabel.SetText(rule.Desc)
		card.ruleEdit.SetText(rule.Py)
		card.py = rule.Py
		card.js = toJsRegex(rule.Py)
		card.verify.SetText(buildVerifyText(rule.Py, a.pathEdit.Text(), tests))
	}
}

func (a *App) Run() int {
	return a.mw.Run()
}
