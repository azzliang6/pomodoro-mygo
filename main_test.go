package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func newTest(t *testing.T) (*app, *ui.Tester, *[]string) {
	assumeGPU = os.Getenv("FRAMECOST_SOFT") == ""
	a := &app{tm: newTimer()}
	var notes []string
	a.tm.notify = func(title, body string) { notes = append(notes, title) }
	return a, ui.NewTester(a.view, 440, 690), &notes
}

func TestStartPauseReset(t *testing.T) {
	a, tt, _ := newTest(t)
	if !tt.HasText("25:00") || !tt.HasText("准备开始") {
		t.Fatalf("initial texts %q", tt.Texts())
	}
	if err := tt.Click("开始"); err != nil {
		t.Fatal(err)
	}
	if !a.tm.running {
		t.Fatal("not running after Start")
	}
	tt.Advance(90*time.Second + 500*time.Millisecond)
	if !tt.HasText("23:30") {
		t.Errorf("after 90 s: %q", tt.Texts())
	}
	if err := tt.Click("暂停"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Minute) // paused: the time does not move
	if a.tm.running || !tt.HasText("23:30") || !tt.HasText("已暂停") {
		t.Errorf("paused: running=%v texts %q", a.tm.running, tt.Texts())
	}
	if err := tt.Click("继续"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(30 * time.Second)
	if !tt.HasText("23:00") {
		t.Errorf("after resume: %q", tt.Texts())
	}
	if err := tt.Click("重置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.tm.running || !tt.HasText("25:00") {
		t.Errorf("after reset: %q", tt.Texts())
	}
}

func TestSpaceAndShortcuts(t *testing.T) {
	a, tt, _ := newTest(t)
	tt.Key(0, ui.KeySpace)
	if !a.tm.running {
		t.Fatal("Space did not start")
	}
	tt.Key(0, ui.KeySpace)
	if a.tm.running {
		t.Fatal("Space did not pause")
	}
	tt.Key(0, ui.KeyS)
	if a.tm.mode != modeShort || !tt.HasText("05:00") {
		t.Errorf("S: mode %d texts %q", a.tm.mode, tt.Texts())
	}
	tt.Key(0, ui.KeyR)
	if a.tm.remain != 5*time.Minute {
		t.Errorf("R: remain %v", a.tm.remain)
	}
}

func TestModes(t *testing.T) {
	a, tt, _ := newTest(t)
	if err := tt.Click("短休息"); err != nil {
		t.Fatal(err)
	}
	if a.tm.mode != modeShort || !tt.HasText("05:00") {
		t.Errorf("short: %d %q", a.tm.mode, tt.Texts())
	}
	tt.Advance(time.Second)
	if err := tt.Click("长休息"); err != nil {
		t.Fatal(err)
	}
	if a.tm.mode != modeLong || !tt.HasText("15:00") {
		t.Errorf("long: %d %q", a.tm.mode, tt.Texts())
	}
	tt.Advance(time.Second)
	if err := tt.Click("专注"); err != nil {
		t.Fatal(err)
	}
	if a.tm.mode != modeFocus || !tt.HasText("25:00") {
		t.Errorf("focus: %d %q", a.tm.mode, tt.Texts())
	}
}

func TestSessionEnds(t *testing.T) {
	// The same sessions, in seconds, so that the test runs in no time.
	unit = time.Second
	defer func() { unit = time.Minute }()
	a, tt, notes := newTest(t)
	every := a.tm.set.Every
	for round := 1; round <= every; round++ {
		tt.Key(0, ui.KeySpace) // start a focus
		tt.Advance(25*time.Second + time.Second)
		if a.tm.running || a.tm.sessions != round {
			t.Fatalf("round %d: running=%v sessions=%d", round, a.tm.running, a.tm.sessions)
		}
		want := modeShort
		if round == every {
			want = modeLong
		}
		if a.tm.mode != want {
			t.Fatalf("round %d: next mode %d, want %d", round, a.tm.mode, want)
		}
		tt.Advance(2 * time.Second) // the ring's swell and the colors settle
		if tt.FramePending() {
			t.Fatalf("round %d: the view keeps asking for frames at rest", round)
		}
		if want == modeLong {
			continue
		}
		tt.Key(0, ui.KeySpace) // and its break
		tt.Advance(5*time.Second + time.Second)
		if a.tm.mode != modeFocus {
			t.Fatalf("round %d: after the break, mode %d", round, a.tm.mode)
		}
	}
	if len(*notes) != every*2-1 {
		t.Errorf("notifications %q", *notes)
	}
	if !tt.HasText("今日已完成 4 个番茄") {
		t.Errorf("texts %q", tt.Texts())
	}
	if a.tm.filled() != every {
		t.Errorf("filled %d", a.tm.filled())
	}
}

func TestPersistence(t *testing.T) {
	path := t.TempDir() + "/state.json"
	now := time.Now()
	a := newTimer()
	a.sessions, a.cycle = 3, 3
	a.write(path, now)
	b := newTimer()
	b.load(path, now)
	if b.sessions != 3 || b.cycle != 3 {
		t.Errorf("loaded %d/%d", b.sessions, b.cycle)
	}
	c := newTimer()
	c.load(path, now.AddDate(0, 0, 1)) // another day starts from zero
	if c.sessions != 0 {
		t.Errorf("next day %d", c.sessions)
	}
}

// center is the middle of a text's box.
func center(t *testing.T, tt *ui.Tester, s string) (float32, float32) {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("no %q in %q", s, tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// TestSpaceAfterClicks: the buttons take no focus, so Space still works.
func TestSpaceAfterClicks(t *testing.T) {
	a, tt, _ := newTest(t)
	tt.Click("短休息")
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySpace)
	if !a.tm.running {
		t.Fatal("Space did nothing after clicking a tab")
	}
	tt.Click("暂停")
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySpace)
	if !a.tm.running {
		t.Fatal("Space did nothing after clicking Pause")
	}
}

func TestNudge(t *testing.T) {
	a, tt, _ := newTest(t)
	tt.Advance(time.Second)
	if err := tt.Click("+"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.tm.set.Focus != 30 || !tt.HasText("30:00") {
		t.Errorf("after +: focus %d, texts %q", a.tm.set.Focus, tt.Texts())
	}
	if err := tt.Click("−"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.tm.set.Focus != 25 || !tt.HasText("25:00") {
		t.Errorf("after −: focus %d", a.tm.set.Focus)
	}
	// Short breaks move a minute at a time.
	tt.Click("短休息")
	tt.Advance(time.Second)
	tt.Click("+")
	tt.Advance(time.Second)
	if a.tm.set.Short != 6 || !tt.HasText("06:00") {
		t.Errorf("short break %d, texts %q", a.tm.set.Short, tt.Texts())
	}
	// Running, the nudge is gone: nothing changes.
	tt.Key(0, ui.KeySpace)
	tt.Advance(time.Second)
	x, y := center(t, tt, "+")
	tt.ClickAt(x, y)
	if a.tm.set.Short != 6 {
		t.Errorf("a hidden nudge changed the length to %d", a.tm.set.Short)
	}
}

func TestHoldRepeats(t *testing.T) {
	a, tt, _ := newTest(t)
	tt.Advance(time.Second)
	x, y := center(t, tt, "+")
	tt.Press(x, y)
	tt.Advance(1500 * time.Millisecond)
	tt.Release(x, y)
	tt.Advance(time.Second)
	if a.tm.set.Focus < 40 {
		t.Errorf("a long hold moved the focus to %d minutes", a.tm.set.Focus)
	}
	got := a.tm.set.Focus
	tt.Advance(time.Second)
	if a.tm.set.Focus != got {
		t.Errorf("the length kept moving after the release: %d", a.tm.set.Focus)
	}
}

func TestSettingsPanel(t *testing.T) {
	a, tt, _ := newTest(t)
	var saved int
	a.tm.saveSettings = func() { saved++ }
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.settingsOpen || !tt.HasText("25 分钟") || !tt.HasText("每 4 个") {
		t.Fatalf("panel: open=%v texts %q", a.settingsOpen, tt.Texts())
	}
	// A press inside the panel does not close it.
	x, y := center(t, tt, "时长")
	tt.ClickAt(x, y)
	tt.Advance(time.Second)
	if !a.settingsOpen {
		t.Fatal("a press on the panel closed it")
	}
	// The + beside the focus length.
	r, _ := tt.Find("25 分钟")
	tt.ClickAt(float32(440-30-16), r.Y+r.H/2) // the + at the panel's right edge
	tt.Advance(time.Second)
	if a.tm.set.Focus != 26 || !tt.HasText("26 分钟") {
		t.Fatalf("after +: focus %d, texts %q", a.tm.set.Focus, tt.Texts())
	}
	// The − beside the short break.
	r, _ = tt.Find("5 分钟")
	tt.ClickAt(r.X+r.W/2-29-4-16, r.Y+r.H/2) // the − just before the value column
	tt.Advance(time.Second)
	if a.tm.set.Short != 4 {
		t.Fatalf("short break %d", a.tm.set.Short)
	}
	if saved != 2 {
		t.Errorf("saved %d times", saved)
	}
	// A preset.
	if err := tt.Click("深度"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.tm.set != presets[1].Set {
		t.Errorf("preset: %+v", a.tm.set)
	}
	// Done closes it and the time shows the new length.
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(2 * time.Second)
	if a.settingsOpen || !tt.HasText("50:00") || false {
		t.Errorf("closed: open=%v texts %q", a.settingsOpen, tt.Texts())
	}
	// Escape too, and clicking the time opens it.
	if err := tt.Click("50:00"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !a.settingsOpen {
		t.Fatal("clicking the time did not open the settings")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(2 * time.Second)
	if a.settingsOpen {
		t.Fatal("Escape did not close the settings")
	}
	// And a press on the dimmed window closes it.
	tt.Click("设置")
	tt.Advance(time.Second)
	tt.ClickAt(200, 60)
	tt.Advance(time.Second)
	if a.settingsOpen {
		t.Fatal("a press outside did not close the settings")
	}
	if tt.FramePending() {
		t.Error("the view keeps asking for frames at rest")
	}
}

func TestConfigureWhileRunning(t *testing.T) {
	tm := newTimer()
	now := time.Now()
	tm.start(now)
	s := tm.set
	s.Focus = 50
	tm.configure(s)
	if tm.total() != 25*time.Minute || tm.left(now.Add(time.Minute)) != 24*time.Minute {
		t.Errorf("a running session changed: total %v", tm.total())
	}
	tm.reset()
	if tm.total() != 50*time.Minute || tm.remain != 50*time.Minute {
		t.Errorf("after reset: total %v remain %v", tm.total(), tm.remain)
	}
	// Paused partway keeps its time too.
	tm.start(now)
	tm.pause(now.Add(10 * time.Minute))
	s.Focus = 30
	tm.configure(s)
	if tm.remain != 40*time.Minute {
		t.Errorf("paused session changed to %v", tm.remain)
	}
	// Limits.
	tm.reset()
	tm.adjust(modeShort, 1000)
	tm.adjust(modeLong, -1000)
	if tm.set.Short != 30 || tm.set.Long != 1 {
		t.Errorf("limits: %+v", tm.set)
	}
}

func TestSettingsPersistence(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	a := newTimer()
	a.saveSettings = func() { a.writeSettings(path) }
	a.configure(settings{Focus: 40, Short: 8, Long: 20, Every: 3})
	b := newTimer()
	b.loadSettings(path)
	if b.set != (settings{40, 8, 20, 3}) || b.remain != 40*time.Minute {
		t.Errorf("loaded %+v, remain %v", b.set, b.remain)
	}
	// A file out of range is brought into it; a damaged one is ignored.
	os.WriteFile(path, []byte(`{"focus":500,"short":0,"long":15,"every":1}`), 0o644)
	c := newTimer()
	c.loadSettings(path)
	if c.set != (settings{90, 1, 15, 2}) {
		t.Errorf("out of range: %+v", c.set)
	}
	os.WriteFile(path, []byte(`not json`), 0o644)
	d := newTimer()
	d.loadSettings(path)
	if d.set != defaultSettings {
		t.Errorf("damaged: %+v", d.set)
	}
	d.loadSettings(path + ".missing")
	if d.set != defaultSettings {
		t.Errorf("missing: %+v", d.set)
	}
}

// TestScreenshots renders the window as PNGs when SHOTS names a directory.
func TestScreenshots(t *testing.T) {
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
	a, tt, _ := newTest(t)
	tt.SetScale(2)
	tt.Frame()
	save(tt, "1-ready.png")
	tt.Key(0, ui.KeySpace)
	tt.Advance(9*time.Minute + 17*time.Second)
	save(tt, "2-focus-running.png")
	a.tm.choose(modeShort)
	a.tm.sessions, a.tm.cycle = 2, 2
	tt.Frame()
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySpace)
	tt.Advance(100*time.Second + 500*time.Millisecond)
	save(tt, "3-short-break.png")

	// The settings panel, over the window.
	a2, tt2, _ := newTest(t)
	a2.tm.sessions, a2.tm.cycle = 1, 1
	a2.tm.configure(presets[1].Set)
	tt2.SetScale(2)
	tt2.Frame()
	tt2.Click("设置")
	tt2.Advance(1500 * time.Millisecond)
	save(tt2, "4-settings.png")

	// The title bar next to stand-ins for the window's caption buttons (the
	// real ones are drawn by Windows' backend and are not in the Tester):
	// 46 wide, the bar's height, a thin white glyph of 10 DIPs, flat.
	barInset = 3 * captionWidth
	previewCaption = func(c *ui.Context) {
		ui.Box(c.Key("fakecaption")).Absolute().Top(0).Right(0).Width(3 * captionWidth).Height(barHeight).PassThrough().
			Draw(func(p *ui.Painter, r ui.Rect) {
				s := p.Scale()
				snap := func(v float32) float32 { return (float32(math.Floor(float64(v*s))) + 0.5) / s }
				w := 1 / s
				cy := r.Y + r.H/2
				mx := r.X + captionWidth/2
				p.Line(snap(mx-5), snap(cy), snap(mx+5), snap(cy), w, captionInk)
				bx := r.X + captionWidth + captionWidth/2
				p.Stroke(ui.Rect{X: snap(bx - 5), Y: snap(cy - 5), W: 10, H: 10}, captionInk, 0, w)
				cx := r.X + 2*captionWidth + captionWidth/2
				p.Line(cx-5, cy-5, cx+5, cy+5, w, captionInk)
				p.Line(cx-5, cy+5, cx+5, cy-5, w, captionInk)
			})
	}
	_, tt4, _ := newTest(t)
	tt4.SetScale(2)
	tt4.Frame()
	tt4.Advance(time.Second)
	cropTop := func(name string) {
		img := tt4.Image()
		b := img.Bounds()
		strip := img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+2*(barHeight+4)))
		f, err := os.Create(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, strip); err != nil {
			t.Fatal(err)
		}
	}
	cropTop("7-titlebar-rest.png")
	gx := 440 - float32(3*captionWidth) - captionWidth/2
	tt4.Move(gx, barHeight/2)
	tt4.Advance(500 * time.Millisecond)
	cropTop("8-titlebar-gear-hover.png")
	tt4.Move(440-captionWidth*2.5, barHeight/2) // over the fake minimize: no hover on the gear
	barInset, previewCaption = 0, nil

	// The narrowest window with the largest text: nothing may clip.
	a3, tt3, _ := newTest(t)
	a3.tm.sessions, a3.tm.cycle = 1, 1
	tt3.SetSize(340, 690)
	tt3.SetPreferences(ui.Preferences{TextScale: 1.3})
	tt3.SetScale(2)
	tt3.Frame()
	tt3.Advance(time.Second)
	save(tt3, "6-main-narrow-large-text.png")
	tt3.Click("设置")
	tt3.Advance(1500 * time.Millisecond)
	save(tt3, "5-settings-narrow-large-text.png")
}

// TestLayoutFits opens the settings at several widths and text sizes wider
// than the Tester's font (a real Windows font is wider), and checks that
// every text lies inside the window, and the panel's inside the panel.
func TestLayoutFits(t *testing.T) {
	for _, w := range []int{340, 360, 400, 440, 520} {
		for _, scale := range []float32{1, 1.15, 1.3} {
			a, tt, _ := newTest(t)
			tt.SetSize(w, 690)
			tt.SetPreferences(ui.Preferences{TextScale: scale})
			tt.Frame()
			name := func(s string) string { return s + " @" + itoa(w) + " x" + ftoa(scale) }
			check := func(stage string, left, right, top, bottom float32, only func(string) bool) {
				for _, s := range tt.Texts() {
					if only != nil && !only(s) {
						continue
					}
					r, ok := tt.Find(s)
					if !ok || r.W == 0 {
						continue
					}
					if r.X < left-1 || r.X+r.W > right+1 || r.Y < top-1 || r.Y+r.H > bottom+1 {
						t.Errorf("%s: %q at %+v is outside [%v %v %v %v]", name(stage), s, r, left, right, top, bottom)
					}
				}
			}
			W, H := float32(w), float32(690)
			// The main screen, idle and running.
			check("main", 0, W, 0, H, nil)
			tt.Key(0, ui.KeySpace)
			tt.Advance(3 * time.Second)
			check("running", 0, W, 0, H, nil)
			tt.Key(0, ui.KeySpace)
			tt.Advance(time.Second)
			// The panel.
			a.settingsOpen = true
			tt.Frame()
			tt.Advance(2 * time.Second)
			inner := func(s string) bool {
				switch s {
				case "时长", "快速选择", "完成", "专注", "短休息", "长休息", "长休间隔", "经典", "深度", "沉浸",
					"25 分钟", "5 分钟", "15 分钟", "每 4 个", "25/5/15", "50/10/20", "90/20/30":
					return true
				}
				return false
			}
			// Under the scrim the main screen's own texts share some names, so
			// look at the panel's: below the title bar's height.
			check("panel", 10+16, W-10-16, 36, H, func(s string) bool { return inner(s) && s != "专注" && s != "短休息" && s != "长休息" })
			// The rows: the value sits between its buttons and the name.
			for _, v := range []string{"25 分钟", "5 分钟", "15 分钟", "每 4 个"} {
				r, _ := tt.Find(v)
				if r.X+r.W > W-10-16-32-4 {
					t.Errorf("%s: value %q reaches the + button: %+v", name("row"), v, r)
				}
			}
			n, _ := tt.Find("长休间隔")
			v, _ := tt.Find("每 4 个")
			if n.X+n.W > v.X {
				t.Errorf("%s: the name %+v runs into the value %+v", name("name"), n, v)
			}
			// The three chips are equal and inside.
			var prev ui.Rect
			for i, s := range []string{"经典", "深度", "沉浸"} {
				r, _ := tt.Find(s)
				if i > 0 && r.X < prev.X+prev.W {
					t.Errorf("%s: chips overlap: %+v %+v", name("chips"), prev, r)
				}
				prev = r
			}
		}
	}
}

func itoa(n int) string     { return fmt.Sprint(n) }
func ftoa(f float32) string { return fmt.Sprintf("%.2f", f) }
