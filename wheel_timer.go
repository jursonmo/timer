package timer

import (
	"fmt"
	"time"

	"github.com/jursonmo/timer/ilist"
)

const (
	Stoped   = 0
	NotReady = 1
	Ready    = 2
	Running  = 3
	InPool   = 4
)

/*				addtimer									release         Get()
//init(Stoped)----------->NotReady ---> Ready --->Running ---------->InPool------->Stoped
							|
			 <--------------|
			    deltimer
*/
//目前只有stoped和NotReady的状态timer.Stop()才返回true, 这样才能ResetTimer
/*
	if t.Stop() {
		t.ResetTimer(d, period)
	}
*/

/*
```go
	1.
	if t.Stop() {
		t.Release() //timer 已经Stop,可以Release
	}
	2.
	callback = func(time.Time, args ...interface{}) {
		t.Release() //timer 的callback已经执行，可以Release
	}
```
*/
// todo:按道理timer excute完后可以Stop() 和 Reset(); 同时timer in sync.Pool 是不能做任何操作的

type WheelTimer = timer
type timer struct {
	ilist.Entry
	list *ilist.List
	w    *Wheel

	expires uint64
	period  uint64
	state   int
	f       func(time.Time, ...interface{})
	arg     []interface{}
}

func Timers() int {
	return defaultWheel.Timers()
}

func NewWheelTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *WheelTimer {
	return defaultWheel.NewWheelTimerFunc(d, f, arg...)
}

func (t *timer) Stop() bool {
	return t.w.delTimer(t)
}

func (t *timer) ResetTimer(d time.Duration, period time.Duration) bool {
	return t.w.resetTimer(t, d, period)
}

func (t *timer) Release() {
	t.w.releaseTimer(t)
}

func (t *timer) Info() string {
	return fmt.Sprintf("expires:%d, period:%d, args:%v", t.expires, t.period, t.arg)
}
