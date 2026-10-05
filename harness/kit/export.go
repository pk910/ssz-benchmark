package bench

import (
	"encoding/binary"
	"os"
	"sort"
	"strconv"

	"golang.org/x/sys/unix"
)

// ProcCounters are the kit's hardware counters opened on threads of
// another process: what benchwrap switches on and off for an adapter of
// another language, on the adapter's messages, as the kit does for its
// own loop.
type ProcCounters struct {
	c   *counters
	pid int
}

// ProcThreadIDs lists the threads of a process.
func ProcThreadIDs(pid int) []int {
	entries, _ := os.ReadDir("/proc/" + strconv.Itoa(pid) + "/task")
	tids := make([]int, 0, len(entries))
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil {
			tids = append(tids, n)
		}
	}
	sort.Ints(tids)
	return tids
}

// OpenProcCounters opens cycles, instructions and the pair BENCH_COUNTERS
// names on one thread of a process, or with all on every thread it has at
// this moment, disabled. Nil when the machine has no counters.
func OpenProcCounters(pid, tid int, all bool) *ProcCounters {
	targets := []int{tid}
	var tids []int
	if all {
		tids = ProcThreadIDs(pid)
		targets = tids
	}
	c := openOn(targets, tids)
	if c == nil {
		return nil
	}
	return &ProcCounters{c: c, pid: pid}
}

// OK reports whether cycles and instructions are counted.
func (p *ProcCounters) OK() bool { return p != nil && p.c.ok() }

// Reset zeroes, Start enables and Stop disables the counters.
func (p *ProcCounters) Reset() {
	if p != nil {
		p.c.reset()
	}
}

// Start enables the counters.
func (p *ProcCounters) Start() {
	if p != nil {
		p.c.start()
	}
}

// Stop disables the counters.
func (p *ProcCounters) Stop() {
	if p != nil {
		p.c.stop()
	}
}

// Close releases the counters.
func (p *ProcCounters) Close() {
	if p != nil {
		p.c.close()
	}
}

// Threads is how many threads are counted; 0 for one thread.
func (p *ProcCounters) Threads() int {
	if p == nil {
		return 0
	}
	return len(p.c.tids)
}

// Complete reports whether every thread of the process was counted: with
// all threads counted, none may have appeared since the counters opened.
func (p *ProcCounters) Complete() bool {
	if p == nil || p.c.tids == nil {
		return true
	}
	now := ProcThreadIDs(p.pid)
	if len(now) != len(p.c.tids) {
		return false
	}
	for i := range now {
		if now[i] != p.c.tids[i] {
			return false
		}
	}
	return true
}

// Values reads every counter, summed over the counted threads, by the
// unit it is reported under.
func (p *ProcCounters) Values() map[string]uint64 {
	out := map[string]uint64{}
	if p == nil {
		return out
	}
	var buf [8]byte
	for i, ev := range p.c.events {
		var sum uint64
		for _, fd := range p.c.fds[i] {
			if n, err := unix.Read(fd, buf[:]); err == nil && n == 8 {
				sum += binary.LittleEndian.Uint64(buf[:])
			}
		}
		out[ev.unit] = sum
	}
	return out
}

// PinThread pins a thread of another process to one cpu, as pinMutator
// pins the kit's own measuring thread.
func PinThread(tid, cpu int) error {
	var set unix.CPUSet
	set.Set(cpu)
	return unix.SchedSetaffinity(tid, &set)
}
