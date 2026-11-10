package app

import (
	"time"

	"github.com/egoist/mygo"
)

const loadingDelay = 150 * time.Millisecond

// Fast operations finish before their loading hint becomes visible.
type loadingHint struct {
	visible bool
	active  bool
	timer   *time.Timer
}

func newLoadingHint(window *mygo.Window) *loadingHint {
	hint := &loadingHint{active: true}
	hint.timer = time.AfterFunc(loadingDelay, func() {
		window.Update(func() {
			if hint.active {
				hint.visible = true
			}
		})
	})
	return hint
}

func (hint *loadingHint) finish() {
	if hint != nil {
		hint.active = false
		hint.visible = false
		hint.timer.Stop()
	}
}

func (hint *loadingHint) show() bool {
	return hint != nil && hint.visible
}
