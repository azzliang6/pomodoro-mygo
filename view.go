package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The palette: one dark canvas and one accent per mode.
var (
	colBg      = ui.Hex("#0e1014")
	colBgGlow  = ui.Hex("#171a21")
	colSurface = ui.Hex("#181b22")
	colTrack   = ui.Hex("#232731")
	colText    = ui.Hex("#f4f5f7")
	colMuted   = ui.Hex("#8b91a0")
	colDim     = ui.Hex("#555b69")
	colEdge    = ui.Hex("#2a2f3a") // the thin border of the window
	colRaised  = ui.Hex("#22262f") // buttons on the panel
	colInk     = ui.Hex("#0e1014") // text on an accent

	accents = [3]ui.Color{
		ui.Hex("#ff6b5e"), // 专注: tomato
		ui.Hex("#3fd6a0"), // 短休息: mint
		ui.Hex("#6aa8ff"), // 长休息: sky
	}
)

const (
	ringSize   = 300
	ringRadius = 112
	ringWidth  = 14
	tabWidth   = 96
	tabHeight  = 38
	tabPad     = 4
	barHeight  = 36
)

// textScale is how much larger than usual the system asks for text to be
// (Windows' "text size"), kept within what the layout has room for.
var textScale float32 = 1

// sz scales a font size by textScale.
func sz(n float32) float32 { return n * textScale }

func (a *app) applyTheme(c *ui.Context) { a.themeOnce(c, &a.themed) }

// themeOnce gives the window of the context the app's theme, once: each
// window has its own context and theme.
func (a *app) themeOnce(c *ui.Context, done *bool) {
	if *done {
		return
	}
	*done = true
	t := *c.Theme()
	t.Dark = true
	t.Background, t.Surface = colBg, colSurface
	t.SurfaceHover, t.SurfacePressed = ui.Hex("#20242d"), ui.Hex("#272c37")
	t.Border, t.Text, t.TextMuted = colTrack, colText, colMuted
	t.Accent, t.AccentText = accents[0], colInk
	c.SetTheme(&t)
}

