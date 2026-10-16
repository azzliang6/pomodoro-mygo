package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// An ask is a question the app puts before an action that would throw away
// the session in progress: switching to another mode, skipping to the next
// session. Nothing changes until the user says yes.
type ask struct {
	title, body, ok string
	short           string // the body in the mini window, in one line
	do              func()
	doneSeq         uint64 // the session count when asked: a session that ended meanwhile cancels it
}

// ruins tells whether leaving the session now would lose some progress: it
// is running, or paused part of the way.
func (t *timer) ruins() bool { return !t.fresh() }

// askFirst runs do at once when nothing would be lost, and otherwise asks.
func (a *app) askFirst(title, body, ok string, do func()) {
	if !a.tm.ruins() {
		do()
		return
	}
	a.confirm = &ask{title: title, body: body, ok: ok, do: do, doneSeq: a.tm.doneSeq, short: "将重置当前进度"}
	a.confirmSeq++
}

// switchMode asks before another tab takes the session over.
func (a *app) switchMode(m int) {
	if m == a.tm.mode {
		return
	}
	a.askFirst("切换到「"+modes[m].Name+"」？", "切换将重置当前进度，是否继续？", "切换", func() { a.tm.choose(m) })
}

// skipSession asks before the session in progress is given up for the next.
func (a *app) skipSession() {
	next := a.tm.next()
	a.askFirst("跳过这一轮？", fmt.Sprintf("跳过将重置当前进度，直接进入「%s」，是否继续？", modes[next].Name), "确定跳过", a.tm.skip)
}

// settleAsk drops a question that no longer holds and answers one with the
// keys: Enter says yes, Escape no. It reports whether a question is open,
// so that Escape does not also close the settings.
func (a *app) settleAsk(c *ui.Context) bool {
	if a.confirm == nil {
		return false
	}
	if a.confirm.doneSeq != a.tm.doneSeq || !a.tm.ruins() {
		a.confirm = nil // the session ended, or was reset
		return false
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.confirm = nil
	} else if c.Shortcut(0, ui.KeyEnter) {
		q := a.confirm
		a.confirm = nil
		q.do()
	}
	return true
}

// confirmDialog draws the open question over the window: a veil that takes
// the clicks, and a card with two buttons. compact is for the mini window.
func (a *app) confirmDialog(c *ui.Context, accent ui.Color, now time.Time, compact bool) {
	q := a.confirm
	if q == nil {
		return
	}
	veil := ui.Column(c.Key("veil")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).
		Background(ui.RGBA(0, 0, 0, 0.6)).Padding(0, 14, 0, 14)
	veil.Opacity(veil.Keyframes("in", []ui.Keyframe{
		{Value: 0},
		{At: 140 * time.Millisecond, Value: 1, Ease: ui.EaseOut},
	}, ui.KeyframeOptions{Run: a.confirmSeq}))
	veil.Children(func() {
		card := ui.Column(c.Key("card")).Radius(18).Background(colSurface).Border(1, colEdge).AlignItems(ui.Stretch).Gap(14).
			Padding(20, 20, 18, 20)
		title, body, btnH, fs := sz(18), sz(13), float32(44), float32(15)
		if compact {
			card = card.Radius(12).Gap(6).Padding(9, 12, 9, 12).Width(miniW - 28).MinWidth(0).Shrink(1)
			title, body, btnH, fs = sz(13), sz(11), 26, 12
		} else {
			card = card.Width(340).MinWidth(0).Shrink(1)
			if !a.software {
				card.Shadow(0, 20, 50, 0, ui.RGBA(0, 0, 0, 0.5))
			}
		}
		card.Children(func() {
			ui.Text(c, q.title).FontSize(title).Bold().TextColor(colText)
			text := q.body
			if compact {
				text = q.short
			}
			ui.Text(c, text).FontSize(body).TextColor(colMuted)
			ui.Row(c.Key("answers")).Gap(10).AlignItems(ui.Stretch).Padding(compact2(compact), 0, 0, 0).Children(func() {
				if a.button(c, accent, now, btn{key: "ask-no", label: "取消", h: btnH, bg: colRaised, fg: colMuted, fgHover: colText, fs: fs}) {
					a.confirm = nil
				}
				if a.button(c, accent, now, btn{key: "ask-yes", label: q.ok, h: btnH, bg: accent, fg: colInk, fgHover: colInk, fs: fs, primary: !compact}) {
					a.confirm = nil
					q.do()
				}
			})
		})
	})
}

func compact2(compact bool) float32 {
	if compact {
		return 0
	}
	return 6
}
