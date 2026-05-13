package timer

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

const testTick = time.Millisecond

func TestMain(m *testing.M) {
	code := m.Run()

	StopDefaultWheelShard()
	time.Sleep(200 * time.Millisecond)
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "goleak: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func newTestWheel(t *testing.T, tick time.Duration, opts ...Option) *Wheel {
	t.Helper()

	w := NewWheel(tick, opts...)
	t.Cleanup(w.Stop)
	return w
}

func waitTime(t *testing.T, ch <-chan time.Time, timeout time.Duration, name string) time.Time {
	t.Helper()

	select {
	case tm := <-ch:
		return tm
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s after %s", name, timeout)
		return time.Time{}
	}
}

func waitStruct(t *testing.T, ch <-chan struct{}, timeout time.Duration, name string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s after %s", name, timeout)
	}
}

func assertNoTime(t *testing.T, ch <-chan time.Time, within time.Duration, name string) {
	t.Helper()

	select {
	case tm := <-ch:
		t.Fatalf("%s fired unexpectedly at %v", name, tm)
	case <-time.After(within):
	}
}

func requireEventually(t *testing.T, timeout time.Duration, check func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %s", timeout, msg)
}

func assertWheelEmpty(t *testing.T, w *Wheel) {
	t.Helper()

	requireEventually(t, 100*time.Millisecond, func() bool {
		return w.Timers() == 0 && w.RealTimers() == 0
	}, fmt.Sprintf("wheel still has timers: Timers=%d RealTimers=%d", w.Timers(), w.RealTimers()))
}

// TestDurationToTicksCeilsAndHandlesNonPositiveDurations 测试 duration 转 tick 的换算逻辑。
// 功能点：非正数应返回 0；不足一个 tick、超过整数 tick 的 duration 应向上取整。
// 方法：直接调用内部换算函数，用表格测试覆盖边界值和典型值。
func TestDurationToTicksCeilsAndHandlesNonPositiveDurations(t *testing.T) {
	tick := 10 * time.Millisecond
	tests := []struct {
		name string
		d    time.Duration
		want uint64
	}{
		{name: "negative", d: -time.Millisecond, want: 0},
		{name: "zero", d: 0, want: 0},
		{name: "sub tick", d: time.Nanosecond, want: 1},
		{name: "exact tick", d: tick, want: 1},
		{name: "one past tick", d: tick + time.Nanosecond, want: 2},
		{name: "almost three ticks", d: 3*tick - time.Nanosecond, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := durationToTicks(tt.d, tick); got != tt.want {
				t.Fatalf("durationToTicks(%s, %s) = %d, want %d", tt.d, tick, got, tt.want)
			}
		})
	}
}

// TestNewWheelRequiresPositiveTick 测试 NewWheel 的参数校验。
// 功能点：tick 必须大于 0；传入 0 或负数时应该 panic，防止创建无效时间轮。
// 方法：对子用例使用 defer recover 捕获 panic，并断言非法 tick 一定触发 panic。
func TestNewWheelRequiresPositiveTick(t *testing.T) {
	for _, tick := range []time.Duration{0, -time.Millisecond} {
		t.Run(tick.String(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewWheel(%s) did not panic", tick)
				}
			}()

			NewWheel(tick)
		})
	}
}

// TestNewTimerFiresOnceAndRemovesItself 测试一次性 Timer 的基本行为。
// 功能点：NewTimer 创建的 timer 应按时触发一次，触发后不再重复触发，并从时间轮中移除。
// 方法：等待 timer.C 收到事件，再用短 timeout 验证没有第二次事件，最后检查 Timers 和 RealTimers 都归零。
func TestNewTimerFiresOnceAndRemovesItself(t *testing.T) {
	w := newTestWheel(t, testTick)
	timer := w.NewTimer(5 * testTick)

	waitTime(t, timer.C, 200*time.Millisecond, "timer")
	assertNoTime(t, timer.C, 10*testTick, "one-shot timer")
	assertWheelEmpty(t, w)
}

// TestAfterAndAfterFunc 测试 After 和 AfterFunc 两个便捷 API。
// 功能点：After 应返回会触发的时间 channel；AfterFunc 应只执行一次回调，并在执行后清理 timer。
// 方法：分别等待 channel 事件和回调完成信号，再用原子计数确认回调只运行一次。
func TestAfterAndAfterFunc(t *testing.T) {
	w := newTestWheel(t, testTick)

	waitTime(t, w.After(5*testTick), 200*time.Millisecond, "After")

	var called int32
	done := make(chan struct{}, 1)
	w.AfterFunc(5*testTick, func() {
		atomic.AddInt32(&called, 1)
		done <- struct{}{}
	})

	waitStruct(t, done, 200*time.Millisecond, "AfterFunc callback")
	time.Sleep(10 * testTick)
	if got := atomic.LoadInt32(&called); got != 1 {
		t.Fatalf("AfterFunc callback count = %d, want 1", got)
	}
	assertWheelEmpty(t, w)
}

