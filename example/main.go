package main

import (
	"fmt"
	"log"
	"time"

	"github.com/jursonmo/timer"
)

func main() {
	testWheel()
	testWheelShard()
}

func testWheel() {
	var testWheel = timer.NewWheel(1 * time.Millisecond)

	// check timer
	t1 := testWheel.NewTimer(500 * time.Millisecond)

	before := time.Now()
	<-t1.C

	after := time.Now()
	println(after.Sub(before).String())

	ok := t1.Stop()
	if ok {
		log.Fatal("should not ok")
	}
	//t1 have timeout , now we can release timer to pool
	t1.Release() //timer.Stop()成功的timer 和 已经超时的timer, 这样才能Release()

	//check timer.Stop()
	go func() {
		t2 := testWheel.NewTimer(10 * time.Millisecond)
		ok := t2.Stop()
		if !ok {
			log.Fatal("should ok")
		}
		before = time.Now()
		<-t2.C //t2 已经Stop 了，会一直阻塞
		log.Fatal("can't be here")
	}()

	//check NewWheelTimerFunc timer
	arg0 := "arg0"
	arg1 := "arg1"
	start := time.Now()
	var t3 *timer.WheelTimer
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
	}
	t3 = testWheel.NewWheelTimerFunc(time.Millisecond*10, f, arg0, arg1)

	time.Sleep(time.Second)
}

func testWheelShard() {
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
