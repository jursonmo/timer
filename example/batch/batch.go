package main

import (
	"log"
	"sync"
	"time"

	"github.com/jursonmo/timer"
)

func main() {
	var testWheel = timer.NewWheel(1 * time.Millisecond)
	//n := 3000
	n := 1000
	var mu sync.Mutex
	stopTimerMap := make(map[int]*timer.Timer)
	timerMap := make(map[int]*timer.Timer)
	f := func(t time.Time, args ...interface{}) {
		mu.Lock()
		index := args[0].(int)
		st, ok := stopTimerMap[index]
		if ok {
			log.Fatalf("index:%d timer:%s has Stop, but still exec", index, st.Info())
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
			log.Fatalf("index:%, not exsit\n", j)
		}
		b := st.Stop()
		if !b {
			log.Printf("Stop index:%d,timer fail\n", j) //如果成功Stop 0-9 的timer, 按道理这10 timer是不会被执行的, 如果执行了，就证明Stop实际没有成功。
			continue
		}
		stopTimerMap[j] = st
	}
	mu.Unlock()

	time.Sleep(time.Second)
	if len(timerMap) > 0 {
		log.Printf("len(timerMap):%d\n", len(timerMap))
		for index := range timerMap {
			log.Printf("index:%d\n", index)
		}
	}
}
