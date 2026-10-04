package bench

import (
	"encoding/binary"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// event is one hardware counter and the unit its per-operation value is
// reported under.
type event struct {
	unit   string
	typ    uint32
	config uint64
	zen    bool // a raw event of the AMD Zen core PMU
}

func cacheMiss(id uint64) uint64 {
	return id | unix.PERF_COUNT_HW_CACHE_OP_READ<<8 | unix.PERF_COUNT_HW_CACHE_RESULT_MISS<<16
}

// Cycles and retired instructions are counted in every run. Cycles per
// operation do not move with the clock frequency the hypervisor happens to
// grant, and instructions per operation are nearly deterministic for the
// same binary, so both are reported next to the wall time.
var baseEvents = []event{
	{unit: "cycles/op", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_CPU_CYCLES},
	{unit: "instrs/op", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_INSTRUCTIONS},
}

// counterSets are the pairs of further counters BENCH_COUNTERS selects by
// name. The machine runs four counters at once, so a run counts one pair
// next to cycles and instructions.
var counterSets = map[string][]event{
	"A": {
		{unit: "br-miss/op", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_BRANCH_MISSES},
		{unit: "l2-miss/op", typ: unix.PERF_TYPE_RAW, config: 0x0864, zen: true},
	},
	"B": {
		{unit: "fe-stall/op", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_STALLED_CYCLES_FRONTEND},
		{unit: "l1d-miss/op", typ: unix.PERF_TYPE_HW_CACHE, config: cacheMiss(unix.PERF_COUNT_HW_CACHE_L1D)},
	},
	"D1": {
		{unit: "dec-uops/op", typ: unix.PERF_TYPE_RAW, config: 0x01aa, zen: true},
		{unit: "l1i-miss/op", typ: unix.PERF_TYPE_HW_CACHE, config: cacheMiss(unix.PERF_COUNT_HW_CACHE_L1I)},
	},
	"D2": {
		{unit: "dtlb-miss/op", typ: unix.PERF_TYPE_RAW, config: 0xff45, zen: true},
		{unit: "itlb-miss/op", typ: unix.PERF_TYPE_RAW, config: 0x0784, zen: true},
	},
}

// isZen reports whether the cpu is an AMD family 17h or later, whose raw
// event codes the counter sets use.
var isZen = func() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	text := string(data)
	if !strings.Contains(text, "AuthenticAMD") {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "cpu family") {
			_, val, _ := strings.Cut(line, ":")
			fam, _ := strconv.Atoi(strings.TrimSpace(val))
			return fam >= 23
		}
	}
	return false
}()

// counters are hardware performance counters, user mode only, of the
// measured thread or of every thread of the process. They stay silent when
// the machine does not expose a PMU.
type counters struct {
	events []event
	fds    [][]int // per event, one descriptor per counted thread
	tids   []int   // the threads counted; nil for the calling thread only
}

func openEvent(ev event, tid int) (int, error) {
	attr := unix.PerfEventAttr{
		Type:   ev.typ,
		Config: ev.config,
		Size:   uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		Bits:   unix.PerfBitDisabled | unix.PerfBitExcludeHv | unix.PerfBitExcludeKernel,
	}
	return unix.PerfEventOpen(&attr, tid, -1, -1, unix.PERF_FLAG_FD_CLOEXEC)
}

// openCounters opens cycles, instructions and the pair BENCH_COUNTERS
// names, disabled, to be switched on and off with the benchmark timer:
// on the calling thread (which pinMutator locked), or with allThreads on
// every thread the process has at this moment. Cycles and instructions
// must open; a counter of the pair that the machine lacks is left out.
func openCounters(allThreads bool) *counters {
	c := &counters{}
	targets := []int{0}
	if allThreads {
		c.tids = threadIDs()
		targets = c.tids
	}
	events := append([]event{}, baseEvents...)
	for _, ev := range counterSets[os.Getenv("BENCH_COUNTERS")] {
		if !ev.zen || isZen {
			events = append(events, ev)
		}
	}
	for i, ev := range events {
		var fds []int
		for _, tid := range targets {
			fd, err := openEvent(ev, tid)
			if err != nil {
				for _, fd := range fds {
					_ = unix.Close(fd)
				}
				fds = nil
				break
			}
			fds = append(fds, fd)
		}
		if fds == nil {
			if i < len(baseEvents) {
				c.close()
				return nil
			}
			continue
		}
		c.events = append(c.events, ev)
		c.fds = append(c.fds, fds)
	}
	return c
}

// threadIDs lists the threads of the process.
func threadIDs() []int {
	entries, _ := os.ReadDir("/proc/self/task")
	var tids []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			tids = append(tids, n)
		}
	}
	sort.Ints(tids)
	return tids
}

func (c *counters) close() {
	if c == nil {
		return
	}
	for _, fds := range c.fds {
		for _, fd := range fds {
			_ = unix.Close(fd)
		}
	}
	c.fds, c.events = nil, nil
}

func (c *counters) ioctl(req uint) {
	if c == nil {
		return
	}
	for _, fds := range c.fds {
		for _, fd := range fds {
			_ = unix.IoctlSetInt(fd, req, 0)
		}
	}
}

func (c *counters) reset()   { c.ioctl(unix.PERF_EVENT_IOC_RESET) }
func (c *counters) start()   { c.ioctl(unix.PERF_EVENT_IOC_ENABLE) }
func (c *counters) stop()    { c.ioctl(unix.PERF_EVENT_IOC_DISABLE) }
func (c *counters) ok() bool { return c != nil && len(c.fds) >= len(baseEvents) }

// complete reports whether every thread of the process was counted: with
// all threads counted, none may have appeared since the counters opened.
func (c *counters) complete() bool {
	if c.tids == nil {
		return true
	}
	now := threadIDs()
	if len(now) != len(c.tids) {
		return false
	}
	for i := range now {
		if now[i] != c.tids[i] {
			return false
		}
	}
	return true
}

// report adds every counter per operation to the benchmark result; with
// all threads counted also their number, and nothing at all when a thread
// escaped the count.
func (c *counters) report(b *testing.B, n int) {
	if !c.ok() || n == 0 {
		return
	}
	if !c.complete() {
		b.ReportMetric(1, "thread-drift")
		return
	}
	var buf [8]byte
	for i, ev := range c.events {
		var sum uint64
		for _, fd := range c.fds[i] {
			if n, err := unix.Read(fd, buf[:]); err == nil && n == 8 {
				sum += binary.LittleEndian.Uint64(buf[:])
			}
		}
		b.ReportMetric(float64(sum)/float64(n), ev.unit)
	}
	if c.tids != nil {
		b.ReportMetric(float64(len(c.tids)), "threads")
	}
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
