package timer

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
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
	defaultTimerSize = 128
)

var defaultWheel *Wheel

func init() {
	defaultWheel = NewWheel(100 * time.Millisecond)
}

type timer struct {
	expires uint64
	period  uint64

	f   func(time.Time, ...interface{})
	arg []interface{}

	w *Wheel

	vec   []*timer
	index int
}

type Wheel struct {
	sync.Mutex

	jiffies    uint64
	taskRuning int32 //记录只在执行timer func 的goroutine 数量
	tv1        [][]*timer
	tv2        [][]*timer
	tv3        [][]*timer
	tv4        [][]*timer
	tv5        [][]*timer

	tick time.Duration

	quit chan struct{}
}

//tick is the time for a jiffies
func NewWheel(tick time.Duration) *Wheel {
	w := new(Wheel)

	w.quit = make(chan struct{})

	f := func(size int) [][]*timer {
		tv := make([][]*timer, size)
		for i := range tv {
			tv[i] = make([]*timer, 0, defaultTimerSize)
		}

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

func (w *Wheel) addTimerInternal(t *timer) {
	expires := t.expires
	idx := t.expires - w.jiffies

	var tv [][]*timer
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

	tv[i] = append(tv[i], t) //bug:这里可能会分配新的内存，原来的timer.vec 会指向老的内存。导致delTimer无法删除指定timer. example/batch.go 就证明了这个bug

	t.vec = tv[i] //如果append分配新的内存，这里的t.vec 会指向老的内存。用sliec的方式保存timer,删除时就不方便了。还是用链表吧
	t.index = len(tv[i]) - 1
}

func (w *Wheel) cascade(tv [][]*timer, index int) int {
	vec := tv[index]
	tv[index] = vec[0:0:defaultTimerSize]

	for _, t := range vec {
		//w.addTimerInternal(t) //fixbug: 如果vec里的timer 删除了, t 可能是nil, 所以这里需要判断t是否为nil
		if t != nil {
			w.addTimerInternal(t)
		} else {
			log.Printf("t != nil \n")
		}
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

	//vec := w.tv1[index]
	//这样处理是有问题的，如果w.tv1这时有timer 加入，用的内存还是vec, 也就是go f(vec) 可能执行新的timer？
	//不太可能,现在w.jiffies++了, 这个w.tv1[index] 要等到轮一圈后才会用到, 除非go f(vec) 一圈后还没执行完
	//w.tv1[index] = vec[0:0:defaultTimerSize]
	vec := make([]*timer, len(w.tv1[index]))
	copy(vec[:], w.tv1[index])

	//置空w.tv1[index], 1. 为了更快gc, 2. 为了time.Stop() 可以正确返回true
	for i := 0; i < len(w.tv1[index]); i++ {
		if w.tv1[index][i] != nil {
			w.tv1[index][i].vec = nil
			w.tv1[index][i].index = -1
			w.tv1[index][i] = nil
		}
	}
	w.tv1[index] = w.tv1[index][0:0:defaultTimerSize]

	w.Unlock()

	//检查 w.taskRuning 的合理性,如果w.tick是50ms, 那么w.taskRuning必须等于0，即50ms 内必须定时器必须执行完。
	//如果w.tick是10ms, 那么w.taskRuning 不能大于5, 即允许还有5个任务(goroutine)在执行timer func
	running := atomic.LoadInt32(&w.taskRuning)
	if w.tick*time.Duration(running) > (time.Millisecond * 50) {
		log.Printf("warnning: %d task still running\n", running)
	}

	f := func(vec []*timer) {
		now := time.Now()
		for _, t := range vec {
			if t == nil {
				continue
			}

			t.f(now, t.arg...)

			if t.period > 0 {
				t.expires = t.period + w.jiffies
				w.addTimer(t)
			}
		}
		atomic.AddInt32(&w.taskRuning, -1)
	}

	if len(vec) > 0 {
		atomic.AddInt32(&w.taskRuning, 1)
		go f(vec)
	}
}

func (w *Wheel) addTimer(t *timer) {
	w.Lock()
	w.addTimerInternal(t)
	w.Unlock()
}

func (w *Wheel) delTimer(t *timer) bool {
	w.Lock()
	defer w.Unlock()
	if t.index == -1 {
		return false
	}
	vec := t.vec
	index := t.index

	if vec != nil && len(vec) > index && vec[index] == t {
		vec[index] = nil
		t.vec = nil //尽快解除引用
		t.index = -1
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
	t := new(timer)

	t.expires = atomic.LoadUint64(&w.jiffies) + uint64(when/w.tick)
	t.period = uint64(period / w.tick)

	t.f = f
	t.arg = arg

	t.w = w

	return t
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
