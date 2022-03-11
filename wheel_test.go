package timer

import (
	"sync"
	"testing"
	"time"
)

var testWheel = NewWheel(1 * time.Millisecond)

func TestTimer(t *testing.T) {
	t1 := testWheel.NewTimer(500 * time.Millisecond)

	before := time.Now()
	<-t1.C

	after := time.Now()

	println(after.Sub(before).String())
}

func TestTicker(t *testing.T) {
	wait := make(chan struct{}, 100)
	i := 0
	f := func() {
		println(time.Now().Unix())
		i++
		if i >= 10 {
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
		mu.Lock()
		t := testWheel.NewTimerFunc(time.Millisecond*500, f, i)
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
}