func (a *app) view(c *ui.Context) {
	a.applyTheme(c)
	textScale = clamp32(c.Preferences().TextScale, 1, 1.3)
	now := c.Now()
	tm := a.tm
	tm.tick(now)

	asking := a.settleAsk(c)
	if a.settingsOpen {
		if !asking && c.Shortcut(0, ui.KeyEscape) {
			a.settingsOpen = false
		}
	} else {
		if c.Shortcut(0, ui.KeySpace) {
			tm.toggle(now)
		}
		if c.Shortcut(0, ui.KeyR) {
			tm.reset()
		}
		if c.Shortcut(0, ui.KeyS) {
			a.skipSession()
		}
		if c.Shortcut(0, ui.KeyM) {
			a.setMini(true)
		}
	}
	if tm.running {
		// The arc moves a fraction of a pixel in a second, so the window is
		// built again when the digits change and half a second later, not
		// at every refresh of the display: the cost while it runs is a
		// couple of frames a second, not sixty.
		d := tm.left(now) % time.Second
		if d == 0 {
			d = time.Second
		}
		c.After(min(d+2*time.Millisecond, 500*time.Millisecond))
	}

	// One accent for the whole window, easing to the color of the mode.
	root := ui.Column(c.Key("root")).Fill()
	accent := root.AnimateColor("accent", accents[tm.mode], ui.Animation{Duration: 450 * time.Millisecond, Ease: ui.EaseInOut})
	open := root.AnimateTo("settings", b2f(a.settingsOpen), ui.Smooth)
	if a.software {
		root.Background(colBg.Mix(accent, 0.035)) // a gradient over the whole window costs a CPU-drawn frame
	} else {
		root.Gradient(colBg, colBg.Mix(accent, 0.07), 180)
	}
	root.Children(func() {
		a.titleBar(c, accent)
		// Every part is as wide as the window leaves room for and centers its
		// own content, so that nothing is wider than the window whatever the
		// width of the text.
		ui.Column(c.Key("content")).Grow(1).AlignItems(ui.Stretch).Justify(ui.Center).Gap(22).Padding(0, 16, 12, 16).Children(func() {
			ui.Row(c.Key("tabsrow")).Justify(ui.Center).Children(func() { a.tabs(c, accent) })
			ui.Row(c.Key("ringrow")).Justify(ui.Center).Children(func() { a.ring(c, accent, now) })
			a.controls(c, accent, now)
			a.footer(c, accent)
		})
		if open > 0.002 || a.settingsOpen {
			a.settingsPanel(c, accent, now, open)
		}
		a.confirmDialog(c, accent, now, false)
		if previewCaption != nil {
			previewCaption(c)
		}
		// The window's thin edge, over everything and taking no clicks.
		ui.Box(c.Key("frame")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Border(1, colEdge)
	})
	a.syncTitle(now)
}

func clamp32(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// titleBar is the title bar the app draws itself: the window has none of
// the system's, and MyGo puts the minimize, maximize and close buttons over
// its right end. The bar drags the window.
func (a *app) titleBar(c *ui.Context, accent ui.Color) {
	bar := c.TitleBar()
	if barInset > bar.Right {
		bar.Right = barInset
	}
	h := max(bar.Height, barHeight)
	// The bar's right end is flush with the window's caption buttons, which
	// are as tall as the bar and 46 wide: the settings button is one more
	// of them, with the same flat look.
	ui.Row(c.Key("titlebar")).Height(h).Shrink(0).AlignItems(ui.Center).Gap(8).
		Padding(0, bar.Right, 0, bar.Left+16).DragWindow().Children(func() {
		ui.Box(c.Key("brand")).Size(8, 8).Radius(999).Background(accent)
		ui.Text(c, "番茄钟").FontSize(sz(13)).FontWeight(500).TextColor(colMuted).SingleLine()
		ui.Box(c.Key("spacer")).Grow(1)

		// The title bar's own buttons sit side by side with no gap (the row's
		// Gap is for the brand and its name), so that with the window's three
		// caption buttons the five are one strip at a pitch of 46.
		ui.Row(c.Key("tools")).Height(h).Shrink(0).AlignItems(ui.Center).Gap(0).Children(func() {
			mb := a.captionButton(c, "mini", "迷你窗口", "迷你窗口（M）", h)
			mb.Draw(func(p *ui.Painter, r ui.Rect) { drawMiniGlyph(p, r) })
			if mb.Clicked() {
				a.setMini(true)
			}

			b := a.captionButton(c, "gear", "设置", "设置", h)
			b.Draw(func(p *ui.Painter, r ui.Rect) { drawSliders(p, r) })
			if b.Clicked() {
				a.settingsOpen = !a.settingsOpen
			}
		})
	})
}

// captionButton is a button of the title bar in the look of the window's
// caption buttons: as tall as the bar and 46 wide, with nothing behind its
// glyph but, as the pointer comes, a light overlay of 10%, 20% pressed.
func (a *app) captionButton(c *ui.Context, key, label, tip string, h float32) ui.Element {
	b := ui.Row(c.Key(key)).Width(captionWidth).Height(h).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).
		Label(label).Tooltip(tip).Cursor(ui.CursorDefault)
	var bg ui.Color
	switch {
	case b.Pressed():
		bg = captionInk.Alpha(0.20)
	case b.Hovered():
		bg = captionInk.Alpha(0.10)
	}
	return b.Background(b.AnimateColor("face", bg, ui.Animation{Duration: 90 * time.Millisecond}))
}

// drawMiniGlyph paints the mini window icon in the thin line of the caption
// glyphs: a window with a small one in its corner.
func drawMiniGlyph(p *ui.Painter, r ui.Rect) {
	s := p.Scale()
	snap := func(v float32) float32 { return (float32(math.Floor(float64(v*s))) + 0.5) / s }
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	w := 1 / s
	p.Stroke(ui.Rect{X: snap(cx - 5.5), Y: snap(cy - 4.5), W: 11, H: 9}, captionInk, 0, w)
	p.Fill(ui.Rect{X: snap(cx + 0.5), Y: snap(cy + 0.5), W: 4.5, H: 3.5}, captionInk, 0)
}

// previewCaption, set by a test, draws stand-ins for the caption buttons.
var previewCaption func(c *ui.Context)

// captionInk is the color of the window's caption glyphs in the dark.
var captionInk = ui.Hex("#ffffff")

// captionWidth is the width of a caption button of Windows 11.
const captionWidth = 46

// barInset stands in for the room the caption buttons take, where there are
// none: tests and previews.
var barInset float32

// drawSliders paints the settings icon as the caption glyphs are: lines
// 1 pixel thin in a box of about 11 DIPs, in their color. It has three
// lines, each with a small ring on it.
func drawSliders(p *ui.Painter, r ui.Rect) {
	s := p.Scale()
	// Lines of one device pixel sit on the middle of a pixel to stay crisp.
	snap := func(v float32) float32 { return (float32(math.Floor(float64(v*s))) + 0.5) / s }
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	x0, x1 := snap(cx-5.5), snap(cx+5.5)
	w := 1 / s
	knobs := [3]float32{-1.8, 2.0, -0.6}
	const gap = 2.6
	for i, k := range knobs {
		y := snap(cy + float32(i-1)*4)
		kx := snap(cx + k)
		p.Line(x0, y, kx-gap, y, w, captionInk)
		p.Line(kx+gap, y, x1, y, w, captionInk)
		var ring ui.Path
		ring.Circle(kx, y, 1.7)
		p.StrokePath(&ring, w, captionInk)
	}
}

// syncTitle puts the time left in the window's title, so that it shows in
// the taskbar.
func (a *app) syncTitle(now time.Time) {
	title := fmt.Sprintf("%s · %s", clock(a.tm.left(now)), modes[a.tm.mode].Name)
	if a.setTitle != nil && title != a.title {
		a.title = title
		a.setTitle(title + " — 番茄钟")
	}
}

// clock writes a duration as 24:59, rounding up, so that 00:00 shows only
// when it is over.
func clock(d time.Duration) string {
	s := int((d + time.Second - 1) / time.Second)
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// tabs is the switcher of modes: a pill that glides under the label chosen.
func (a *app) tabs(c *ui.Context, accent ui.Color) {
	tm := a.tm
	track := ui.Row(c.Key("tabs")).Width(3*tabWidth + 2*tabPad).Height(tabHeight + 2*tabPad).
		Radius(999).Background(colSurface).Padding(tabPad)
	x := track.AnimateTo("pill", float32(tm.mode*tabWidth), ui.Snappy)
	track.Children(func() {
		ui.Box(c.Key("pill")).Absolute().Top(tabPad).Left(tabPad+x).Width(tabWidth).Height(tabHeight).
			Radius(999).Background(accent.Alpha(0.16)).Border(1, accent.Alpha(0.5))
		for i, m := range modes {
			b := ui.Row(c.Key(fmt.Sprint("tab", i))).Width(tabWidth).Height(tabHeight).Radius(999).
				Justify(ui.Center).AlignItems(ui.Center).Label(m.Name).Cursor(ui.CursorPointer)
			color := colMuted
			switch {
			case i == tm.mode:
				color = colText
			case b.Hovered():
				color = colText.Mix(colMuted, 0.4)
			}
			color = b.AnimateColor("fg", color, ui.Animation{Duration: 160 * time.Millisecond})
			b.Children(func() {
				ui.Text(c, m.Name).FontSize(sz(14)).FontWeight(600).TextColor(color).SingleLine()
			})
			if b.Clicked() && i != tm.mode {
				a.switchMode(i)
			}
		}
	})
}

// ring is the progress ring with the time left in large type.
func (a *app) ring(c *ui.Context, accent ui.Color, now time.Time) {
	tm := a.tm
	ring := ui.Column(c.Key("ring")).Size(ringSize, ringSize).Justify(ui.Center).AlignItems(ui.Center).Gap(0)

	// The progress eases to its target (a reset or a new mode unwinds the
	// ring), and follows the clock while it runs.
	var prog float32
	if tm.running {
		prog = ring.Jump("progress", tm.progress(now)) // the clock moves it, no easing to redraw
	} else {
		prog = ring.AnimateTo("progress", tm.progress(now), ui.Smooth)
	}
	// A finished session makes the ring swell once.
	swell := ring.Keyframes("swell", []ui.Keyframe{
		{Value: 1},
		{At: 180 * time.Millisecond, Value: 1.06, Ease: ui.EaseOut},
		{At: 700 * time.Millisecond, Value: 1, Ease: ui.EaseInOut},
	}, ui.KeyframeOptions{Run: tm.doneSeq})
	ring.Scale(swell, swell)

	ring.Draw(func(p *ui.Painter, r ui.Rect) {
		a.software = !p.Accelerated() && !assumeGPU
		a.geo.draw(p, r, accent, prog, a.software)
	})
	ring.Children(func() {
		ui.Text(c, modes[tm.mode].Name).FontSize(sz(15)).FontWeight(500).LetterSpacing(4).TextColor(accent)
		// The time opens the settings when the timer waits.
		face := ui.Row(c.Key("time")).Cursor(ui.CursorPointer).Label("时长设置")
		timeColor := colText
		if face.Hovered() && tm.fresh() {
			timeColor = colText.Mix(accent, 0.45)
		}
		timeColor = face.AnimateColor("fg", timeColor, ui.Animation{Duration: 160 * time.Millisecond})
		face.Children(func() {
			ui.Text(c, clock(tm.left(now))).FontSize(68).Bold().LetterSpacing(1).TextColor(timeColor).
				ContentTransition(ui.ContentTransition{Numeric: true}).SingleLine()
		})
		if face.Clicked() && tm.fresh() {
			a.settingsOpen = true
		}
		status := "准备开始"
		switch {
		case tm.running:
			status = "进行中"
		case tm.remain < tm.total():
			status = "已暂停"
		}
		ui.Text(c, status).FontSize(sz(13)).TextColor(colMuted).
			ContentTransition(ui.ContentTransition{})

		// A quick nudge of the length, while the timer waits.
		step := 1
		if tm.mode == modeFocus {
			step = 5
		}
		idle := tm.fresh()
		ui.Row(c.Key("nudge")).Gap(14).AlignItems(ui.Center).Padding(8, 0, 0, 0).Children(func() {
			if a.button(c, accent, now, btn{key: "nudge-", label: "−", w: 32, h: 32, bg: colSurface, fg: colMuted, fgHover: colText,
				fs: 18, repeat: true, hidden: !idle, disabled: !tm.canAdjust(tm.mode, -step), tip: fmt.Sprintf("减少 %d 分钟", step)}) {
				tm.adjust(tm.mode, -step)
			}
			if a.button(c, accent, now, btn{key: "nudge+", label: "+", w: 32, h: 32, bg: colSurface, fg: colMuted, fgHover: colText,
				fs: 18, repeat: true, hidden: !idle, disabled: !tm.canAdjust(tm.mode, step), tip: fmt.Sprintf("增加 %d 分钟", step)}) {
				tm.adjust(tm.mode, step)
			}
		})
	})
}

// assumeGPU makes the tests draw as a window on a GPU does, which they
// do not, painting in memory: the screenshots and the frame costs then
// show the look and the work of the real thing.
var assumeGPU bool

// ringGeo holds the ring's shapes, built once for the ring's place and
// size, so that a frame builds no paths: the track, the arc, which a frame
// only trims, and the knob.
type ringGeo struct {
	cx, cy float32
	track  ui.Path
	arc    ui.Path
	knob   ui.Path
	built  bool
}

const (
	arcSteps    = 120
	tickCount   = 60
	tickInner   = ringRadius + 22
	tickOuter   = ringRadius + 27
	tickOuterBg = ringRadius + 33
)

var unitCircle = func() (t [arcSteps + 1][2]float32) {
	for i := range t {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/arcSteps
		t[i] = [2]float32{float32(math.Cos(ang)), float32(math.Sin(ang))}
	}
	return
}()

var tickDirs = func() (t [tickCount][2]float32) {
	for i := range t {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/tickCount
		t[i] = [2]float32{float32(math.Cos(ang)), float32(math.Sin(ang))}
	}
	return
}()

func (g *ringGeo) build(cx, cy float32) {
	g.cx, g.cy, g.built = cx, cy, true
	g.track.Reset().Circle(cx, cy, ringRadius)
	g.arc.Reset()
	for i, u := range unitCircle {
		x, y := cx+ringRadius*u[0], cy+ringRadius*u[1]
		if i == 0 {
			g.arc.MoveTo(x, y)
		} else {
			g.arc.LineTo(x, y)
		}
	}
}

// draw paints the ticks, the track and the arc with its glow. Where the
// frame is drawn by the CPU (no GPU, or Windows' software renderer) it
// leaves the glow out, which is what costs there.
func (g *ringGeo) draw(p *ui.Painter, r ui.Rect, accent ui.Color, prog float32, software bool) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if !g.built || g.cx != cx || g.cy != cy {
		g.build(cx, cy)
	}
	prog = max(0, min(prog, 1))

	// Sixty ticks around, lit as the minutes of the session go by.
	lit := int(math.Ceil(float64(prog) * tickCount))
	for i := 0; i < tickCount; i++ {
		out, w := float32(tickOuter), float32(1.5)
		if i%5 == 0 {
			out, w = tickOuterBg, 2
		}
		col := colTrack
		if i < lit {
			col = accent.Alpha(0.55)
		}
		u := tickDirs[i]
		p.Line(cx+u[0]*tickInner, cy+u[1]*tickInner, cx+u[0]*out, cy+u[1]*out, w, col)
	}

	// The track.
	p.StrokePath(&g.track, ringWidth, colTrack)

	// The arc, from twelve o'clock, clockwise.
	if prog > 0.001 {
		g.arc.Trim(0, prog)
		if !software {
			p.StrokePath(&g.arc, ringWidth+18, accent.Alpha(0.09))
		}
		p.StrokePath(&g.arc, ringWidth, accent)

		// A bright knob at the end of the arc.
		pos := float64(prog) * arcSteps
		i := min(int(pos), arcSteps-1)
		f := float32(pos - float64(i))
		ux := unitCircle[i][0] + (unitCircle[i+1][0]-unitCircle[i][0])*f
		uy := unitCircle[i][1] + (unitCircle[i+1][1]-unitCircle[i][1])*f
		g.knob.Reset().Circle(cx+ringRadius*ux, cy+ringRadius*uy, 6)
		p.FillPath(&g.knob, colText)
	}
}

// controls are Reset, the big Start/Pause button, and Skip.
func (a *app) controls(c *ui.Context, accent ui.Color, now time.Time) {
	tm := a.tm
	ui.Row(c.Key("controls")).Gap(12).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		if a.button(c, accent, now, btn{key: "reset", label: "重置", w: 72, h: 46, bg: colSurface, fg: colMuted, fgHover: colText, fs: 15}) {
			tm.reset()
		}
		label := "开始"
		switch {
		case tm.running:
			label = "暂停"
		case tm.remain < tm.total():
			label = "继续"
		}
		if a.button(c, accent, now, btn{key: "toggle", label: label, w: 140, h: 60, bg: accent, fg: colInk, fgHover: colInk, fs: 18, primary: true}) {
			tm.toggle(now)
		}
		if a.button(c, accent, now, btn{key: "skip", label: "跳过", w: 72, h: 46, bg: colSurface, fg: colMuted, fgHover: colText, fs: 15}) {
			a.skipSession()
		}
	})
}

