# timer

`timer` 是一个基于多级时间轮实现的 Go 定时器库。它参考 Linux 内核定时器的设计思路，通过多个不同精度的时间轮来管理 timer，而不是为每个 timer 记录等待圈数。

这个实现属于“低精度、高效率”的定时器：用 tick 粒度换取更低的调度和管理成本，适合大量短周期或可接受毫秒级误差的定时任务。

## 特性

- 多级时间轮实现，适合管理大量 timer。
- 支持 `Timer`、`Ticker`、`After`、`Sleep`、`AfterFunc` 等接近 Go 标准库的接口。
- 支持自定义 callback，并可传递任意参数。
- 使用链表组织 timer，便于定时器增删。
- 使用 `sync.Pool` 复用内部 timer 对象，减少频繁分配。
- 支持 `WheelShard`，按 P 分片减少并发场景下的锁竞争。

## 适用场景

这个库更适合：

- 大量连接、会话、请求的超时管理。
- 任务调度、延迟执行、周期性检查。
- 对定时精度要求不是纳秒级，但对 timer 数量和创建成本比较敏感的场景。
- callback 本身很轻量、不会长时间阻塞的场景。

如果业务需要非常高的定时精度，或者 callback 会频繁阻塞很久，应该谨慎使用时间轮方案，或者在 callback 中自行派发 goroutine。

## 安装

```bash
go get github.com/jursonmo/timer
```

## 快速开始

```go
package main

import (
	"fmt"
	"time"

	"github.com/jursonmo/timer"
)

func main() {
	w := timer.NewWheelShard(time.Millisecond)
	defer w.Stop()

	start := time.Now()
	<-w.After(10 * time.Millisecond)

	fmt.Printf("timer fired after %s\n", time.Since(start))
}
```

## 使用示例

### 创建一次性 Timer

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

t := w.NewTimer(10 * time.Millisecond)
start := time.Now()

<-t.C
fmt.Printf("after %s timer timeout\n", time.Since(start))

// timer 已经触发，内部对象可以释放回池中。
t.Release()
```

### 使用 After

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

start := time.Now()
<-w.After(10 * time.Millisecond)
fmt.Printf("w.After: %s\n", time.Since(start))
```

### 使用 Sleep

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

w.Sleep(10 * time.Millisecond)
fmt.Println("wake up")
```

### 创建 Ticker

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

ticker := w.NewTicker(10 * time.Millisecond)

for i := 0; i < 5; i++ {
	t := <-ticker.C
	fmt.Printf("tick at %dms\n", t.UnixNano()/int64(time.Millisecond))
}

if ticker.Stop() {
	ticker.Release()
}
```

### 使用 AfterFunc

`AfterFunc` 的行为接近 Go 标准库，会在到期后执行传入的函数。

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

done := make(chan struct{}, 1)
start := time.Now()

t := w.AfterFunc(10*time.Millisecond, func() {
	fmt.Printf("AfterFunc fired after %s\n", time.Since(start))
	done <- struct{}{}
})

<-done
t.Release()
```

### 使用自定义 callback 和参数

如果需要拿到触发时间，或者需要传递自定义参数，可以使用 `NewWheelTimerFunc`。

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

var t *timer.WheelTimer
done := make(chan struct{}, 1)

arg0 := "arg0"
arg1 := "arg1"
start := time.Now()

t = w.NewWheelTimerFunc(10*time.Millisecond, func(tm time.Time, args ...interface{}) {
	fmt.Printf("callback fired after %s\n", tm.Sub(start))

	if args[0].(string) != arg0 {
		panic("unexpected arg0")
	}
	if args[1].(string) != arg1 {
		panic("unexpected arg1")
	}

	t.Release()
	done <- struct{}{}
}, arg0, arg1)

<-done
```

### 停止 timer

`Stop` 只在 timer 还没有准备执行时返回 `true`。如果 `Stop` 成功，说明 callback 或 channel 发送不会再发生，此时可以调用 `Release`。

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

t := w.NewTimer(time.Second)

if t.Stop() {
	t.Release()
}
```

### 重置 timer

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

t := w.NewTimer(time.Second)

if t.Reset(10 * time.Millisecond) {
	<-t.C
	t.Release()
}
```

## Callback 执行模型

Go 标准库的 `time.AfterFunc(d, callback)` 到期后会创建新的 goroutine 执行 callback。如果 `AfterFunc` 调用很多，可能会产生大量 goroutine，进而影响性能。

本库的 `AfterFunc` 为了保持接近标准库的行为，也会把 `func()` 放到新的 goroutine 中执行。如果 callback 很轻量，并且你希望减少 goroutine 创建成本，可以优先使用 `NewTimerFunc` 或 `NewWheelTimerFunc`：它们会在时间轮处理到期 timer 的执行路径里直接运行 callback。

对于可能阻塞的任务，建议在 callback 内自行启动 goroutine：