// TestNewTimerFuncPassesTimestampAndArgs 测试自定义 callback timer 的参数传递。
// 功能点：NewTimerFunc 的回调应收到非零触发时间，并完整收到创建 timer 时传入的可变参数。
// 方法：在 callback 中检查 time.Time 和 args，callback 完成后再检查时间轮已清空。
func TestNewTimerFuncPassesTimestampAndArgs(t *testing.T) {
	w := newTestWheel(t, testTick)
	done := make(chan struct{}, 1)

	w.NewTimerFunc(5*testTick, func(tm time.Time, args ...interface{}) {
		if tm.IsZero() {
			t.Errorf("callback timestamp is zero")
		}
		if len(args) != 2 || args[0] != "key" || args[1] != 42 {
			t.Errorf("callback args = %#v, want [key 42]", args)
		}
		done <- struct{}{}
	}, "key", 42)

	waitStruct(t, done, 200*time.Millisecond, "NewTimerFunc callback")
	assertWheelEmpty(t, w)
}

// TestResetTimer 测试 Timer.Reset 的状态约束和重新调度能力。
// 功能点：已 Stop 的 timer 可以 Reset 后重新触发；活跃 timer 可以 Reset 到新的时间；已执行完成的 timer 不能 Reset。
// 方法：分别构造 stopped、active、executed 三种状态，检查 Reset 返回值并等待重新调度后的触发事件。
func TestResetTimer(t *testing.T) {
	w := newTestWheel(t, testTick)

	stopped := w.NewTimer(100 * testTick)
	if !stopped.Stop() {
		t.Fatalf("Stop before Reset returned false")
	}
	if !stopped.Reset(5 * testTick) {
		t.Fatalf("Reset of stopped timer returned false")
	}
	waitTime(t, stopped.C, 200*time.Millisecond, "reset stopped timer")
	if stopped.Reset(5 * testTick) {
		t.Fatalf("Reset of already executed timer returned true")
	}

	active := w.NewTimer(100 * testTick)
	if !active.Reset(5 * testTick) {
		t.Fatalf("Reset of active timer returned false")
	}
	waitTime(t, active.C, 200*time.Millisecond, "reset active timer")
	assertWheelEmpty(t, w)
}

// TestTimersAndRealTimersTrackAddsAndStops 测试 timer 计数和真实链表内容的一致性。
// 功能点：批量添加 timer 后 Timers 和 RealTimers 都应等于添加数量；批量 Stop 后两者都应归零。
// 方法：创建多个长延迟 timer 避免自然触发，检查计数后逐个 Stop，再轮询确认时间轮为空。
func TestTimersAndRealTimersTrackAddsAndStops(t *testing.T) {
	w := newTestWheel(t, testTick)

	const n = 32
	timers := make([]*Timer, 0, n)
	for i := 0; i < n; i++ {
		timers = append(timers, w.NewTimer(time.Second))
	}

	if got := w.Timers(); got != n {
		t.Fatalf("Timers() = %d, want %d", got, n)
	}
	if got := w.RealTimers(); got != n {
		t.Fatalf("RealTimers() = %d, want %d", got, n)
	}

	for i, timer := range timers {
		if !timer.Stop() {
			t.Fatalf("timer %d Stop returned false", i)
		}
	}
	assertWheelEmpty(t, w)
}

