package main

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// 候选规则区的滚轮滚动
//
// Windows 把 WM_MOUSEWHEEL 投递给指针下方的窗口。卡片里的「表达式」「匹配结果」是
// EDIT 控件，会先把滚轮消息消化在控件内部（滚动自身文本），外层 ScrollView 收不到，
// 于是光标停在这些控件上就滚不动页面；而页面一旦滚动，这些控件又会被送到指针下方，
// 滚动随之中断。
//
// 这里给滚动容器及其内部的 EDIT 控件挂同一套窗口过程：滚轮消息一律交由滚动容器处理，
// 并按像素缓动，让整块内容连续滑动而不是一格一格地跳。

const (
	scrollTimerID   = 0x5753 // 滚动缓动定时器 ID
	scrollTickMS    = 15     // 缓动帧间隔，约等于系统定时器精度
	scrollLine96DPI = 20     // 单行步长（与 walk 自身口径一致），按 DPI 放大
	scrollEaseDiv   = 2      // 缓动系数：每帧走完剩余距离的 1/Div

	wheelDelta             = 120
	spiGetWheelScrollLines = 0x0068
	wheelPageScroll        = 0xFFFFFFFF
)

// scroller 负责一个 ScrollView 的滚轮滚动与缓动动画。
type scroller struct {
	view    *walk.ScrollView
	content win.HWND // ScrollView 内部承载卡片的子窗口
	linePx  int
	pos     int // 已应用的滚动偏移（原生像素）
	target  int // 缓动目标偏移
	acc     int // 不足一格的滚轮增量，累积后再滚
	active  bool
}

func newScroller(view *walk.ScrollView) *scroller {
	if view == nil {
		return nil
	}

	sc := &scroller{view: view}
	sc.linePx = view.IntFrom96DPI(scrollLine96DPI)
	if sc.linePx <= 0 {
		sc.linePx = scrollLine96DPI
	}
	sc.content = win.GetWindow(view.Handle(), win.GW_CHILD)
	sc.pos, sc.target = sc.offset(), sc.offset()

	hookWindow(view.Handle(), sc, true)

	return sc
}

// watch 让卡片内的控件也走同一套滚动逻辑：它们收到的滚轮消息会转交给滚动容器。
func (sc *scroller) watch(w walk.Window) {
	if sc == nil || w == nil {
		return
	}
	hookWindow(w.Handle(), sc, false)
}

// wheel 处理一次滚轮消息，返回 true 表示已由滚动容器消化。
func (sc *scroller) wheel(hwnd win.HWND, delta int) bool {
	if sc == nil {
		return false
	}
	if px := sc.wheelPixels(delta); px != 0 && sc.scrollBy(px) {
		return true
	}
	// 容器已到顶/到底，滚不动时把消息交还给控件自身，让其在内部滚动。
	return hwnd == sc.view.Handle()
}

// wheelPixels 把滚轮增量换算成像素，遵循系统的「每次滚动几行」设置。
func (sc *scroller) wheelPixels(delta int) int {
	if delta == 0 {
		return 0
	}

	lines := wheelScrollLines()
	if lines == 0 {
		return 0
	}
	if lines < 0 || uint32(lines) >= wheelPageScroll {
		page := sc.pageHeight()
		if page <= 0 {
			return 0
		}
		return -page * delta / wheelDelta
	}

	// 滚轮正向（向远离用户的方向拨动）是向上滚，位移取反。
	// 高精度触控板会送来不足一格的增量，先累积再滚，避免每次都被截断。
	sc.acc -= sc.linePx * lines * delta
	px := sc.acc / wheelDelta
	sc.acc -= px * wheelDelta

	return px
}

// scrollBy 把滚动目标移动 px 像素（正数向下），返回是否产生了位移。
func (sc *scroller) scrollBy(px int) bool {
	max := sc.maxOffset()
	if max <= 0 {
		return false
	}

	// 用户可能刚拖过滚动条或调整过窗口，先把基准对齐到真实位置。
	if !sc.active {
		if cur := sc.offset(); cur != sc.pos {
			sc.pos, sc.target = cur, cur
		}
	}

	next := sc.target + px
	if next > max {
		next = max
	}
	if next < 0 {
		next = 0
	}
	if next == sc.target {
		return false
	}

	sc.target = next
	sc.start()

	return true
}

// start 启动缓动定时器；定时器不可用时直接跳到目标位置。
func (sc *scroller) start() {
	if sc.active {
		return
	}
	if win.SetTimer(sc.view.Handle(), scrollTimerID, scrollTickMS, 0) == 0 {
		sc.pos = sc.target
		sc.apply()
		return
	}
	sc.active = true
}

func (sc *scroller) stop() {
	if sc == nil || !sc.active {
		return
	}
	sc.active = false
	win.KillTimer(sc.view.Handle(), scrollTimerID)
}

