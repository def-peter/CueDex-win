package app

import (
	"log"

	"github.com/egoist/mygo"
)

// notifier shows a desktop notification. A test supplies its own, so a failure
// can be checked without the notification area and without a window.
type notifier interface {
	Show(title, body string) error
}

// toast puts a notification in the notification area through MyGo.
type toast struct{}

func (toast) Show(title, body string) error {
	return mygo.NewNotification(mygo.NotificationOptions{
		// One ID, so a second failure replaces the first instead of stacking.
		ID:     "cuedex-problem",
		Title:  title,
		Body:   body,
		Silent: true,
	}).Show()
}

// notify reports a failure outside the panel. The panel is usually closed, and
// a click on the tray icon is a poor place to find out that an integration
// never took effect.
func (a *App) notify(p problem) {
	if p.empty() || a.toasts == nil {
		return
	}
	body := p.text(a.state.text())
	go func() {
		if err := a.toasts.Show("CueDex", body); err != nil {
			log.Printf("cuedex: notification: %v", err)
		}
	}()
}
