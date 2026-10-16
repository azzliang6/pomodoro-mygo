package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The mini window: a small one that stays above the others and shows only
// what matters while working, the time left and the mode, a thin line of
// progress and a button to start or pause.
const (
	miniW   = 264
	miniH   = 112
	miniBar = 30 // the height of its top row, which holds the window buttons
)

// setMini switches between the full window and the mini one; the windows
// are made in main.go, which sets onMini.
func (a *app) setMini(v bool) {
	if a.tm.mini == v {
		return
	}
	a.confirm = nil
	a.tm.setMini(v)
	if a.onMini != nil {
		a.onMini(v)
	}
}

// miniView is the view of the mini window. It draws no more than the full
// one does: while the timer runs it builds the window once a second, when
// the digits change, which is when the line of progress moves a pixel at
// the most; the digits change in place, not rolling.
func (a *app) miniView(c *ui.Context) {
	a.themeOnce(c, &a.miniThemed)
	textScale = clamp32(c.Preferences().TextScale, 1, 1.3)
	now := c.Now()
	tm := a.tm
	tm.tick(now)

	asking := a.settleAsk(c)
	if c.Shortcut(0, ui.KeySpace) {
		tm.toggle(now)
	}
	if c.Shortcut(0, ui.KeyR) {
		tm.reset()
	}
	if c.Shortcut(0, ui.KeyS) {
		a.skipSession()
	}
	if c.Shortcut(0, ui.KeyM) || (!asking && c.Shortcut(0, ui.KeyEscape)) {
		a.setMini(false)
	}
	if tm.running {
		d := tm.left(now) % time.Second
		if d == 0 {
			d = time.Second
		}
		c.After(d + 2*time.Millisecond)
	}

	root := ui.Column(c.Key("mini")).Fill().DragWindow()
	accent := root.AnimateColor("accent", accents[tm.mode], ui.Animation{Duration: 450 * time.Millisecond, Ease: ui.EaseInOut})
	root.Background(colBg.Mix(accent, 0.05))
	root.Children(func() {
		// The top row: the mode with its color, and the window's buttons.
		ui.Row(c.Key("minibar")).Height(miniBar).Shrink(0).AlignItems(ui.Center).Gap(7).Padding(0, 0, 0, 14).Children(func() {
			ui.Box(c.Key("dot")).Size(7, 7).Radius(999).Background(accent)
			ui.Text(c, modes[tm.mode].Name).FontSize(sz(12)).FontWeight(600).LetterSpacing(2).TextColor(accent).SingleLine()
			ui.Box(c.Key("spacer")).Grow(1)
			back := a.captionButton(c, "restore", "还原窗口", "还原窗口（M）", miniBar)
			back.Width(36)
			back.Draw(func(p *ui.Painter, r ui.Rect) { drawRestoreGlyph(p, r) })
			if back.Clicked() {
				a.setMini(false)
			}
			quit := a.captionButton(c, "quit", "关闭", "关闭", miniBar)
			quit.Width(36)
			quit.Draw(func(p *ui.Painter, r ui.Rect) { drawCloseGlyph(p, r) })
			if quit.Clicked() && a.onQuit != nil {
				a.onQuit()
			}
		})

		// The time, and the buttons.
		ui.Row(c.Key("minimain")).Grow(1).AlignItems(ui.Center).Gap(10).Padding(0, 14, 0, 14).Children(func() {
			col := colText
			if !tm.running {
				col = colText.Mix(colMuted, 0.4)
			}
			ui.Text(c, clock(tm.left(now))).FontSize(40).Bold().LetterSpacing(1).TextColor(col).SingleLine().Grow(1)
			skip := a.miniRound(c, "skip", "跳过", "跳过（S）", 34)
			skip.Background(skip.AnimateColor("face", colSurface.Mix(colText, b2f(skip.Hovered())*0.09), ui.Animation{Duration: 120 * time.Millisecond}))
			skip.Draw(func(p *ui.Painter, r ui.Rect) { drawSkipGlyph(p, r, colMuted.Mix(colText, b2f(skip.Hovered()))) })
			if skip.Clicked() {
				a.skipSession()
			}
			label, tip := "开始", "开始（空格）"
			switch {
			case tm.running:
				label, tip = "暂停", "暂停（空格）"
			case tm.remain < tm.total():
				label, tip = "继续", "继续（空格）"
			}
			play := a.miniRound(c, "toggle", label, tip, 44)
			play.Background(play.AnimateColor("face", accent.Mix(colText, b2f(play.Hovered())*0.16), ui.Animation{Duration: 120 * time.Millisecond}))
			running := tm.running
			play.Draw(func(p *ui.Painter, r ui.Rect) { drawPlayGlyph(p, r, running) })
			if play.Clicked() {
				tm.toggle(now)
			}
		})

		// The thin line of progress, as wide as the window leaves room for.
		prog := tm.progress(now)
		bar := ui.Box(c.Key("progress")).Height(3).Shrink(0).Radius(999).Background(colTrack).Margin(0, 14, 14, 14)
		if tm.running {
			prog = bar.Jump("p", prog)
		} else {
			prog = bar.AnimateTo("p", prog, ui.Smooth)
		}
		bar.Draw(func(p *ui.Painter, r ui.Rect) {
			a.software = !p.Accelerated() && !assumeGPU
			if w := r.W * max(0, min(prog, 1)); w > 0 {
				p.Fill(ui.Rect{X: r.X, Y: r.Y, W: max(w, r.H), H: r.H}, accent, r.H/2)
			}
		})
		a.confirmDialog(c, accent, now, true)
		// The window's thin edge, over everything and taking no clicks.
		ui.Box(c.Key("frame")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Border(1, colEdge)
	})
	a.syncTitle(now)
}

// miniRound is a round button of the mini window, not focusable, so that
// the keys keep to the window.
func (a *app) miniRound(c *ui.Context, key, label, tip string, size float32) ui.Element {
	b := ui.Row(c.Key(key)).Size(size, size).Shrink(0).Radius(999).Justify(ui.Center).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Cursor(ui.CursorPointer)
	if b.Pressed() {
		b.Scale(0.95, 0.95)
	}
	return b
}

// drawRestoreGlyph paints the caption glyph of "restore": two squares.
func drawRestoreGlyph(p *ui.Painter, r ui.Rect) {
	s := p.Scale()
	snap := func(v float32) float32 { return (float32(math.Floor(float64(v*s))) + 0.5) / s }
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	w := 1 / s
	p.Stroke(ui.Rect{X: snap(cx - 4.5), Y: snap(cy - 2.5), W: 7, H: 7}, captionInk, 0, w)
	var back ui.Path
	back.MoveTo(snap(cx-2.5), snap(cy-2.5)).LineTo(snap(cx-2.5), snap(cy-4.5)).LineTo(snap(cx+4.5), snap(cy-4.5)).
		LineTo(snap(cx+4.5), snap(cy+2.5)).LineTo(snap(cx+2.5), snap(cy+2.5))
	p.StrokePath(&back, w, captionInk)
}

// drawCloseGlyph paints the caption glyph of "close": a thin cross.
func drawCloseGlyph(p *ui.Painter, r ui.Rect) {
	w := 1 / p.Scale()
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	var x ui.Path
	x.MoveTo(cx-4.5, cy-4.5).LineTo(cx+4.5, cy+4.5).MoveTo(cx+4.5, cy-4.5).LineTo(cx-4.5, cy+4.5)
	p.StrokePath(&x, w, captionInk)
}

// drawPlayGlyph paints a triangle to start, or two bars to pause, in the
// ink of the accent.
func drawPlayGlyph(p *ui.Painter, r ui.Rect, pause bool) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if pause {
		p.Fill(ui.Rect{X: cx - 6, Y: cy - 7, W: 4.5, H: 14}, colInk, 1.2)
		p.Fill(ui.Rect{X: cx + 1.5, Y: cy - 7, W: 4.5, H: 14}, colInk, 1.2)
		return
	}
	var tri ui.Path
	tri.MoveTo(cx-4.5, cy-8).LineTo(cx+7.5, cy).LineTo(cx-4.5, cy+8).Close()
	p.FillPath(&tri, colInk)
}

// drawSkipGlyph paints a triangle with a bar after it.
func drawSkipGlyph(p *ui.Painter, r ui.Rect, col ui.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	var tri ui.Path
	tri.MoveTo(cx-6, cy-5.5).LineTo(cx+3, cy).LineTo(cx-6, cy+5.5).Close()
	p.FillPath(&tri, col)
	p.Fill(ui.Rect{X: cx + 4.5, Y: cy - 5.5, W: 2, H: 11}, col, 1)
}
