#### v1.1.x 在时间轮里，用链表来组织timer,方便正确的对timer 进行增删，同时用sync.Pool 来缓存timer.
#### v1.0.x 是在https://github.com/siddontang/go/tree/master/time2 基础上修改了一些bug , 但仍然有bug，可以运行测试用例TestStopTimer: go test *.go -test.run TestStopTimer 就会发现Stop timer 返回true,但实际上没有真正Stop 掉。原因已经找到，代码：https://github.com/jursonmo/timer/blob/f6cdf1071e4c9b916a5a526104c1f864f8ad4485/wheel.go#L120 有注释。