// btn describes a button of the app.
type btn struct {
	key, label string
	sub        string  // a second, smaller line under the label
	w, h, fs   float32 // w 0 makes the button share the room of its row
	bg, fg     ui.Color
	fgHover    ui.Color
	primary    bool
	repeat     bool // holding it repeats the click
	hidden     bool // faded out, and taking no clicks
	disabled   bool
	tip        string
}

// hold is a button held down, for the ones that repeat.
type hold struct {
	start, last time.Time
	fired       bool
}

// button is a pill that swells under the pointer, sinks when pressed and
// eases its colors; it reports a click. These buttons take no keyboard
// focus, so that Space keeps starting and pausing the timer after a click.
func (a *app) button(c *ui.Context, accent ui.Color, now time.Time, o btn) bool {
	b := ui.Row(c.Key(o.key)).Height(o.h).Radius(999).
		Justify(ui.Center).AlignItems(ui.Center).Label(o.label + o.sub)
	if o.w > 0 {
		b.Width(o.w).Shrink(0)
	} else {
		b.Grow(1).Basis(0).MinWidth(0)
	}
	if o.tip != "" {
		b.Tooltip(o.tip)
	}
	off := o.hidden || o.disabled
	b.Disabled(off)
	if !off {
		b.Cursor(ui.CursorPointer)
	}
	hov, press := b.Hovered() && !off, b.Pressed() && !off

	scale := float32(1)
	face, text := o.bg, o.fg
	switch {
	case press:
		scale = 0.95
		face = o.bg.Mix(colBg, 0.18)
	case hov:
		scale = 1.05
		text = o.fgHover
		if o.primary {
			face = o.bg.Mix(colText, 0.16)
		} else {
			face = o.bg.Mix(colText, 0.09)
		}
	}
	s := b.AnimateTo("scale", scale, ui.Snappy)
	b.Scale(s, s)
	b.Background(b.AnimateColor("face", face, ui.Animation{Duration: 140 * time.Millisecond}))
	opacity := float32(1)
	switch {
	case o.hidden:
		opacity = 0
	case o.disabled:
		opacity = 0.3
	}
	b.Opacity(b.AnimateTo("opacity", opacity, ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}))
	if o.primary {
		// A soft glow in the accent under the main button.
		if !a.software { // a blurred shadow is what a CPU-drawn frame pays for
			b.Shadow(0, 8, 24, 0, accent.Alpha(0.30))
		}
	} else {
		b.Border(1, colTrack)
	}
	textColor := b.AnimateColor("text", text, ui.Animation{Duration: 140 * time.Millisecond})
	subColor := b.AnimateColor("sub", text.Alpha(0.6), ui.Animation{Duration: 140 * time.Millisecond})
	b.Children(func() {
		if o.sub == "" {
			ui.Text(c, o.label).FontSize(sz(o.fs)).FontWeight(600).TextColor(textColor).SingleLine().
				ContentTransition(ui.ContentTransition{})
			return
		}
		ui.Column(c.Key(o.key + "txt")).AlignItems(ui.Center).Gap(1).Children(func() {
			ui.Text(c, o.label).FontSize(sz(o.fs)).FontWeight(600).TextColor(textColor).SingleLine()
			ui.Text(c, o.sub).FontSize(sz(o.fs - 1)).TextColor(subColor).SingleLine()
		})
	})

	clicked := b.Clicked() && !off
	if o.repeat {
		if a.held == nil {
			a.held = map[string]*hold{}
		}
		h := a.held[o.key]
		if press {
			if h == nil {
				h = &hold{start: now, last: now}
				a.held[o.key] = h
			}
			if now.Sub(h.start) > 450*time.Millisecond && now.Sub(h.last) >= 70*time.Millisecond {
				h.last, h.fired = now, true
				clicked = true
			}
			c.AnimationFrame()
		} else if h != nil {
			if h.fired {
				clicked = false // the release of a long hold is no click
			}
			delete(a.held, o.key)
		}
	}
	return clicked
}

