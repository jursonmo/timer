### 这里实现的时间轮精度比较低的，属于精度低效率高的定时器, 也就是以精度换取效率。 这个完全参照内核的定时器的实现方式，用go来实现。 

1. 使用
```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/jursonmo/timer"
)
func main(){
	tick := 1 * time.Millisecond
	w := timer.NewWheelShard(tick)
	//用法一：创建定时器
	t1 := w.NewTimer(time.Millisecond * 10)
	start := time.Now()
	<-t1.C
	fmt.Printf("after %s timer timeout", time.Since(start))

	//用法二：类似原生time 库time.After()
	start = time.Now()
	<-w.After(time.Millisecond * 10):
	fmt.Printf("w.After: %s ", time.Since(start))

	//用法三：创建ticker
	d := time.Millisecond * 10
	ticker := w.NewTicker(d)
	n := 0
	for t := range ticker.C {
		n++
		fmt.Printf("Millisecond:%d\n", t.UnixNano()/int64(time.Millisecond))
		if n > 5 {
			if ticker.Stop() {
				ticker.Release()
				break
			} else {
				panic(" ticker.Stop() fail")
			}
		}
	}
	
	//用法四：类似原生time 库的AfterFunc
	start = time.Now()
	w.AfterFunc(time.Millisecond * 10, func(){
		fmt.Printf("in w.AfterFunc handler, time elapse:%s\n", time.Since(start))
	})

	//用法五：创建“自定义回调函数”的定时器, 
	var t3 *timer.WheelTimer
	timerDone := make(chan struct{}, 1)
	arg0 := "arg0"
	arg1 := "arg1"
	callback := func(t time.Time, args ...interface{}) {
		log.Printf("t3 func exec after %v\n", t.Sub(start))
		if args[0].(string) != arg0 {
			log.Fatal("should arg0")
		}
		if args[1].(string) != arg1 {
			log.Fatal("should arg1")
		}
		// ok := t3.Stop()
		// if ok {
		// 	log.Fatal("t3 should not be Stop")
		// }
		t3.Release()
		timerDone <- struct{}{}
	}
	t3 = w.NewWheelTimerFunc(d, callback, arg0, arg1)

	select {
	case <-time.After(d + 2*tick):
		panic("timer haven't done?")
	case <-timerDone:
		break
    }
}
```
2. go 标准库timer.AfterFunc(d, callback) 最终会创建新的goroutine 来执行回调函数callback，如果timer.AfterFun调用过多的话，就会产生很多goroutine,性能下降，如果callback是非阻塞的，其实完全可以不需要为每个timer 都创建新的goroutine 来执行回调函数callback，在少数的goroutine 里顺序执行timer callback 就行。

使用这个timer库时，如果想执行阻塞的任务，可以在callback 创建goroutine, 比如：
```go
w := timer.NewWheelShard(1 * time.Millisecond)
callback := func(t time.Time, args ...interface{}) {
        //new a groutine to do 
        go func() {
            //do something blocking

        }()
}
w.NewWheelTimerFunc(time.Millisecond*100, callback, "arg1", "arg2")

```
#### release 记录：
##### v1.2.x 增加 wheel shard 避免锁的竞争，提高性能。
##### v1.1.x 在时间轮里，用链表来组织timer,方便正确的对timer 进行增删，同时用sync.Pool 来缓存timer对象.
##### v1.0.x 是在https://github.com/siddontang/go/tree/master/time2 基础上修改了一些bug , 但仍然有bug，可以运行测试用例TestStopTimer: go test *.go -test.run TestStopTimer 就会发现Stop timer 返回true,但实际上没有真正Stop 掉。原因已经找到，代码：https://github.com/jursonmo/timer/blob/f6cdf1071e4c9b916a5a526104c1f864f8ad4485/wheel.go#L120 有注释。

##### [初次对timer 的修改](https://github.com/jursonmo/gocode/tree/master/src/timer), 正如最后所说的，用slice 来当timer 列表，其实不利于timer的删除，最好还是用ilist, 添加删除都很方便。类似于内核链表做法。

推荐阅读：
[go-timewheel](https://xiaorui.cc/archives/6160)
