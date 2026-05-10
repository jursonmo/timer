package timer

import (
	_ "unsafe"
)

////go:linkname 调 runtime 私有 API，Go 版本兼容性风险很高。

//go:linkname procPin runtime.procPin
func procPin() int

//go:linkname procUnpin runtime.procUnpin
func procUnpin()

func GetPid() int {
	pid := procPin() //this goroutine will not be scheduled  when pinned, gc can't stop this goroutine
	procUnpin()
	return pid
}
