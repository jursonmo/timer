package timer

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jursonmo/timer/ilist"
)

const (
	tvn_bits uint64 = 6
	tvr_bits uint64 = 8
	tvn_size uint64 = 64  //1 << tvn_bits
	tvr_size uint64 = 256 //1 << tvr_bits

	tvn_mask uint64 = 63  //tvn_size - 1
	tvr_mask uint64 = 255 //tvr_size -1
)

const (
//defaultTimerSize = 128
)

var defaultWheel *Wheel

func init() {
	defaultWheel = NewWheel(100 * time.Millisecond)
}

type Wheel struct {
	sync.Mutex
	jiffies    uint64
	timerPool  timerPooler
	timers     int
	taskRuning int32 //记录正在执行timer func 的goroutine 数量

	// tv1        [][]*timer
	// tv2        [][]*timer
	// tv3        [][]*timer
	// tv4        [][]*timer
	// tv5        [][]*timer
	tv1 []ilist.List
	tv2 []ilist.List
	tv3 []ilist.List
	tv4 []ilist.List
	tv5 []ilist.List

	tick time.Duration

	quit chan struct{}
}

type Option func(*Wheel)

func WithTimerPool(p timerPooler) Option {
	return func(w *Wheel) {
		w.timerPool = p
	}
}

//tick is the time for a jiffies
func NewWheel(tick time.Duration, opts ...Option) *Wheel {
	w := new(Wheel)
	for _, opt := range opts {
		opt(w)
	}

	if w.timerPool == nil {
		w.timerPool = NewTimerSyncPool()
	}

	w.quit = make(chan struct{})

	f := func(size int) []ilist.List {
		tv := make([]ilist.List, size)
		return tv
	}

	w.tv1 = f(int(tvr_size))
	w.tv2 = f(int(tvn_size))
	w.tv3 = f(int(tvn_size))
	w.tv4 = f(int(tvn_size))
	w.tv5 = f(int(tvn_size))

	w.jiffies = 0
	w.tick = tick

	go w.run()
	return w
}

func (w *Wheel) Timers() int {
	w.Lock()
	defer w.Unlock()
	return w.timers
}

func (w *Wheel) addTimerInternal(t *timer) {
	expires := t.expires
	idx := t.expires - w.jiffies

	var tv []ilist.List
	var i uint64

	if idx < tvr_size {
		i = expires & tvr_mask
		tv = w.tv1
	} else if idx < (1 << (tvr_bits + tvn_bits)) {
		i = (expires >> tvr_bits) & tvn_mask
		tv = w.tv2
	} else if idx < (1 << (tvr_bits + 2*tvn_bits)) {
		i = (expires >> (tvr_bits + tvn_bits)) & tvn_mask
		tv = w.tv3
	} else if idx < (1 << (tvr_bits + 3*tvn_bits)) {
		i = (expires >> (tvr_bits + 2*tvn_bits)) & tvn_mask
		tv = w.tv4
	} else if int64(idx) < 0 {
		i = w.jiffies & tvr_mask
		tv = w.tv1
	} else {
		if idx > 0x00000000ffffffff {
			idx = 0x00000000ffffffff

			expires = idx + w.jiffies
		}

		i = (expires >> (tvr_bits + 3*tvn_bits)) & tvn_mask
		tv = w.tv5
	}
	tv[i].PushBack(t)
	t.list = &tv[i]
	t.state = NotReady
}

func (w *Wheel) cascade(tv []ilist.List, index int) int {
	var t *timer
	list := tv[index]
	for !list.Empty() {
		e := list.Front()
		list.Remove(e)
		t = e.(*timer)
		w.addTimerInternal(t)
	}
	return index
}

func (w *Wheel) getIndex(n int) int {
	return int((w.jiffies >> (tvr_bits + uint64(n)*tvn_bits)) & tvn_mask)
}

func (w *Wheel) onTick() {
	w.Lock()

	index := int(w.jiffies & tvr_mask)

	if index == 0 && (w.cascade(w.tv2, w.getIndex(0))) == 0 &&
		(w.cascade(w.tv3, w.getIndex(1))) == 0 &&
		(w.cascade(w.tv4, w.getIndex(2))) == 0 &&
		(w.cascade(w.tv5, w.getIndex(3)) == 0) {

	}

	//w.jiffies++
	atomic.AddUint64(&w.jiffies, 1) //w.jiffies有变化时,用atomic.Add, 让其他任务可以在没有加锁的情况下,用atomic.Load来获取最新值。
	for e := w.tv1[index].Front(); e != nil; e = e.Next() {
		t := e.(*timer)
		t.state = Ready
		t.list = nil
		w.timers--
	}
	execList := w.tv1[index]
	w.tv1[index].Reset()
	w.Unlock()

	//检查 w.taskRuning 的合理性,如果w.tick是50ms, 那么w.taskRuning必须等于0，即50ms 内必须定时器必须执行完。
	//如果w.tick是10ms, 那么w.taskRuning 不能大于5, 即允许还有5个任务(goroutine)在执行timer func
	running := atomic.LoadInt32(&w.taskRuning)
	if w.tick*time.Duration(running) > (time.Millisecond * 50) {
		log.Printf("warnning: %d task still running\n", running)
	}

	f := func(list ilist.List) {
		now := time.Now()
		for !list.Empty() {
			e := list.Front()
			list.Remove(e)
			e.Reset()
			t := e.(*timer)
			t.state = Running
			t.f(now, t.arg...)

			if t.period > 0 {
				t.expires = t.period + w.jiffies
				w.addTimer(t)
			}
		}
		atomic.AddInt32(&w.taskRuning, -1)
	}

	if !execList.Empty() {
		atomic.AddInt32(&w.taskRuning, 1)
		go f(execList)
	}
}

