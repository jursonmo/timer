### 这里实现的时间轮精度比较低的，属于精度低效率高的定时器, 也就是以精度换取效率。 这个完全参照内核的定时器的实现方式，用go来实现。 
go 标准库timer.AfterFunc(d, callback) 最终会创建新的goroutine 来执行回调函数callback，如果timer.AfterFun调用过多的话，就会产生很多goroutine,性能下降，如果callback是非阻塞的，其实完全可以不用每个timer 都创建新的goroutine 来执行回调函数callback。

#### v1.1.x 在时间轮里，用链表来组织timer,方便正确的对timer 进行增删，同时用sync.Pool 来缓存timer对象.
#### v1.0.x 是在https://github.com/siddontang/go/tree/master/time2 基础上修改了一些bug , 但仍然有bug，可以运行测试用例TestStopTimer: go test *.go -test.run TestStopTimer 就会发现Stop timer 返回true,但实际上没有真正Stop 掉。原因已经找到，代码：https://github.com/jursonmo/timer/blob/f6cdf1071e4c9b916a5a526104c1f864f8ad4485/wheel.go#L120 有注释。