// sync 让内部记录的位置与滚动条实际位置保持一致。
func (sc *scroller) sync() {
	if sc == nil {
		return
	}
	cur := sc.offset()
	sc.pos, sc.target = cur, cur
}

func (sc *scroller) tick() {
	diff := sc.target - sc.pos
	if diff == 0 {
		sc.stop()
		return
	}

	step := diff / scrollEaseDiv
	if diff > -3 && diff < 3 {
		// 收尾几步直接走完，避免末尾停在 1~2 像素上多耗几帧。
		step = diff
	} else if step == 0 {
		if diff > 0 {
			step = 1
		} else {
			step = -1
		}
	}
	sc.pos += step
	sc.apply()

	if sc.pos == sc.target {
		sc.stop()
	}
}

// apply 把滚动位置写到滚动条，并同步移动内容窗口。
func (sc *scroller) apply() {
	// 内容高度可能在滚动过程中变化（例如清空结果后卡片变矮），这里夹紧一次，
	// 避免窗口被推到底部之外留下空白。
	if max := sc.maxOffset(); sc.pos > max {
		sc.pos, sc.target = max, max
	}
	if sc.pos < 0 {
		sc.pos, sc.target = 0, 0
	}

	var si win.SCROLLINFO
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.FMask = win.SIF_POS
	si.NPos = int32(sc.pos)
	win.SetScrollInfo(sc.view.Handle(), win.SB_VERT, &si, true)

	if sc.content != 0 {
		win.SetWindowPos(sc.content, 0, 0, int32(-sc.pos), 0, 0,
			win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	}
}

func (sc *scroller) offset() int {
	var si win.SCROLLINFO
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.FMask = win.SIF_POS
	if !win.GetScrollInfo(sc.view.Handle(), win.SB_VERT, &si) {
		return 0
	}
	return int(si.NPos)
}

func (sc *scroller) pageHeight() int {
	var si win.SCROLLINFO
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.FMask = win.SIF_PAGE
	if !win.GetScrollInfo(sc.view.Handle(), win.SB_VERT, &si) {
		return 0
	}
	return int(si.NPage)
}

func (sc *scroller) maxOffset() int {
	var si win.SCROLLINFO
	si.CbSize = uint32(unsafe.Sizeof(si))
	si.FMask = win.SIF_RANGE | win.SIF_PAGE
	if !win.GetScrollInfo(sc.view.Handle(), win.SB_VERT, &si) {
		return 0
	}
	max := int(si.NMax) + 1 - int(si.NPage)
	if max < 0 {
		max = 0
	}
	return max
}

// wheelScrollLines 读取系统的「每次滚动几行」设置，0 表示禁用滚轮，
// 返回负数或 wheelPageScroll 表示一次滚一屏。
func wheelScrollLines() int {
	var v uint32
	if win.SystemParametersInfo(spiGetWheelScrollLines, 0, unsafe.Pointer(&v), 0) {
		return int(v)
	}
	return 3
}

// hookedWindow 记录被接管的窗口的原始窗口过程。
type hookedWindow struct {
	oldProc uintptr
	sc      *scroller
	isView  bool
}

var (
	hookMu    sync.Mutex
	hookTable = make(map[win.HWND]*hookedWindow)
	hookEntry = syscall.NewCallback(hookDispatch)
)

// hookWindow 接管 hwnd 的窗口过程，把滚轮消息交给 sc 处理。
func hookWindow(hwnd win.HWND, sc *scroller, isView bool) {
	if hwnd == 0 || sc == nil {
		return
	}

	hookMu.Lock()
	defer hookMu.Unlock()

	if _, ok := hookTable[hwnd]; ok {
		return
	}
	old := win.SetWindowLongPtr(hwnd, win.GWLP_WNDPROC, hookEntry)
	if old == 0 {
		return
	}
	hookTable[hwnd] = &hookedWindow{oldProc: old, sc: sc, isView: isView}
}

func hookDispatch(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	hookMu.Lock()
	h := hookTable[hwnd]
	hookMu.Unlock()

	if h == nil {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case win.WM_MOUSEWHEEL:
		if h.sc.wheel(hwnd, int(int16(win.HIWORD(uint32(wParam))))) {
			return 0
		}

	case win.WM_TIMER:
		if wParam == scrollTimerID {
			h.sc.tick()
			return 0
		}

	case win.WM_VSCROLL:
		if h.isView {
			// 用户操作滚动条时终止缓动，避免动画与手动拖动互相覆盖。
			h.sc.stop()
			ret := win.CallWindowProc(h.oldProc, hwnd, msg, wParam, lParam)
			h.sc.sync()
			return ret
		}

	case win.WM_NCDESTROY:
		hookMu.Lock()
		delete(hookTable, hwnd)
		hookMu.Unlock()
	}

	return win.CallWindowProc(h.oldProc, hwnd, msg, wParam, lParam)
}
