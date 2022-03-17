### 这里实现的时间轮精度比较低的，属于精度低效率高的定时器, 也就是以精度换取效率。 这个完全参照内核的定时器的实现方式，用go来实现。 

1. 使用
```go
func main(){
	tick := 1 * time.Millisecond
	w := timer.NewWheelShard(tick)
	t1 := w.NewTimer(time.Millisecond * 10)
	start := time.Now()
	<-t1.C
	fmt.Printf("after %s timer timeout", time.Since(start))

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

	var t3 *timer.WheelTimer
	timerDone := make(chan struct{}, 1)
	arg0 := "arg0"
	arg1 := "arg1"
	f := func(t time.Time, args ...interface{}) {
		log.Printf("t3 func exec after %v\n", t.Sub(start))
		if args[0].(string) != arg0 {
			log.Fatal("should arg0")
		}
		if args[1].(string) != arg1 {
			log.Fatal("should arg1")
		}
		ok := t3.Stop()
		if ok {
			log.Fatal("t3 should not be Stop")
		}
		t3.Release()
		timerDone <- struct{}{}
	}
	t3 = w.NewWheelTimerFunc(d, f, arg0, arg1)

	select {
	case <-time.After(d + 2*tick):
		panic("timer haven't done?")
	case <-timerDone:
		break
    }
}
```
2. go 标准库timer.AfterFunc(d, callback) 最终会创建新的goroutine 来执行回调函数callback，如果timer.AfterFun调用过多的话，就会产生很多goroutine,性能下降，如果callback是非阻塞的，其实完全可以不用每个timer 都创建新的goroutine 来执行回调函数callback。

使用这个timer库时，如果想执行阻塞的任务，可以在callback 创建goroutine, 比如：
```go
w := timer.NewWheel(1 * time.Millisecond)
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

##### [初次对timer 的修改](https://github.com/jursonmo/gocode/tree/master/src/timer)