// footer shows the dots of the cycle, today's count and the shortcuts.
func (a *app) footer(c *ui.Context, accent ui.Color) {
	tm := a.tm
	ui.Column(c.Key("footer")).AlignItems(ui.Stretch).Gap(10).Children(func() {
		ui.Row(c.Key("dots")).Gap(10).Justify(ui.Center).Children(func() {
			for i := 0; i < tm.set.Every; i++ {
				d := ui.Box(c.Key(fmt.Sprint("dot", i))).Size(10, 10).Radius(999)
				on := i < tm.filled()
				col, sc := colTrack, float32(1)
				if on {
					col, sc = accents[modeFocus], 1.25
				}
				d.Background(d.AnimateColor("c", col, ui.Animation{Duration: 300 * time.Millisecond}))
				k := d.AnimateTo("k", sc, ui.Bouncy)
				d.Scale(k, k)
			}
		})
		ui.Text(c, fmt.Sprintf("今日已完成 %d 个番茄", tm.sessions)).FontSize(sz(14)).TextColor(colMuted).TextAlign(ui.Center).
			ContentTransition(ui.ContentTransition{Numeric: true})
		ui.Text(c, "空格  开始 / 暂停　·　R  重置　·　S  跳过").FontSize(sz(12)).TextColor(colDim).TextAlign(ui.Center)
	})
}

