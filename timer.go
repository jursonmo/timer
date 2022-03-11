package timer

import (
	"fmt"
	"time"
)

type Timer struct {
	C <-chan time.Time
	r *timer
}

func After(d time.Duration) <-chan time.Time {
	return defaultWheel.After(d)
}

func Sleep(d time.Duration) {
	defaultWheel.Sleep(d)
}

func AfterFunc(d time.Duration, f func()) *Timer {
	return defaultWheel.AfterFunc(d, f)
}

func NewTimer(d time.Duration) *Timer {
	return defaultWheel.NewTimer(d)
}

func (t *Timer) Reset(d time.Duration) bool {
	return t.r.w.resetTimer(t.r, d, 0)
}

func (t *Timer) Stop() bool {
	return t.r.w.delTimer(t.r)
}

func NewTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *Timer {
	return defaultWheel.NewTimerFunc(d, f, arg...)
}

func (t *Timer) Info() string {
	return fmt.Sprintf("expires:%d, period:%d, args:%v", t.r.expires, t.r.period, t.r.arg)
}