func (w *Wheel) addTimer(t *timer) {
	w.Lock()
	w.addTimerInternal(t)
	w.timers++
	w.Unlock()
}

//目前只有stoped和NotReady的状态timer.Stop()才返回true,
// todo:那道理timer excute完后可以Stop() 和 Reset(); 同时timer in sync.Pool 是不能做任何操作的
func (w *Wheel) delTimer(t *timer) bool {
	w.Lock()
	defer w.Unlock()
	if t.state == Stoped {
		return true
	}
	if t.list != nil /*&& t.state == NotReady*/ {
		t.list.Remove(t)
		t.Entry.Reset()
		t.list = nil
		t.state = Stoped
		return true
	}
	return false
}

func (w *Wheel) resetTimer(t *timer, when time.Duration, period time.Duration) bool {
	b := w.delTimer(t)
	if !b {
		return b
	}
	t.expires = atomic.LoadUint64(&w.jiffies) + uint64(when/w.tick)
	t.period = uint64(period / w.tick)

	w.addTimer(t)
	return true
}

func (w *Wheel) newTimer(when time.Duration, period time.Duration,
	f func(time.Time, ...interface{}), arg ...interface{}) *timer {
	//t := new(timer)
	t := w.getTimer()

	t.expires = atomic.LoadUint64(&w.jiffies) + uint64(when/w.tick)
	t.period = uint64(period / w.tick)

	t.f = f
	t.arg = arg

	t.w = w

	return t
}

func (w *Wheel) getTimer() *timer {
	if w.timerPool == nil {
		return new(timer)
	}
	t := w.timerPool.Get()
	//check timer and reset
	if t.list != nil || t.f != nil || t.arg != nil {
		log.Fatalf("timer is not init state")
	}
	if t.state != Stoped && t.state != InPool {
		log.Fatalf("t.state != Stoped && t.state != InPool")
	}
	t.state = Stoped
	return t
}

//并发不安全; todo:按道理只有在Stoped 状态和 timer 执行完的状态才能放回到池里(todo:timer需要加锁并修改状态)
func (w *Wheel) releaseTimer(t *timer) {
	if w.timerPool == nil {
		return
	}
	//check timer
	if t.list != nil {
		log.Fatalf("timer is in wheel, can't be released")
	}
	//init timer
	t.f = nil
	t.arg = nil //gc faster
	t.state = InPool
	w.timerPool.Put(t)
}

//如果没有实现PoolNewCounter,返回 -1
func (w *Wheel) PoolNewCount() int64 {
	if counter, ok := w.timerPool.(PoolNewCounter); ok {
		return counter.PoolNewCount()
	}
	return -1
}

func (w *Wheel) run() {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.onTick()
		case <-w.quit:
			return
		}
	}
}

func (w *Wheel) Stop() {
	close(w.quit)
}

func sendTime(t time.Time, arg ...interface{}) {
	a := arg[0]
	ch := a.(chan time.Time)
	select {
	case ch <- t:
	default:
	}
}

func goFunc(t time.Time, arg ...interface{}) {
	go arg[0].(func())()
}

func dummyFunc(t time.Time, arg interface{}) {

}

func (w *Wheel) After(d time.Duration) <-chan time.Time {
	return w.NewTimer(d).C
}

func (w *Wheel) Sleep(d time.Duration) {
	<-w.NewTimer(d).C
}

func (w *Wheel) Tick(d time.Duration) <-chan time.Time {
	return w.NewTicker(d).C
}

func (w *Wheel) TickFunc(d time.Duration, f func()) *Ticker {
	t := &Ticker{
		r: w.newTimer(d, d, goFunc, f),
	}

	w.addTimer(t.r)

	return t

}

func (w *Wheel) AfterFunc(d time.Duration, f func()) *Timer {
	t := &Timer{
		r: w.newTimer(d, 0, goFunc, f),
	}

	w.addTimer(t.r)

	return t
}

func (w *Wheel) NewTimer(d time.Duration) *Timer {
	c := make(chan time.Time, 1)
	t := &Timer{
		C: c,
		r: w.newTimer(d, 0, sendTime, c),
	}

	w.addTimer(t.r)

	return t
}

func (w *Wheel) NewTicker(d time.Duration) *Ticker {
	c := make(chan time.Time, 1)
	t := &Ticker{
		C: c,
		r: w.newTimer(d, d, sendTime, c),
	}

	w.addTimer(t.r)

	return t
}

//add by mo
func (w *Wheel) NewTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *Timer {
	t := &Timer{
		r: w.newTimer(d, 0, f, arg...),
	}

	w.addTimer(t.r)

	return t
}

//相比NewTimerFunc, 不用创建Timer{}对象，直接创建*timer(*WheelTimer)对象。
func (w *Wheel) NewWheelTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *timer {
	t := w.newTimer(d, 0, f, arg...)
	w.addTimer(t)
	return t
}
