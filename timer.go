package timer

import (
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
	//return t.r.w.resetTimer(t.r, d, 0)
	return t.r.ResetTimer(d, 0)
}

func (t *Timer) Stop() bool {
	//return t.r.w.delTimer(t.r)
	return t.r.Stop()
}

func (t *Timer) Release() {
	//t.r.w.releaseTimer(t.r)
	t.r.Release()
}

// 需要传入callback的接口，应该用NewWheelTimerFunc 接口, 而不是NewTimerFunc
func NewTimerFunc(d time.Duration, callback func(time.Time, ...interface{}), arg ...interface{}) *Timer {
	return defaultWheel.NewTimerFunc(d, callback, arg...)
}

func (t *Timer) Info() string {
	return t.r.Info()
}
