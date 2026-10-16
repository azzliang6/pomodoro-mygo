package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func newMiniTest(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	assumeGPU = os.Getenv("FRAMECOST_SOFT") == ""
	a := &app{tm: newTimer()}
	a.tm.notify = func(string, string) {}
	return a, ui.NewTester(a.miniView, miniW, miniH)
}

// wire makes the app's window switch hooks record what they are asked.
func wire(a *app, calls *[]bool) {
	a.onMini = func(v bool) { *calls = append(*calls, v) }
}

func TestMiniShowsKeyInfo(t *testing.T) {
	for _, scale := range []float32{1, 1.3} {
		a, tt := newMiniTest(t)
		tt.SetPreferences(ui.Preferences{TextScale: scale})
		tt.Frame()
		tt.Advance(time.Second)
		for _, s := range []string{"25:00", "专注"} {
			if !tt.HasText(s) {
				t.Fatalf("no %q in %q", s, tt.Texts())
			}
		}
		// Everything lies inside the small window.
		for _, s := range tt.Texts() {
			if r, ok := tt.Find(s); ok && r.W > 0 && (r.X < 0 || r.X+r.W > miniW || r.Y < 0 || r.Y+r.H > miniH) {
				t.Errorf("scale %v: %q at %+v is outside %dx%d", scale, s, r, miniW, miniH)
			}
		}
		if err := tt.Click("开始"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(65 * time.Second)
		if !a.tm.running || !tt.HasText("23:5") || !tt.HasText("暂停") {
			t.Fatalf("running %v texts %q", a.tm.running, tt.Texts())
		}
		tt.Key(0, ui.KeySpace)
		tt.Advance(time.Second)
		if a.tm.running || !tt.HasText("继续") {
			t.Fatalf("Space did not pause: %q", tt.Texts())
		}
		tt.Key(0, ui.KeyR)
		tt.Advance(time.Second)
		if !tt.HasText("25:00") {
			t.Fatalf("R did not reset: %q", tt.Texts())
		}
		tt.Key(0, ui.KeyS) // nothing to lose: skips at once
		tt.Advance(time.Second)
		if a.tm.mode != modeShort || !tt.HasText("短休息") || !tt.HasText("05:00") {
			t.Fatalf("S did not skip: mode %d %q", a.tm.mode, tt.Texts())
		}
	}
}

func TestMiniSwitching(t *testing.T) {
	// The full window's button and key, and the mini window's.
	a, tt, _ := newTest(t)
	var calls []bool
	wire(a, &calls)
	tt.Frame()
	if err := tt.Click("迷你窗口"); err != nil {
		t.Fatal(err)
	}
	if !a.tm.mini || len(calls) != 1 || !calls[0] {
		t.Fatalf("mini %v calls %v", a.tm.mini, calls)
	}
	a.tm.mini, calls = false, nil
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyM)
	if !a.tm.mini || len(calls) != 1 || !calls[0] {
		t.Fatalf("M: mini %v calls %v", a.tm.mini, calls)
	}

	b, mt := newMiniTest(t)
	calls = nil
	wire(b, &calls)
	b.tm.mini = true
	mt.Frame()
	if err := mt.Click("还原窗口"); err != nil {
		t.Fatal(err)
	}
	if b.tm.mini || len(calls) != 1 || calls[0] {
		t.Fatalf("restore: mini %v calls %v", b.tm.mini, calls)
	}
	b.tm.mini, calls = true, nil
	mt.Advance(time.Second)
	mt.Key(0, ui.KeyEscape)
	if b.tm.mini || len(calls) != 1 {
		t.Fatalf("Esc: mini %v calls %v", b.tm.mini, calls)
	}
}

func TestMiniPersists(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	a := newTimer()
	a.saveSettings = func() { a.writeSettings(path) }
	a.configure(settings{Focus: 30, Short: 5, Long: 15, Every: 4})
	a.setMini(true)
	b := newTimer()
	b.loadSettings(path)
	if !b.mini || b.set.Focus != 30 {
		t.Errorf("loaded mini %v, %+v", b.mini, b.set)
	}
	b.setMini(false)
	b.saveSettings = func() { b.writeSettings(path) }
	b.setMini(true)
	b.setMini(false)
	c := newTimer()
	c.loadSettings(path)
	if c.mini {
		t.Error("the mini state was not saved as off")
	}
	// A file from before has no mini field: the full window.
	os.WriteFile(path, []byte(`{"focus":40,"short":8,"long":20,"every":3}`), 0o644)
	d := newTimer()
	d.loadSettings(path)
	if d.mini || d.set.Focus != 40 {
		t.Errorf("old file: mini %v, %+v", d.mini, d.set)
	}
}

func TestMiniBuildsOncePerSecond(t *testing.T) {
	a := &app{tm: newTimer()}
	assumeGPU = true
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.miniView(c) }, miniW, miniH)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Advance(3 * time.Second)
	before := builds
	tt.Advance(20 * time.Second)
	if n := builds - before; n < 15 || n > 30 {
		t.Errorf("%d builds in 20 s, want about one a second", n)
	}
}

