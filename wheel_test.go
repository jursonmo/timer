package timer

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestTimer(t *testing.T) {
	tick := 1 * time.Millisecond
	var w = NewWheel(tick)
	d := 10 * time.Millisecond
	timer := w.NewTimer(d)

	ctx, _ := context.WithTimeout(context.Background(), d+2*tick)
	//ctx 比timer 的timeout 时间多两个tick, 按道理timer 先超时
	select {
	case <-ctx.Done():
		t.Fatalf("") //在低负载的情况下，如果ctx 先超时,说明timer 功能不正常
	case <-timer.C:
		break
	}
	w.Stop()
}

func TestTicker(t *testing.T) {
	var testWheel = NewWheel(1 * time.Millisecond)
	wait := make(chan struct{}, 100)
	i := 0
	f := func() {
		println(time.Now().Unix())
		i++
		if i >= 5 {
			wait <- struct{}{}
		}
	}
	before := time.Now()

	t1 := testWheel.TickFunc(1000*time.Millisecond, f)

	<-wait

	t1.Stop()

	after := time.Now()

	println(after.Sub(before).String())
	testWheel.Stop()
}

/*
//可以调用两次timer.Stop() 都返回true
func TestRepeatStopTimer(t *testing.T) {
	w := NewWheel(1 * time.Millisecond)
	timer := w.NewTimer(500 * time.Millisecond)

	if !timer.Stop() {
		t.Fatalf("t.Stop() fail")
	}
	if timer.Stop() {
		t.Fatalf("shouldn't repeat Stop() timer")
	}
	w.Stop()
}
*/
func TestResetTimer(t *testing.T) {
	tick := 1 * time.Millisecond
	w := NewWheel(tick)
	d := 10 * time.Millisecond
	timer := w.NewTimer(d)
	if !timer.Stop() {
		t.Fatalf("timer.Stop() fail")
	}
	//should Stop timer before Reset()
	if !timer.Reset(d) {
		t.Fatalf("timer.Reset() fail")
	}
	time.Sleep(d + 2*tick) //wait and timer should timeout and excuted
	if timer.Reset(d) {
		t.Fatalf("timer executed, timer.Reset() should be failed")
	}
	w.Stop()
}

func TestTimers(t *testing.T) {
	w := NewWheel(1 * time.Millisecond)
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	n := 10000
	for i := 0; i < n; i++ {
		delay := 500 + r.Intn(300)
		w.NewTimer(time.Duration(delay) * time.Millisecond)
	}
	if timers := w.Timers(); timers != n {
		t.Fatalf("add %d timer, but there are %d timer in wheel\n", n, timers)
	}

	time.Sleep(time.Second)
	if timers := w.Timers(); timers != 0 {
		t.Fatalf("all timer should have executed, but there are %d timer in wheel\n", timers)
	}
	w.Stop()
}

//go test *.go -test.run TestStopTimer
func TestStopTimer(t *testing.T) {
	var testWheel = NewWheel(1 * time.Millisecond)
	//n := 3000
	n := 1000
	var mu sync.Mutex
	stopTimerMap := make(map[int]*Timer)
	timerMap := make(map[int]*Timer)
	f := func(_ time.Time, args ...interface{}) {
		mu.Lock()
		index := args[0].(int)
		st, ok := stopTimerMap[index]
		if ok {
			t.Fatalf("index:%d timer:%s has Stop, but still exec", index, st.Info())
		}
		delete(timerMap, index)
		mu.Unlock()
	}
	for i := 0; i < n; i++ {
		t := testWheel.NewTimerFunc(time.Millisecond*500, f, i)
		mu.Lock()
		timerMap[i] = t
		mu.Unlock()

		if i/1000 > 0 {
			time.Sleep(time.Millisecond * time.Duration(i/1000))
		}
	}

	//defaultTimerSize = 128
	//大于128后,append t时,会创建新的内存， t.vec指向老的内存，这样t.Stop() 按道理是无法删除
	mu.Lock()
	for j := 0; j < 10; j++ {
		st, ok := timerMap[j]
		if !ok {
			t.Logf("index:%d, not exsit, have been exec ?\n", j)
			continue
		}
		b := st.Stop()
		if !b {
			t.Logf("Stop index:%d,timer fail\n", j)
			continue
		}
		//Stop success and record
		stopTimerMap[j] = st //如果成功Stop 0-9 的timer, 按道理这10 timer是不会被执行的, 如果执行了，就证明Stop实际没有成功。
	}
	mu.Unlock()

	time.Sleep(time.Second)
	if len(timerMap) > 0 {
		t.Logf("len(timerMap):%d\n", len(timerMap))
		// for index := range timerMap {
		// 	t.Logf("index:%d\n", index)
		// }
	}
	testWheel.Stop()
}

func TestTimerPool(t *testing.T) {
	var testWheel = NewWheel(2*time.Millisecond, WithTimerPool(NewTimerSyncPool()))
	if testWheel.PoolNewCount() == -1 { //wheel timerPool have not implements PoolNewCount
		t.Logf("wheel timerPool have not implements PoolNewCount, can't TestTimerPool")
		return
	}

	n := 1000
	release := 0
	timer := testWheel.NewTimer(time.Duration(10) * time.Millisecond) //new one first
	for i := 0; i < n; i++ {
		if timer.Stop() {
			timer.Release()
			release++
		}
		//this timer should get from pool unless gc happen
		timer = testWheel.NewTimer(time.Duration(10+i) * time.Millisecond)
	}

	//除非发生了gc, 不然timer Release 后，就会给 NewTimer。但是如果pool 功能正常，是不会发生gc 的。
	if (n - release + 1) != int(testWheel.PoolNewCount()) {
		t.Fatalf("alloc:%d, pool new count:%d", n-release+1, testWheel.PoolNewCount())
	}
	t.Logf("alloc:%d, pool new count:%d", n-release+1, testWheel.PoolNewCount())
}
