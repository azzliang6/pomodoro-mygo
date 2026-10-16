package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A mode is one kind of session of the technique.
type mode struct {
	Name string // shown in the tab and above the time
}

const (
	modeFocus = iota
	modeShort
	modeLong
)

var modes = [3]mode{{"专注"}, {"短休息"}, {"长休息"}}

// settings are what the user can change: the length of each session in
// minutes, and how many pomodoros earn a long break.
type settings struct {
	Focus int `json:"focus"`
	Short int `json:"short"`
	Long  int `json:"long"`
	Every int `json:"every"`
}

var defaultSettings = settings{Focus: 25, Short: 5, Long: 15, Every: 4}

// presets are the quick choices of the settings panel.
var presets = []struct {
	Name string
	Set  settings
}{
	{"经典", settings{25, 5, 15, 4}},
	{"深度", settings{50, 10, 20, 4}},
	{"沉浸", settings{90, 20, 30, 3}},
}

// limits of each length, in minutes, and of the rounds per cycle.
var (
	minutesMin = [3]int{1, 1, 1}
	minutesMax = [3]int{90, 30, 60}
)

const (
	everyMin = 2
	everyMax = 8
)

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

// fixed brings every field of s into its limits.
func (s settings) fixed() settings {
	s.Focus = clamp(s.Focus, minutesMin[0], minutesMax[0])
	s.Short = clamp(s.Short, minutesMin[1], minutesMax[1])
	s.Long = clamp(s.Long, minutesMin[2], minutesMax[2])
	s.Every = clamp(s.Every, everyMin, everyMax)
	return s
}

func (s settings) minutes(m int) int { return [3]int{s.Focus, s.Short, s.Long}[m] }

func (s *settings) setMinutes(m, v int) {
	v = clamp(v, minutesMin[m], minutesMax[m])
	switch m {
	case modeFocus:
		s.Focus = v
	case modeShort:
		s.Short = v
	default:
		s.Long = v
	}
}

// timer is the app's state: which session it is on, whether it runs, and
// what is done today. It knows nothing of the interface; every method that
// depends on time takes it as an argument, so tests drive it with a clock.
type timer struct {
	set     settings
	mode    int
	running bool
	endAt   time.Time     // when the running session ends
	remain  time.Duration // what is left while it does not run
	span    time.Duration // the length of the session in progress

	sessions int // pomodoros finished today
	cycle    int // pomodoros finished since the last long break
	day      string

	// doneSeq counts finished sessions, to replay the ring's pulse.
	doneSeq uint64

	// hooks, set by main: a notification, saving the state and the
	// settings, and waking the
	// window when the session ends while no frame runs (a minimized window).
	notify       func(title, body string)
	save         func()
	saveSettings func()
	mini         bool // the window is the small one that stays on top
	wake         func(d time.Duration) (stop func())
	stop         func()
}

func newTimer() *timer {
	t := &timer{set: defaultSettings}
	t.remain = t.length(modeFocus)
	t.span = t.remain
	return t
}

// unit is what one "minute" of the settings lasts; tests make it a second.
var unit = time.Minute

// length is how long a session of a mode lasts, by the settings.
func (t *timer) length(m int) time.Duration {
	return time.Duration(t.set.minutes(m)) * unit
}

// total is the length of the session in progress. It stays what it was
// when the session began, so that changing a setting does not make the
// ring jump.
func (t *timer) total() time.Duration { return t.span }

// fresh tells whether the session waits at its full length.
func (t *timer) fresh() bool { return !t.running && t.remain == t.span }

// configure takes new settings. A session that waits untouched shows the
// new length at once; one running or paused keeps its own until the next
// reset or session.
func (t *timer) configure(s settings) {
	s = s.fixed()
	if s == t.set {
		return
	}
	fresh := t.fresh()
	t.set = s
	if fresh {
		t.remain = t.length(t.mode)
		t.span = t.remain
	}
	if t.saveSettings != nil {
		t.saveSettings()
	}
}

// adjust changes the length of a mode by delta minutes.
func (t *timer) adjust(m, delta int) {
	s := t.set
	s.setMinutes(m, s.minutes(m)+delta)
	t.configure(s)
}

// canAdjust tells whether the length of a mode can move by delta.
func (t *timer) canAdjust(m, delta int) bool {
	v := t.set.minutes(m) + delta
	return v >= minutesMin[m] && v <= minutesMax[m]
}

// left is the time left at now.
func (t *timer) left(now time.Time) time.Duration {
	if !t.running {
		return t.remain
	}
	return max(0, t.endAt.Sub(now))
}