// Another tab while the timer has progress asks first; at rest, it does not.
func TestSwitchAsksFirst(t *testing.T) {
	a, tt, _ := newTest(t)
	tt.Frame()
	// Idle at full time: switches at once.
	if err := tt.Click("短休息"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.tm.mode != modeShort || a.confirm != nil {
		t.Fatalf("idle switch: mode %d, ask %v", a.tm.mode, a.confirm)
	}
	tt.Click("专注")
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySpace)
	tt.Advance(90 * time.Second)
	left := a.tm.left(tt.Now())

	// Running: asks, and nothing changes.
	if err := tt.Click("长休息"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.confirm == nil || a.tm.mode != modeFocus || !a.tm.running {
		t.Fatalf("running: ask %v mode %d running %v", a.confirm, a.tm.mode, a.tm.running)
	}
	for _, s := range []string{"切换到「长休息」？", "切换将重置当前进度，是否继续？", "取消", "切换"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	// The keys keep working behind the question: Space pauses.
	tt.Key(0, ui.KeySpace)
	tt.Advance(time.Second)
	if a.tm.running || a.confirm == nil {
		t.Fatalf("Space with a question open: running %v, ask %v", a.tm.running, a.confirm)
	}
	// Cancel by the button: the progress is as it was.
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.confirm != nil || a.tm.mode != modeFocus || a.tm.left(tt.Now()) > left || a.tm.remain == a.tm.span {
		t.Fatalf("cancel: ask %v mode %d remain %v", a.confirm, a.tm.mode, a.tm.remain)
	}
	// Cancel by Escape.
	tt.Click("短休息")
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if a.confirm != nil || a.tm.mode != modeFocus || a.settingsOpen {
		t.Fatalf("Esc: ask %v mode %d", a.confirm, a.tm.mode)
	}
	// Confirm by the button.
	tt.Click("短休息")
	tt.Advance(time.Second)
	if err := tt.Click("切换"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.confirm != nil || a.tm.mode != modeShort || !a.tm.fresh() {
		t.Fatalf("confirm: ask %v mode %d fresh %v", a.confirm, a.tm.mode, a.tm.fresh())
	}
	// Confirm by Enter, from paused mid-session.
	tt.Key(0, ui.KeySpace)
	tt.Advance(20 * time.Second)
	tt.Key(0, ui.KeySpace)
	tt.Advance(time.Second)
	tt.Click("长休息")
	tt.Advance(time.Second)
	if a.confirm == nil {
		t.Fatal("paused with progress should ask")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if a.confirm != nil || a.tm.mode != modeLong || !a.tm.fresh() {
		t.Fatalf("Enter: ask %v mode %d", a.confirm, a.tm.mode)
	}
	// Skipping asks the same way.
	tt.Key(0, ui.KeySpace)
	tt.Advance(5 * time.Second)
	tt.Key(0, ui.KeyS)
	tt.Advance(time.Second)
	if a.confirm == nil || a.tm.mode != modeLong {
		t.Fatalf("skip should ask: ask %v mode %d", a.confirm, a.tm.mode)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	// R is an explicit reset: no question, and an open one goes away.
	tt.Click("跳过")
	tt.Advance(time.Second)
	if a.confirm == nil {
		t.Fatal("the skip button should ask")
	}
	tt.Key(0, ui.KeyR)
	tt.Advance(time.Second)
	if a.confirm != nil || !a.tm.fresh() || a.tm.mode != modeLong {
		t.Fatalf("R: ask %v mode %d fresh %v", a.confirm, a.tm.mode, a.tm.fresh())
	}
}

func TestMiniSkipAsks(t *testing.T) {
	a, tt := newMiniTest(t)
	tt.SetPreferences(ui.Preferences{TextScale: 1.3}) // the largest text still fits
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Advance(30 * time.Second)
	if err := tt.Click("跳过"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.confirm == nil || !tt.HasText("取消") {
		t.Fatalf("no question: %q", tt.Texts())
	}
	for _, s := range tt.Texts() {
		if r, ok := tt.Find(s); ok && r.W > 0 && (r.X < 0 || r.X+r.W > miniW || r.Y < 0 || r.Y+r.H > miniH) {
			t.Errorf("%q at %+v is outside the mini window", s, r)
		}
	}
	tt.Click("取消")
	tt.Advance(time.Second)
	if a.confirm != nil || a.tm.mode != modeFocus {
		t.Fatal("cancel changed something")
	}
	tt.Click("跳过")
	tt.Advance(time.Second)
	tt.Click("确定跳过")
	tt.Advance(time.Second)
	if a.tm.mode != modeShort || !a.tm.fresh() {
		t.Fatalf("confirmed skip: mode %d fresh %v", a.tm.mode, a.tm.fresh())
	}
}

// A length changed while a session runs waits for the next one: nothing to ask.
func TestDurationChangeDoesNotWipe(t *testing.T) {
	tm := newTimer()
	now := time.Unix(1000, 0)
	tm.start(now)
	tm.tick(now.Add(10 * time.Minute))
	span := tm.span
	left := tm.left(now.Add(10 * time.Minute))
	tm.adjust(modeFocus, 5)
	tm.configure(presets[1].Set)
	if tm.span != span || tm.left(now.Add(10*time.Minute)) != left || !tm.running {
		t.Errorf("a setting changed the session: span %v left %v running %v", tm.span, tm.left(now.Add(10*time.Minute)), tm.running)
	}
}

// TestMiniScreenshots writes the mini window and the question to SHOTS.
func TestMiniScreenshots(t *testing.T) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		t.Skip("set SHOTS to a directory to write screenshots")
	}
	save := func(tt *ui.Tester, name string) {
		f, err := os.Create(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	a, tt := newMiniTest(t)
	tt.SetScale(2)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Advance(9*time.Minute + 17*time.Second)
	save(tt, "9-mini-focus.png")
	tt.Click("跳过")
	tt.Advance(time.Second)
	save(tt, "12-mini-skip-question.png")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	a.tm.sessions, a.tm.cycle = 2, 2
	tt.Key(0, ui.KeySpace)
	tt.Advance(100*time.Second + 500*time.Millisecond)
	save(tt, "10-mini-break.png")

	b, tb, _ := newTest(t)
	tb.SetScale(2)
	tb.Frame()
	tb.Key(0, ui.KeySpace)
	tb.Advance(9*time.Minute + 17*time.Second)
	tb.Click("短休息")
	tb.Advance(time.Second)
	save(tb, "11-switch-question.png")
	_ = b
}

// TestSoftwareLook: where the frame is drawn by the CPU, as the Tester's
// are, the app leaves the glow out and says so in the settings.
func TestSoftwareLook(t *testing.T) {
	a, tt, _ := newTest(t)
	assumeGPU = false
	defer func() { assumeGPU = true }()
	tt.SetSize(340, 690)
	tt.SetPreferences(ui.Preferences{TextScale: 1.3})
	tt.Frame()
	tt.Advance(time.Second)
	if !a.software {
		t.Fatal("the Tester's frames are drawn in memory, which the app should know")
	}
	tt.Click("设置")
	tt.Advance(1500 * time.Millisecond)
	r, ok := tt.Find("显示：软件渲染（已精简特效）")
	if !ok {
		t.Fatalf("no renderer line in %q", tt.Texts())
	}
	if r.X < 0 || r.X+r.W > 340 {
		t.Errorf("the renderer line %+v is outside the window", r)
	}
}

// TestRunningBuildsFewFrames: a running timer builds the view when the
// digits change and between, a couple of times a second, not at every
// refresh of the display (the ring moves a fraction of a pixel a second).
func TestRunningBuildsFewFrames(t *testing.T) {
	a := &app{tm: newTimer()}
	assumeGPU = true
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 440, 690)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	tt.Advance(3 * time.Second) // the start's own animations end
	before := builds
	tt.Advance(20 * time.Second)
	if n := builds - before; n < 20 || n > 60 {
		t.Errorf("%d builds in 20 s of running, want about 2 a second", n)
	}
	if !tt.HasText("24:3") && !tt.HasText("24:2") {
		t.Errorf("the clock does not run: %q", tt.Texts())
	}
}

// TestTitleBarPitch: the title bar's two buttons and the window's three
// caption buttons (46 wide each, 138 reserved on the right) make one strip
// at a pitch of 46, with no gap, whatever the width and the text size.
func TestTitleBarPitch(t *testing.T) {
	barInset = 3 * captionWidth
	defer func() { barInset = 0 }()
	for _, w := range []int{360, 440, 520} {
		for _, scale := range []float32{1, 1.3} {
			_, tt, _ := newTest(t)
			tt.SetSize(w, 690)
			tt.SetPreferences(ui.Preferences{TextScale: scale})
			tt.Frame()
			tt.Advance(time.Second)
			mini, ok1 := tt.Find("迷你窗口")
			gear, ok2 := tt.Find("设置")
			if !ok1 || !ok2 {
				t.Fatalf("buttons not found in %q", tt.Texts())
			}
			// Centers of the five: mini, gear, then the window's three.
			centers := []float32{mini.X + mini.W/2, gear.X + gear.W/2}
			for i := 0; i < 3; i++ {
				centers = append(centers, float32(w)-3*captionWidth+captionWidth/2+float32(i)*captionWidth)
			}
			for i := 1; i < len(centers); i++ {
				if d := centers[i] - centers[i-1]; d < captionWidth-0.5 || d > captionWidth+0.5 {
					t.Errorf("width %d scale %v: pitch between %d and %d is %v, want %d (mini %+v gear %+v)", w, scale, i-1, i, d, captionWidth, mini, gear)
				}
			}
			if mini.W < captionWidth-0.5 || gear.W < captionWidth-0.5 {
				t.Errorf("buttons %v and %v wide, want %d", mini.W, gear.W, captionWidth)
			}
		}
	}
}
