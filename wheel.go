package timer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jursonmo/timer/ilist"
	"github.com/jursonmo/timer/log"
)

const (
	tvn_bits uint64 = 6
	tvr_bits uint64 = 8
	tvn_size uint64 = 64  //1 << tvn_bits
	tvr_size uint64 = 256 //1 << tvr_bits

	tvn_mask uint64 = 63  //tvn_size - 1
	tvr_mask uint64 = 255 //tvr_size -1

	maxTimerCbTake = 10 * time.Millisecond
)

const (
// defaultTimerSize = 128
)

var defaultWheel *Wheel

func init() {
	//defaultWheel = NewWheel(100 * time.Millisecond)
}

/*
	add by mo:

List              e               e               e
+----+          +----+          +----+          +----+
|head|--------->|next|--------->|next|--------->|next|--->nil
|    |    nil<--|prev|<---------|prev|<---------|prev|
|tail|-->       +----+          +----+          +----+
+----+  |                                          ^
//      ------------------------------------------>|
*/
type Wheel struct {
	sync.Mutex
	name string
	log  log.Logger
	pad  [7]uint64 //avoid share false ?
	//pad   [cpu.CacheLinePadSize - unsafe.Sizeof(sync.Mutex)%cpu.CacheLinePadSize]byte
	jiffies    uint64 //jiffies atomic 读比较多，写比较少，很多读的时候其实不需要同步，但是跟sync.Mutex组成了cacheline
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

	quit  chan struct{}
	close bool
}

type Option func(*Wheel)

func WithName(name string) Option {
	return func(w *Wheel) {
		w.name = name
	}
}

func WithTimerPool(p timerPooler) Option {
	return func(w *Wheel) {
		w.timerPool = p
	}
}
func WithLogger(l log.Logger) Option {
	return func(w *Wheel) {
		w.log = l
	}
}

func (w *Wheel) String() string {
	return fmt.Sprintf("wheel:%s, tick:%v, timers:%v, taskRuning:%d, close:%v", w.name, w.tick, w.Timers(), atomic.LoadInt32(&w.taskRuning), w.close)
}

// tick is the time for a jiffies
func NewWheel(tick time.Duration, opts ...Option) *Wheel {
	w := new(Wheel)
	for _, opt := range opts {
		opt(w)
	}
	if w.log == nil {
		w.log = log.DefaultLog
	}
	if w.timerPool == nil {
		w.timerPool = NewTimerSyncPool()
	}
	if w.name == "" {
		w.name = fmt.Sprintf("create at %v", time.Now())
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

func (w *Wheel) RealTimers() int {
	w.Lock()
	defer w.Unlock()
	timersInWheel := 0
	f := func(lists []ilist.List) int {
		n := 0
		for _, list := range lists {
			for e := list.Front(); e != nil; e = e.Next() {
				n++
			}
		}
		return n
	}
	timersInWheel += f(w.tv5)
	timersInWheel += f(w.tv4)
	timersInWheel += f(w.tv3)
	timersInWheel += f(w.tv2)
	timersInWheel += f(w.tv1)
	return timersInWheel
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
	list := &tv[index]
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
		w.log.Warnf("warnning: %d task still running\n", running)
	}

	f := func(list ilist.List) {
		for !list.Empty() {
			e := list.Front()
			list.Remove(e)
			e.Reset()
			t := e.(*timer)
			t.state = Running
			start := time.Now()
			t.f(start, t.arg...)

			//check the time of the callback taken
			if take := time.Since(start); take > maxTimerCbTake {
				w.log.Warnf("timer:%s cb run take:%v, over maxTimerCbTake:%v", t, take, maxTimerCbTake)
			}
			if t.period > 0 {
				t.expires = t.period + atomic.LoadUint64(&w.jiffies)
				if !w.addTimer(t) {
					w.log.Errorf("add period timer:%+v fail", t)
				}
			}
		}
		atomic.AddInt32(&w.taskRuning, -1)
	}

	if !execList.Empty() {
		atomic.AddInt32(&w.taskRuning, 1)
		go f(execList)
	}
}

func (w *Wheel) addTimer(t *timer) bool {
	w.Lock()
	if t.list != nil {
		//w.Unlock()
		//return false
		w.log.Fatalf("repeat addTimer? timer still in wheel")
	}
	w.addTimerInternal(t)
	w.timers++
	w.Unlock()
	return true
}

// 目前只有stoped和NotReady的状态timer.Stop()才返回true,
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
		w.timers-- //主动删除timer时，需要减少timers
		return true
	}
	return false
}

func (w *Wheel) resetTimer(t *timer, when time.Duration, period time.Duration) bool {
	ok := w.delTimer(t)
	if !ok {
		return false
	}
	t.expires = atomic.LoadUint64(&w.jiffies) + uint64(when/w.tick)
	t.period = uint64(period / w.tick)

	return w.addTimer(t)
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
		w.log.Fatalf("timer is not init state")
	}
	if t.state != Stoped && t.state != FromPool {
		w.log.Fatalf("t.state != Stoped && t.state != FromPool")
	}
	t.state = Stoped
	return t
}

// 并发不安全; todo:按道理只有在Stoped 状态和 timer 执行完的状态才能释放(放回到池里).(todo:timer需要加锁并修改状态)
func (w *Wheel) releaseTimer(t *timer) {
	if w.timerPool == nil {
		return
	}
	//check timer
	if t.list != nil {
		w.log.Fatalf("timer is in wheel, can't be released")
	}
	if !t.Entry.IsInit() {
		w.log.Fatalf("timer haven't executed")
	}
	//init timer
	t.f = nil
	t.arg = nil //gc faster
	t.state = InPool
	w.timerPool.Put(t)
}

// 如果没有实现PoolNewCounter,返回 -1
func (w *Wheel) PoolNewCount() int64 {
	if counter, ok := w.timerPool.(PoolNewCounter); ok {
		return counter.PoolNewCount()
	}
	return -1
}

func (w *Wheel) run() {
	defer w.log.Infof("Wheel quit, %v", w)
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
	w.close = true
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

	if w.addTimer(t.r) {
		return t
	}

	return nil

}

func (w *Wheel) AfterFunc(d time.Duration, f func()) *Timer {
	t := &Timer{
		r: w.newTimer(d, 0, goFunc, f),
	}

	if w.addTimer(t.r) {
		return t
	}

	return nil
}

func (w *Wheel) NewTimer(d time.Duration) *Timer {
	c := make(chan time.Time, 1)
	t := &Timer{
		C: c,
		r: w.newTimer(d, 0, sendTime, c),
	}

	if w.addTimer(t.r) {
		return t
	}

	return nil
}

func (w *Wheel) NewTicker(d time.Duration) *Ticker {
	c := make(chan time.Time, 1)
	t := &Ticker{
		C: c,
		r: w.newTimer(d, d, sendTime, c),
	}

	if w.addTimer(t.r) {
		return t
	}

	return nil
}

// add by mo
func (w *Wheel) NewTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *Timer {
	t := &Timer{
		r: w.newTimer(d, 0, f, arg...),
	}

	if w.addTimer(t.r) {
		return t
	}

	return nil
}

// 相比NewTimerFunc, 不用创建Timer{}对象，直接创建*timer(*WheelTimer)对象。
func (w *Wheel) NewWheelTimerFunc(d time.Duration, f func(time.Time, ...interface{}), arg ...interface{}) *timer {
	t := w.newTimer(d, 0, f, arg...)
	if w.addTimer(t) {
		return t
	}

	return nil
}
