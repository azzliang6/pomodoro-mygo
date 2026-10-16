// Pomodoro is a pomodoro timer in native UI: a ring that fills as the
// session goes by, three modes and a notification when a session ends.
// Written with MyGo; the window is drawn by MyGo itself, with no web page.
package main

import (
	_ "embed"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var icon []byte

// app is what the window shows.
type app struct {
	tm           *timer
	themed       bool
	miniThemed   bool
	settingsOpen bool
	held         map[string]*hold
	title        string
	setTitle     func(string)
	geo          ringGeo    // the ring's shapes, built once
	software     bool       // the frame is drawn by the CPU, which the last paint reported
	onMini       func(bool) // main.go changes the windows
	onQuit       func()
	confirm      *ask // the question open, if any
	confirmSeq   uint64
}

func newApp() *app {
	a := &app{tm: newTimer(), held: map[string]*hold{}}
	a.tm.loadSettings(settingsPath())
	a.tm.load(statePath(), time.Now())
	return a
}

// windows are the two windows of the app: the full one, and the small one
// that stays on top, of which only one shows at a time.
type windows struct {
	a    *app
	main *mygo.Window

	mu       sync.Mutex
	mini     *mygo.Window
	leaving  bool // the mini window closes to give way to the full one
	quitting bool
}

func (w *windows) miniWindow() *mygo.Window {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mini
}

// current is the window that shows.
func (w *windows) current() *mygo.Window {
	if m := w.miniWindow(); m != nil {
		return m
	}
	return w.main
}

// title puts the title in both windows: the taskbar shows the time left.
func (w *windows) title(s string) {
	w.main.SetTitle(s)
	if m := w.miniWindow(); m != nil {
		m.SetTitle(s)
	}
}

// refresh builds the views again, from any goroutine.
func (w *windows) refresh(fn func()) {
	w.main.Update(fn)
	if m := w.miniWindow(); m != nil {
		m.Update(func() {})
	}
}

// openMini makes the mini window: borderless, above the other windows, of
// a size the user cannot change. Its place is remembered by MyGo under its
// StateKey; the first time it is at the top right of the screen.
func (w *windows) openMini() {
	if w.miniWindow() != nil {
		return
	}
	x, y := 0, 0
	if d := mygo.Screen.PrimaryDisplay(); d.WorkArea.Width > 0 {
		x = d.WorkArea.X + d.WorkArea.Width - miniW - 24
		y = d.WorkArea.Y + 24
	}
	m := mygo.NewWindow(mygo.WindowOptions{
		Title:             "番茄钟",
		Width:             miniW,
		Height:            miniH,
		X:                 x,
		Y:                 y,
		StateKey:          "mini",
		Frameless:         true,
		AlwaysOnTop:       true,
		DisableResize:     true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		BackgroundColor:   "#0e1014",
		Content:           ui.View(w.a.miniView),
	})
	_ = m.SetIcon(icon)
	// Closing it by the keyboard (Alt+F4) quits the app, which has no
	// other window showing: the full one is hidden.
	m.OnClose(func(*mygo.CloseEvent) {
		w.mu.Lock()
		quit := !w.leaving && !w.quitting
		if quit {
			w.quitting = true
		}
		w.mu.Unlock()
		if quit {
			go mygo.App.Quit()
		}
	})
	w.mu.Lock()
	w.mini = m
	w.mu.Unlock()
}

// toMini hides the full window, which keeps its size and place, and shows
// the mini one.
func (w *windows) toMini() {
	w.openMini()
	w.main.Hide()
}

// toFull shows the full window, as it was, and closes the mini one.
func (w *windows) toFull() {
	w.main.Show()
	w.mu.Lock()
	m := w.mini
	w.mini = nil
	w.leaving = true
	w.mu.Unlock()
	if m != nil {
		m.Close()
	}
	w.mu.Lock()
	w.leaving = false
	w.mu.Unlock()
}

func (w *windows) quit() {
	w.mu.Lock()
	w.quitting = true
	w.mu.Unlock()
	mygo.App.Quit()
}

func main() {
	// The app is dark whatever the system's appearance is: the window's
	// buttons, menus and dialogs follow it.
	mygo.Theme.SetSource(mygo.ThemeDark)
	a := newApp()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "番茄钟",
			Width:     440,
			Height:    690,
			MinWidth:  360,
			MinHeight: 660,
			StateKey:  "main",
			// Hidden while the mini window is what shows.
			Hidden: a.tm.mini,
			// No system title bar: the view draws its own, in the palette of
			// the app, under the window's minimize, maximize and close buttons.
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  barHeight,
			BackgroundColor: "#0e1014",
			Content:         ui.View(a.view),
		})
		_ = win.SetIcon(icon)
		ws := &windows{a: a, main: win}
		a.setTitle = ws.title
		a.onMini = func(v bool) {
			if v {
				ws.toMini()
			} else {
				ws.toFull()
			}
		}
		a.onQuit = ws.quit
		if a.tm.mini {
			ws.openMini()
		}

		tm := a.tm
		tm.saveSettings = func() { tm.writeSettings(settingsPath()) }
		tm.save = func() { tm.write(statePath(), time.Now()) }
		tm.notify = func(title, body string) {
			// Showing a notification may wait on the system: not on the UI thread.
			go func() {
				n := mygo.NewNotification(mygo.NotificationOptions{ID: "pomodoro-done", Title: title, Body: body})
				n.OnClick(func() { ws.current().Focus() })
				if err := n.Show(); err != nil {
					log.Println("notification:", err)
				}
			}()
		}
		// A minimized window draws no frames: a timer ends the session anyway.
		tm.wake = func(d time.Duration) func() {
			t := time.AfterFunc(d, func() {
				ws.refresh(func() { tm.tick(time.Now()) })
			})
			return func() { t.Stop() }
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