// settingsPanel slides up over a dimmed window: the length of each session,
// how many pomodoros earn a long break, and three presets. Everything in it
// shares the width the panel has, so wide text cannot push a control out.
func (a *app) settingsPanel(c *ui.Context, accent ui.Color, now time.Time, open float32) {
	tm := a.tm
	// The scrim: a press outside the panel closes it.
	scrim := ui.Box(c.Key("scrim")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.RGBA(4, 5, 8, 0.62*open))
	if scrim.Clicked() {
		a.settingsOpen = false
	}

	panel := ui.Column(c.Key("panel")).Absolute().Left(10).Right(10).Bottom(10).
		Padding(16).Gap(14).Radius(24).Background(colSurface).Border(1, colEdge).
		Shadow(0, 20, 50, 0, ui.RGBA(0, 0, 0, 0.5)).AlignItems(ui.Stretch)
	panel.Opacity(open).Translate(0, (1-open)*40)
	panel.Children(func() {
		ui.Row(c.Key("phead")).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "设置").FontSize(sz(20)).Bold().TextColor(colText).SingleLine()
			ui.Box(c.Key("pspacer")).Grow(1)
			if a.button(c, accent, now, btn{key: "done", label: "完成", w: 72, h: 36, bg: accent, fg: colInk, fgHover: colInk, fs: 14, primary: true}) {
				a.settingsOpen = false
			}
		})
		ui.Text(c, "时长").FontSize(sz(12)).FontWeight(600).LetterSpacing(2).TextColor(colMuted)
		for m := range modes {
			a.stepRow(c, accent, now, fmt.Sprint("len", m), modes[m].Name, accents[m],
				fmt.Sprintf("%d 分钟", tm.set.minutes(m)),
				tm.canAdjust(m, -1), tm.canAdjust(m, 1),
				func(d int) { tm.adjust(m, d) })
		}
		ui.Box(c.Key("rule")).Height(1).Background(colTrack)
		a.stepRow(c, accent, now, "every", "长休间隔", colMuted,
			fmt.Sprintf("每 %d 个", tm.set.Every),
			tm.set.Every > everyMin, tm.set.Every < everyMax,
			func(d int) {
				s := tm.set
				s.Every += d
				tm.configure(s)
			})
		ui.Text(c, "每完成这么多个番茄，就进入一次长休息。").FontSize(sz(12)).TextColor(colDim)
		ui.Text(c, "快速选择").FontSize(sz(12)).FontWeight(600).LetterSpacing(2).TextColor(colMuted)
		// Three chips of the same width, whatever the window's.
		ui.Row(c.Key("presets")).Gap(8).Children(func() {
			for i, p := range presets {
				fg, bg := colMuted, colRaised
				if tm.set == p.Set {
					fg, bg = colText, accent.Alpha(0.18)
				}
				if a.button(c, accent, now, btn{key: fmt.Sprint("preset", i), label: p.Name, sub: fmt.Sprintf("%d/%d/%d", p.Set.Focus, p.Set.Short, p.Set.Long),
					h: 54, bg: bg, fg: fg, fgHover: colText, fs: 14}) {
					tm.configure(p.Set)
				}
			}
		})
		ui.Text(c, "计时进行中修改，将在下次重置或下一轮生效。").FontSize(sz(12)).TextColor(colDim)
		// Which renderer draws the window: a slow one is why an animation stutters.
		render := "显示：GPU 加速"
		if a.software {
			render = "显示：软件渲染（已精简特效）"
		}
		ui.Text(c, render).FontSize(sz(11)).TextColor(colDim).SingleLine()
	})
}

