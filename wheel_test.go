package timer

import (
	"context"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestTimer(t *testing.T) {
	tick := 1 * time.Millisecond
	var w = NewWheel(tick)
	defer w.Stop()
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
}

func TestTicker(t *testing.T) {
	var testWheel = NewWheel(1 * time.Millisecond)
	defer testWheel.Stop()
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
	defer w.Stop()

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
		t.Fatalf("timer should have executed, timer.Reset() should be failed")
	}
}

func TestTimers(t *testing.T) {
	w := NewWheel(1 * time.Millisecond)
	defer w.Stop()

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

	if w.Timers() != w.RealTimers() {
		t.Fatalf("w.Timers():%d not eq w.RealTimers():%d \n", w.Timers(), w.RealTimers())
	}
}

// go test *.go -test.run TestStopTimer
func TestStopTimer(t *testing.T) {
	var testWheel = NewWheel(1 * time.Millisecond)
	defer testWheel.Stop()

	//n := 3000
	n := 1000
	var mu sync.Mutex
	stopTimerMap := make(map[int]*WheelTimer)
	timerMap := make(map[int]*WheelTimer)
	f := func(_ time.Time, args ...interface{}) {
		mu.Lock()
		index := args[0].(int)
		st, ok := stopTimerMap[index]
		if ok {
			t.Fatalf("index:%d timer:%s has Stop, but still exec", index, st.Info())
		}
		delete(timerMap, index) //timer execute, delete from timerMap
		mu.Unlock()
	}
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	//创建1000个延迟范围在500-520 毫秒的定时器
	for i := 0; i < n; i++ {
		delay := 500 + r.Intn(20)
		t := testWheel.NewWheelTimerFunc(time.Duration(delay)*time.Millisecond, f, i)
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
	if len(timerMap) != len(stopTimerMap) {
		t.Fatalf("len(timerMap)=%d not equal len(stopTimerMap)=%d ", len(timerMap), len(stopTimerMap))
	}
	//timerMap 剩下的是没有得到执行的timer, 即中途被stop的timer
	if !reflect.DeepEqual(timerMap, stopTimerMap) {
		t.Fatalf("timerMap not equal stopTimerMap ")
	}

}

func TestTimerPool(t *testing.T) {
	var testWheel = NewWheel(2*time.Millisecond, WithTimerPool(NewTimerSyncPool()))
	defer testWheel.Stop()

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
	//go version 1.14 以后的版本，需要两次gc才会把pool 的对象回收。
	if (n - release + 1) != int(testWheel.PoolNewCount()) {
		t.Fatalf("alloc:%d, pool new count:%d", n-release+1, testWheel.PoolNewCount())
	}
	t.Logf("alloc:%d, pool new count:%d", n-release+1, testWheel.PoolNewCount())
}

func BenchmarkWheelTimerFunc(b *testing.B) {
	var w = NewWheel(1 * time.Millisecond)
	defer w.Stop()
	// s := rand.NewSource(time.Now().UnixNano())
	// r := rand.New(s)
	f := func(t time.Time, args ...interface{}) {}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		delay := 10
		t := w.NewWheelTimerFunc(time.Duration(delay)*time.Millisecond, f, i)
		if t.Stop() {
			t.Release()
		}
		delay++
	}

	if w.Timers() != 0 {
		b.Fatalf("w.Timers():%d not eq 0\n", w.Timers())
	}
	if w.RealTimers() != 0 {
		b.Fatalf("w.RealTimers():%d not eq 0\n", w.RealTimers())
	}
}

func BenchmarkWheelTimerParallel(b *testing.B) {
	var w = NewWheel(1 * time.Millisecond)
	defer w.Stop()
	// s := rand.NewSource(time.Now().UnixNano())
	// r := rand.New(s)
	f := func(t time.Time, args ...interface{}) {}

	//log.Printf("------------start----------")
	b.Log("------------start----------")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			delay := 10
			t := w.NewWheelTimerFunc(time.Duration(delay)*time.Millisecond, f, delay)
			if t.Stop() {
				t.Release()
			}
			delay++
		}
	})
	if w.Timers() != 0 {
		b.Fatalf("w.Timers():%d not eq 0\n", w.Timers())
	}
	if w.RealTimers() != 0 {
		b.Fatalf("w.RealTimers():%d not eq 0\n", w.RealTimers())
	}
}

func BenchmarkWheelShardTimerParallel(b *testing.B) {
	var w = NewWheelShard(1 * time.Millisecond)
	defer w.Stop()
	// s := rand.NewSource(time.Now().UnixNano())
	// r := rand.New(s)
	f := func(t time.Time, args ...interface{}) {}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			delay := 10
			t := w.NewWSTimerFunc(time.Duration(delay)*time.Millisecond, f, delay)
			if t.Stop() {
				t.Release()
			}
			delay++
		}
	})
	if w.Timers() != 0 {
		b.Fatalf("w.Timers():%d not eq 0\n", w.Timers())
	}
}

// 检测之前的所有测试用例是否有泄露, 注意把 TestGoroutineLeak 放在所有测试用例的最后
func TestGoroutineLeak(t *testing.T) {
	defer func() {
		time.Sleep(time.Second)
		goleak.VerifyNone(t) //检测之前的所有测试用例是否有泄露
	}()

	StopDefaultWheelShard() //需要 stop 默认的时间轮
}

/*
// 测试单个测试用例
go test -v *.go -test.run TestStopTimer
go test -v *.go -run TestStopTimer

//测试 所有测试用例
MacBook-Pro:timer obc$ go test *.go -v
=== RUN   TestTimer
--- PASS: TestTimer (0.01s)
=== RUN   TestTicker
1647501212
1647501213
1647501214
1647501215
1647501216
5.008177058s
--- PASS: TestTicker (5.01s)
=== RUN   TestResetTimer
--- PASS: TestResetTimer (0.01s)
=== RUN   TestTimers
--- PASS: TestTimers (1.01s)
=== RUN   TestStopTimer
    wheel_test.go:168: len(timerMap):10
--- PASS: TestStopTimer (1.00s)
=== RUN   TestTimerPool
    wheel_test.go:206: alloc:1, pool new count:1
--- PASS: TestTimerPool (0.00s)
PASS
ok  	command-line-arguments	7.053s

MacBook-Pro:timer obc$ go test -bench .  -benchmem
goos: darwin
goarch: amd64
pkg: github.com/jursonmo/timer
BenchmarkWheelTimerFunc-4            	 5765389	       201 ns/op	      24 B/op	       2 allocs/op
BenchmarkWheelTimerParallel-4        	 4115576	       282 ns/op	      24 B/op	       2 allocs/op
BenchmarkWheelShardTimerParallel-4   	12065094	        98.3 ns/op	      24 B/op	       2 allocs/op
PASS
ok  	github.com/jursonmo/timer	11.243s
*/
