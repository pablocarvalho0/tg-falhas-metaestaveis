package main

import (
	"syscall"
	"unsafe"
)

// fixarNaCPU restringe a thread atual a uma CPU (sched_setaffinity). Se a
// chamada falhar, a thread continua solta: a vigia ainda funciona, só deixa
// de garantir a cobertura de todas as CPUs.
func fixarNaCPU(cpu int) {
	var mascara [16]uint64 // 1024 CPUs
	mascara[cpu/64] |= 1 << (uint(cpu) % 64)
	_, _, _ = syscall.RawSyscall(syscall.SYS_SCHED_SETAFFINITY, 0, unsafe.Sizeof(mascara), uintptr(unsafe.Pointer(&mascara)))
}
