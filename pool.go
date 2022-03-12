package timer

import (
	"sync"
	"sync/atomic"
)

type PoolNewCounter interface {
	PoolNewCount() int64
}

type timerPooler interface {
	//PoolNewCount() int64
	Get() *WheelTimer
	Put(*WheelTimer)
}

type timerSyncPool struct {
	newCount int64
	pool     sync.Pool
}

func NewTimerSyncPool() timerPooler {
	tp := &timerSyncPool{}
	tp.pool = sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&tp.newCount, 1)
			return new(WheelTimer)
		},
	}
	return tp
}

func (tp *timerSyncPool) Get() *WheelTimer {
	return tp.pool.Get().(*WheelTimer)
}
func (tp *timerSyncPool) Put(t *WheelTimer) {
	tp.pool.Put(t)
}

func (tp *timerSyncPool) PoolNewCount() int64 {
	return atomic.LoadInt64(&tp.newCount)
}
