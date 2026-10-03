package bench

import (
	"os"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

// pinMutator locks the calling goroutine to its thread and pins that
// thread to the cpu named by BENCH_MUTATOR_CPU, so the measured loop never
// migrates between cores; the runtime's other threads (GC workers, async
// hashing workers) stay free to use the rest of the process cpuset.
func pinMutator() {
	cpu, err := strconv.Atoi(os.Getenv("BENCH_MUTATOR_CPU"))
	if err != nil {
		return
	}
	runtime.LockOSThread()
	var set unix.CPUSet
	set.Set(cpu)
	_ = unix.SchedSetaffinity(0, &set)
}