// progress is the part of the session that is over, from 0 to 1.
func (t *timer) progress(now time.Time) float32 {
	return float32(1 - float64(t.left(now))/float64(t.total()))
}

func (t *timer) cancelWake() {
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
}

// start runs the session, or resumes it.
func (t *timer) start(now time.Time) {
	if t.running {
		return
	}
	if t.remain <= 0 {
		t.remain = t.length(t.mode)
		t.span = t.remain
	}
	t.running = true
	t.endAt = now.Add(t.remain)
	t.cancelWake()
	if t.wake != nil {
		t.stop = t.wake(t.remain)
	}
}

// pause holds the session where it is.
func (t *timer) pause(now time.Time) {
	if !t.running {
		return
	}
	t.remain = t.left(now)
	t.running = false
	t.cancelWake()
}

// toggle is the Space key and the big button.
func (t *timer) toggle(now time.Time) {
	if t.running {
		t.pause(now)
	} else {
		t.start(now)
	}
}

// reset puts the current session back to its full length.
func (t *timer) reset() {
	t.running = false
	t.cancelWake()
	t.remain = t.length(t.mode)
	t.span = t.remain
}

// choose switches to a mode, stopping the session in progress.
func (t *timer) choose(m int) {
	t.mode = m
	t.reset()
}

// next is the mode that follows the current one.
func (t *timer) next() int {
	if t.mode != modeFocus {
		return modeFocus
	}
	if t.cycle > 0 && t.cycle%t.set.Every == 0 {
		return modeLong
	}
	return modeShort
}

// skip moves to the next session without counting this one.
func (t *timer) skip() { t.choose(t.next()) }

// tick ends the session when its time is up. It is safe to call every frame.
func (t *timer) tick(now time.Time) {
	if !t.running || now.Before(t.endAt) {
		return
	}
	finished := t.mode
	if finished == modeFocus {
		t.sessions++
		t.cycle++
	}
	t.doneSeq++
	t.running = false
	t.cancelWake()
	t.mode = t.next()
	t.remain = t.length(t.mode)
	t.span = t.remain
	if t.notify != nil {
		switch finished {
		case modeFocus:
			if t.mode == modeLong {
				t.notify("专注完成", fmt.Sprintf("这一轮完成了，休息 %d 分钟吧。", t.set.Long))
			} else {
				t.notify("专注完成", fmt.Sprintf("做得好，休息 %d 分钟吧。", t.set.Short))
			}
		default:
			t.notify("休息结束", "准备好开始下一个番茄了吗？")
		}
	}
	if t.save != nil {
		t.save()
	}
}

// filled is how many of the four dots are lit.
func (t *timer) filled() int {
	n := t.cycle % t.set.Every
	if n == 0 && t.cycle > 0 && t.mode == modeLong {
		n = t.set.Every
	}
	return n
}

// stored is what survives a restart: today's count.
type stored struct {
	Day      string `json:"day"`
	Sessions int    `json:"sessions"`
	Cycle    int    `json:"cycle"`
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pomodoro-mygo", "state.json")
}

func today(now time.Time) string { return now.Format("2006-01-02") }

// load restores the count of today.
func (t *timer) load(path string, now time.Time) {
	t.day = today(now)
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s stored
	if json.Unmarshal(b, &s) != nil || s.Day != t.day {
		return
	}
	t.sessions, t.cycle = s.Sessions, s.Cycle
}

// write saves the count of today.
func (t *timer) write(path string, now time.Time) {
	if path == "" {
		return
	}
	b, _ := json.Marshal(stored{Day: today(now), Sessions: t.sessions, Cycle: t.cycle})
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pomodoro-mygo", "settings.json")
}

// loadSettings restores the settings; without a file, or with a damaged
// one, the defaults stay.
func (t *timer) loadSettings(path string) {
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	f := file{settings: defaultSettings}
	if json.Unmarshal(b, &f) != nil {
		return
	}
	t.set, t.mini = f.settings.fixed(), f.Mini
	t.remain = t.length(t.mode)
	t.span = t.remain
}

// file is what settings.json holds: the settings, flat, and whether the
// window was the mini one.
type file struct {
	settings
	Mini bool `json:"mini"`
}

// setMini remembers that the window is, or is not, the mini one.
func (t *timer) setMini(v bool) {
	if t.mini == v {
		return
	}
	t.mini = v
	if t.saveSettings != nil {
		t.saveSettings()
	}
}

// writeSettings saves the settings.
func (t *timer) writeSettings(path string) {
	if path == "" {
		return
	}
	b, _ := json.MarshalIndent(file{t.set, t.mini}, "", "  ")
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}
