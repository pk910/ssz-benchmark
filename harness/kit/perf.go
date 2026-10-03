package bench

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// counters are hardware performance counters of the measured thread:
// cycles and retired instructions. Cycles per operation do not move with
// the clock frequency the hypervisor happens to grant, and instructions
// per operation are nearly deterministic for the same binary, so both are
// reported next to the wall time. They stay silent when the machine does
// not expose a PMU.
type counters struct {
	fds []int
}

// openCounters opens the counters on the calling thread (which pinMutator
// locked), disabled, to be switched on and off with the benchmark timer.
func openCounters() *counters {
	c := &counters{}
	for _, cfg := range []uint64{unix.PERF_COUNT_HW_CPU_CYCLES, unix.PERF_COUNT_HW_INSTRUCTIONS} {
		attr := unix.PerfEventAttr{
			Type:   unix.PERF_TYPE_HARDWARE,
			Config: cfg,
			Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
			Bits:   unix.PerfBitDisabled | unix.PerfBitExcludeHv | unix.PerfBitExcludeKernel,
		}
		fd, err := unix.PerfEventOpen(&attr, 0, -1, -1, unix.PERF_FLAG_FD_CLOEXEC)
		if err != nil {
			c.close()
			return nil
		}
		c.fds = append(c.fds, fd)
	}
	return c
}

func (c *counters) close() {
	if c == nil {
		return
	}
	for _, fd := range c.fds {
		_ = unix.Close(fd)
	}
	c.fds = nil
}

func (c *counters) ioctl(req uint) {
	if c == nil {
		return
	}
	for _, fd := range c.fds {
		_ = unix.IoctlSetInt(fd, req, 0)
	}
}

func (c *counters) reset()   { c.ioctl(unix.PERF_EVENT_IOC_RESET) }
func (c *counters) start()   { c.ioctl(unix.PERF_EVENT_IOC_ENABLE) }
func (c *counters) stop()    { c.ioctl(unix.PERF_EVENT_IOC_DISABLE) }
func (c *counters) ok() bool { return c != nil && len(c.fds) == 2 }

func (c *counters) read() (cycles, instrs uint64) {
	if !c.ok() {
		return 0, 0
	}
	var buf [8]byte
	vals := [2]uint64{}
	for i, fd := range c.fds {
		if n, err := unix.Read(fd, buf[:]); err == nil && n == 8 {
			vals[i] = binary.LittleEndian.Uint64(buf[:])
		}
	}
	return vals[0], vals[1]
}

// report adds cycles/op and instrs/op to the benchmark result.
func (c *counters) report(b *testing.B) {
	if !c.ok() || b.N == 0 {
		return
	}
	cycles, instrs := c.read()
	b.ReportMetric(float64(cycles)/float64(b.N), "cycles/op")
	b.ReportMetric(float64(instrs)/float64(b.N), "instrs/op")
}

// threadKernel returns the page faults (minor and major) and the kernel
// time in nanoseconds the calling thread has accumulated.
func threadKernel() (faults, sysNs int64) {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_THREAD, &ru); err != nil {
		return 0, 0
	}
	return int64(ru.Minflt + ru.Majflt), int64(ru.Stime.Sec)*1e9 + int64(ru.Stime.Usec)*1e3
}