// TestStopTimerPreventsCallbackExecution 测试 Stop 的删除语义。
// 功能点：Stop 返回成功的 timer 不应再执行 callback；未 Stop 的 timer 应全部执行且只执行一次。
// 方法：批量创建 callback timer，停止其中一部分；收集执行的 index，断言停止集合未出现、未停止集合全部出现且没有重复。
func TestStopTimerPreventsCallbackExecution(t *testing.T) {
	w := newTestWheel(t, testTick)

	const (
		n            = 256
		stoppedCount = 32
	)
	fired := make(chan int, n)
	timers := make([]*WheelTimer, 0, n)
	for i := 0; i < n; i++ {
		timer := w.NewWheelTimerFunc(40*testTick, func(_ time.Time, args ...interface{}) {
			fired <- args[0].(int)
		}, i)
		timers = append(timers, timer)
	}

	stopped := make(map[int]struct{}, stoppedCount)
	for i := 0; i < stoppedCount; i++ {
		if !timers[i].Stop() {
			t.Fatalf("timer %d Stop returned false", i)
		}
		stopped[i] = struct{}{}
	}

	expected := n - stoppedCount
	seen := make(map[int]struct{}, expected)
	timeout := time.After(500 * time.Millisecond)
	for len(seen) < expected {
		select {
		case idx := <-fired:
			if _, ok := stopped[idx]; ok {
				t.Fatalf("stopped timer %d still executed", idx)
			}
			if _, ok := seen[idx]; ok {
				t.Fatalf("timer %d executed more than once", idx)
			}
			seen[idx] = struct{}{}
		case <-timeout:
			t.Fatalf("only %d/%d unstopped timers executed", len(seen), expected)
		}
	}

	select {
	case idx := <-fired:
		t.Fatalf("unexpected extra timer execution: %d", idx)
	case <-time.After(20 * testTick):
	}
	assertWheelEmpty(t, w)
}

// TestTickerFiresRepeatedlyAndStops 测试 Ticker 的周期触发和停止行为。
// 功能点：Ticker 应持续按周期产生事件；Stop 成功后不应再产生新事件，并从时间轮移除。
// 方法：连续等待三次 tick，再重试 Stop 直到成功；清空已有缓冲后验证短时间内没有新的 tick。
func TestTickerFiresRepeatedlyAndStops(t *testing.T) {
	w := newTestWheel(t, testTick)
	ticker := w.NewTicker(5 * testTick)

	for i := 0; i < 3; i++ {
		waitTime(t, ticker.C, 200*time.Millisecond, fmt.Sprintf("ticker tick %d", i+1))
	}

	requireEventually(t, 100*time.Millisecond, ticker.Stop, "ticker did not stop")
	for {
		select {
		case <-ticker.C:
		default:
			assertNoTime(t, ticker.C, 25*testTick, "stopped ticker")
			assertWheelEmpty(t, w)
			return
		}
	}
}

// TestTimerCascadesFromHigherWheelLevel 测试多级时间轮的 cascade 逻辑。
// 功能点：超过 tv1 范围的 timer 应先进入更高层级时间轮，随后在 jiffies 推进时级联回 tv1 并最终执行。
// 方法：创建 delay 大于 tvr_size 个 tick 的 timer，等待 callback 完成，再确认时间轮清空。
func TestTimerCascadesFromHigherWheelLevel(t *testing.T) {
	w := newTestWheel(t, testTick)
	done := make(chan struct{}, 1)

	w.NewWheelTimerFunc(time.Duration(tvr_size+5)*testTick, func(time.Time, ...interface{}) {
		done <- struct{}{}
	})

	waitStruct(t, done, 800*time.Millisecond, "cascaded timer")
	assertWheelEmpty(t, w)
}

// TestTimerPoolReusesReleasedTimers 测试 timer pool 的复用行为。
// 功能点：Stop 后 Release 的 timer 应放回 pool；再次创建 timer 时应复用对象而不是增加新的分配计数。
// 方法：使用带 PoolNewCount 的 sync.Pool 实现，创建、Stop、Release、再创建，检查 PoolNewCount 保持为 1。
func TestTimerPoolReusesReleasedTimers(t *testing.T) {
	w := newTestWheel(t, 2*testTick, WithTimerPool(NewTimerSyncPool()))
	if w.PoolNewCount() == -1 {
		t.Skip("timer pool does not expose PoolNewCount")
	}

	timer := w.NewTimer(time.Second)
	if got := w.PoolNewCount(); got != 1 {
		t.Fatalf("PoolNewCount after first timer = %d, want 1", got)
	}
	if !timer.Stop() {
		t.Fatalf("first timer Stop returned false")
	}
	timer.Release()

	timer = w.NewTimer(time.Second)
	if got := w.PoolNewCount(); got != 1 {
		t.Fatalf("PoolNewCount after reusing released timer = %d, want 1", got)
	}
	if !timer.Stop() {
		t.Fatalf("second timer Stop returned false")
	}
	timer.Release()
	assertWheelEmpty(t, w)
}

func BenchmarkWheelTimerFunc(b *testing.B) {
	var w = NewWheel(1 * time.Millisecond)
	defer w.Stop()
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
	f := func(t time.Time, args ...interface{}) {}

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