```go
w := timer.NewWheelShard(time.Millisecond)
defer w.Stop()

w.NewWheelTimerFunc(100*time.Millisecond, func(t time.Time, args ...interface{}) {
	go func() {
		// do something blocking
	}()
}, "arg1", "arg2")
```

直接运行在时间轮执行路径中的 callback 应尽量保持轻量。如果 callback 执行时间过长，会影响同一批到期 timer 的处理。

## API 概览

### 包级默认时间轮

```go
timer.After(d)
timer.Sleep(d)
timer.AfterFunc(d, f)
timer.NewTimer(d)
timer.NewTicker(d)
timer.Tick(d)
timer.TickFunc(d, f)
timer.NewTimerFunc(d, f, args...)
timer.NewWheelTimerFunc(d, f, args...)
timer.Timers()
timer.StopDefaultWheelShard()
```

默认时间轮使用 `NewWheelShard(100 * time.Millisecond)` 创建。

### 自定义时间轮

```go
w := timer.NewWheel(time.Millisecond)
defer w.Stop()

ws := timer.NewWheelShard(time.Millisecond)
defer ws.Stop()
```

`NewWheel` 创建单个时间轮。`NewWheelShard` 会按当前 `runtime.GOMAXPROCS(0)` 创建多个 wheel，并尽量按 P 选择对应的 wheel，以减少并发添加 timer 时的锁竞争。

### Timer

```go
t := w.NewTimer(d)
<-t.C

t.Stop()
t.Reset(d)
t.Release()
t.Info()
```

`NewTimerFunc` 会创建带自定义 callback 的 `Timer`，适合需要回调参数但仍希望拿到 `Timer` 包装对象的场景。

### Ticker

```go
ticker := w.NewTicker(d)
<-ticker.C

ticker.Stop()
ticker.Reset(d)
ticker.Release()
```

### WheelTimer

`WheelTimer` 是更轻量的 callback timer，适合不需要 `Timer{}` 包装对象、只关心 callback 的场景。

```go
t := w.NewWheelTimerFunc(d, func(tm time.Time, args ...interface{}) {
	// handle timeout
}, args...)

t.Stop()
t.ResetTimer(d, period)
t.Release()
```

## 精度说明

时间轮的精度由创建时传入的 `tick` 决定：

```go
w := timer.NewWheelShard(time.Millisecond)
```

如果 `tick` 是 `1ms`，timer 的触发时间会向上换算到 tick 边界。更小的 tick 可以提升精度，但会增加时间轮 tick 调度成本；更大的 tick 可以降低开销，但触发误差也会变大。

## 生命周期注意事项

- `Stop` 返回 `true`：timer 已成功停止，可以调用 `Release`。
- timer 已经触发：callback 执行完成后，或者 channel 收到时间后，可以调用 `Release`。
- timer 还在时间轮中时不能 `Release`。
- `After`、`Sleep`、`Tick` 等便捷接口没有直接暴露 `Release`，适合简单场景；大量 timer 场景建议使用显式 `NewTimer` / `NewWheelTimerFunc` 并在合适时释放。
- `Wheel.Stop` / `WheelShard.Stop` 用于停止内部 tick goroutine，通常在自定义 wheel 不再使用时调用。

## 测试

```bash
go test ./...
```

运行 benchmark：

```bash
go test -bench=. ./...
```

## 实现说明

最初版本基于 `github.com/siddontang/go/tree/master/time2` 修改，并修复了一些 timer 删除和停止相关的问题。早期版本使用 slice 组织 timer 列表，不利于删除；当前版本改用链表组织 timer，使 timer 的添加和删除更直接。

这个实现没有使用单个 round 记录 timer 还需要等待多少圈，而是采用多组精度不同的时间轮，例如秒级、分钟级等层级。timer 到期前会逐级 cascade 到低层时间轮，最终在 `tv1` 中触发。

## Release 记录

### v1.2.x

- 增加 `WheelShard`，减少并发场景下的锁竞争，提高性能。

### v1.1.x

- 在时间轮中使用链表组织 timer，方便正确地增删 timer。
- 使用 `sync.Pool` 缓存 timer 对象，减少分配。

### v1.0.x
-  是在https://github.com/siddontang/go/tree/master/time2 基础上修改了一些bug , 但仍然有bug，可以运行测试用例TestStopTimer: go test *.go -test.run TestStopTimer 就会发现Stop timer 返回true,但实际上没有真正Stop 掉。原因已经找到，代码：https://github.com/jursonmo/timer/blob/f6cdf1071e4c9b916a5a526104c1f864f8ad4485/wheel.go#L120 有注释。

- [初次对timer 的修改](https://github.com/jursonmo/gocode/tree/master/src/timer), 正如最后所说的，用slice 来当timer 列表，其实不利于timer的删除，最好还是用ilist, 添加删除都很方便。类似于内核链表做法。
## 推荐阅读

- [go-timewheel](https://xiaorui.cc/archives/6160)
- [早期 timer 修改记录](https://github.com/jursonmo/gocode/tree/master/src/timer)
