package timer

import (
	"runtime"
	"time"
)

type wheel_shard struct {
	wheels []*Wheel
}

var defaultWheelShard *wheel_shard

func init() {
	defaultWheelShard = NewWheelShard(100 * time.Millisecond)
}

func StopDefaultWheelShard() {
	defaultWheelShard.Stop()
}

func (ws *wheel_shard) Stop() {
	for i := 0; i < len(ws.wheels); i++ {
		ws.wheels[i].Stop()
	}
}

func NewWheelShard(tick time.Duration, opts ...Option) *wheel_shard {
	pnum := runtime.GOMAXPROCS(0)
	wheels := make([]*Wheel, pnum)
	for i := 0; i < len(wheels); i++ {
		wheels[i] = NewWheel(tick, opts...)
	}

	return &wheel_shard{wheels: wheels}
}

//GetPid() 获取MPG 的Pid, 从而获取对应的wheel, 但是在wheel addTimer() 还是要加锁
//因为当前goroutine 是可能被调度到P处理,任何goroutine代码都需要在P上运行，P的数量觉得程序的并行数量,
//也就是可能有两个不同M下的goroutine, 同时操作同一个wheel, 所以wheel 的操作需要加锁
// wheel_shard 只是尽量减少加锁时的竞争而已，不能完全避免锁竞争。
func (ws *wheel_shard) NewWSTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *WheelTimer {
	pid := GetPid()
	return ws.wheels[pid].NewWheelTimerFunc(d, f, arg...)
}

//ticker
func (ws *wheel_shard) NewTicker(d time.Duration) *Ticker {
	pid := GetPid()
	return ws.wheels[pid].NewTicker(d)
}

func (ws *wheel_shard) TickFunc(d time.Duration, f func()) *Ticker {
	pid := GetPid()
	return ws.wheels[pid].TickFunc(d, f)
}

func (ws *wheel_shard) Tick(d time.Duration) <-chan time.Time {
	pid := GetPid()
	return ws.wheels[pid].Tick(d)
}

// Timer
func (ws *wheel_shard) After(d time.Duration) <-chan time.Time {
	pid := GetPid()
	return ws.wheels[pid].After(d)
}

func (ws *wheel_shard) Sleep(d time.Duration) {
	pid := GetPid()
	ws.wheels[pid].Sleep(d)
}

func (ws *wheel_shard) AfterFunc(d time.Duration, f func()) *Timer {
	pid := GetPid()
	return ws.wheels[pid].AfterFunc(d, f)
}

func (ws *wheel_shard) NewTimer(d time.Duration) *Timer {
	pid := GetPid()
	return ws.wheels[pid].NewTimer(d)
}

func (ws *wheel_shard) NewTimerFunc(d time.Duration, callback func(time.Time, ...interface{}), arg ...interface{}) *Timer {
	pid := GetPid()
	return ws.wheels[pid].NewTimerFunc(d, callback, arg...)
}

//WheelTimer
func (ws *wheel_shard) Timers() int {
	n := 0
	for i := 0; i < len(ws.wheels); i++ {
		n += ws.wheels[i].Timers()
	}
	return n
}

func (ws *wheel_shard) NewWheelTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *WheelTimer {
	pid := GetPid()
	return ws.wheels[pid].NewWheelTimerFunc(d, f, arg...)
}
