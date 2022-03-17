package timer

import (
	_ "unsafe"
)

//go:linkname procPin runtime.procPin
func procPin() int

//go:linkname procUnpin runtime.procUnpin
func procUnpin()

func GetPid() int {
	pid := procPin() //this goroutine will not be scheduled  when pinned, gc can't stop this goroutine
	procUnpin()
	return pid
}