// stepRow is a setting with a − and a + around its value. The buttons and
// the value keep to the right edge, in columns that line up from row to row,
// and the name takes what is left of the width, cut with an ellipsis before
// it could push a control out.
func (a *app) stepRow(c *ui.Context, accent ui.Color, now time.Time, key, name string, dot ui.Color, value string, canLess, canMore bool, change func(delta int)) {
	ui.Row(c.Key(key)).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Box(c.Key(key+"dot")).Size(8, 8).Radius(999).Background(dot).Shrink(0)
		ui.Text(c, name).FontSize(sz(15)).FontWeight(500).TextColor(colText).SingleLine().Grow(1).Basis(0).MinWidth(0)
		ui.Row(c.Key(key + "ctl")).AlignItems(ui.Center).Gap(4).Shrink(0).Children(func() {
			if a.button(c, accent, now, btn{key: key + "-", label: "−", w: 32, h: 32, bg: colRaised, fg: colText, fgHover: colText,
				fs: 18, repeat: true, disabled: !canLess}) {
				change(-1)
			}
			ui.Row(c.Key(key + "val")).MinWidth(sz(58)).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, value).FontSize(sz(15)).Bold().TextColor(colText).SingleLine().
					ContentTransition(ui.ContentTransition{Numeric: true})
			})
			if a.button(c, accent, now, btn{key: key + "+", label: "+", w: 32, h: 32, bg: colRaised, fg: colText, fgHover: colText,
				fs: 18, repeat: true, disabled: !canMore}) {
				change(1)
			}
		})
	})
}
