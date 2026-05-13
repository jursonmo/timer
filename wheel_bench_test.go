package timer

import (
	"fmt"
	"testing"
	"time"
)

/*
# 跑当前包所有 benchmark，跳过普通单测
go test -run '^$' -bench . -benchmem

# 跑所有包的 benchmark
go test ./... -run '^$' -bench . -benchmem

# 单独跑某个 benchmark
go test -run '^$' -bench '^BenchmarkNewTimer$' -benchmem

# 单独跑并行 benchmark
go test -run '^$' -bench '^BenchmarkNewTimerParallel$' -benchmem

# 跑名字包含 NewWheelTimerFunc 的 benchmark
go test -run '^$' -bench 'NewWheelTimerFunc' -benchmem

# 更稳定地重复跑 5 次
go test -run '^$' -bench '^BenchmarkNewWheelTimerFunc$' -benchmem -count=5

# 指定每个 benchmark 至少跑 3 秒
go test -run '^$' -bench '^BenchmarkNewWheelTimerFunc$' -benchmem -benchtime=3s
*/

const (
	benchTick  = time.Millisecond
	benchDelay = time.Hour
)

type benchDiscardLogger struct{}

func (benchDiscardLogger) Debugf(string, ...interface{}) {}
func (benchDiscardLogger) Infof(string, ...interface{})  {}
func (benchDiscardLogger) Warnf(string, ...interface{})  {}
func (benchDiscardLogger) Errorf(string, ...interface{}) {}
func (benchDiscardLogger) Fatalf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}

func newBenchWheel(b *testing.B) *Wheel {
	b.Helper()

	w := NewWheel(benchTick, WithLogger(benchDiscardLogger{}))
	b.Cleanup(func() {
		if got := w.Timers(); got != 0 {
			b.Fatalf("Timers() = %d, expected 0", got)
		}
		if got := w.RealTimers(); got != 0 {
			b.Fatalf("RealTimers() = %d, expected 0", got)
		}
		w.Stop()
	})
	return w
}

func newBenchWheelShard(b *testing.B) *wheel_shard {
	b.Helper()

	ws := NewWheelShard(benchTick, WithLogger(benchDiscardLogger{}))
	b.Cleanup(func() {
		if got := ws.Timers(); got != 0 {
			b.Fatalf("Timers() = %d, expected 0", got)
		}
		for i, w := range ws.wheels {
			if got := w.RealTimers(); got != 0 {
				b.Fatalf("wheels[%d].RealTimers() = %d, expected 0", i, got)
			}
		}
		ws.Stop()
	})
	return ws
}

func stopReleaseTimer(b *testing.B, timer *Timer) {
	b.Helper()
	if timer == nil {
		b.Fatal("timer is nil")
	}
	if !timer.Stop() {
		b.Fatal("timer fired before Stop")
	}
	timer.Release()
}

func stopReleaseWheelTimer(b *testing.B, timer *WheelTimer) {
	b.Helper()
	if timer == nil {
		b.Fatal("timer is nil")
	}
	if !timer.Stop() {
		b.Fatal("timer fired before Stop")
	}
	timer.Release()
}

func BenchmarkNewTimer(b *testing.B) {
	w := newBenchWheel(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := w.NewTimer(benchDelay)
		stopReleaseTimer(b, timer)
	}
}

func BenchmarkNewTimerParallel(b *testing.B) {
	w := newBenchWheel(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			timer := w.NewTimer(benchDelay)
			stopReleaseTimer(b, timer)
		}
	})
}

func BenchmarkAfterFunc(b *testing.B) {
	w := newBenchWheel(b)

	f := func() {}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := w.AfterFunc(benchDelay, f)
		stopReleaseTimer(b, timer)
	}
}

func BenchmarkAfterFuncParallel(b *testing.B) {
	w := newBenchWheel(b)

	f := func() {}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			timer := w.AfterFunc(benchDelay, f)
			stopReleaseTimer(b, timer)
		}
	})
}

func BenchmarkNewWheelTimerFunc(b *testing.B) {
	w := newBenchWheel(b)

	f := func(time.Time, ...interface{}) {}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := w.NewWheelTimerFunc(benchDelay, f)
		stopReleaseWheelTimer(b, timer)
	}
}

func BenchmarkNewWheelTimerFuncParallel(b *testing.B) {
	w := newBenchWheel(b)

	f := func(time.Time, ...interface{}) {}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			timer := w.NewWheelTimerFunc(benchDelay, f)
			stopReleaseWheelTimer(b, timer)
		}
	})
}

func BenchmarkWheelShardNewTimer(b *testing.B) {
	ws := newBenchWheelShard(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := ws.NewTimer(benchDelay)
		stopReleaseTimer(b, timer)
	}
}

func BenchmarkWheelShardNewTimerParallel(b *testing.B) {
	ws := newBenchWheelShard(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			timer := ws.NewTimer(benchDelay)
			stopReleaseTimer(b, timer)
		}
	})
}
