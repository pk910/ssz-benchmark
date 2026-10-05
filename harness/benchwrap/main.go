// benchwrap runs an adapter of another language the way the runner runs
// a Go benchmark binary: it takes the Go test flags the runner passes,
// starts the adapter with the pattern and the iteration counts in its
// environment, switches the hardware counters of the adapter's measuring
// thread on its messages, and prints one Go benchmark result line per
// leaf, so the runner parses the output as it parses the kit's.
//
//	benchwrap -adapter <launcher> [-test.bench <pattern>] [-test.benchtime <d>] [-test.timeout <d>] [-- <adapter args>]
//
// The adapter gets BENCH_PATTERN (the -test.bench pattern, Go testing
// semantics: split at "/", each element matched against the element of
// the benchmark name), BENCH_TIME (the benchtime, "1x" for one iteration
// or a duration to calibrate against), BENCH_ITERS and the rest of the
// runner's environment, and a pipe on file descriptor 3 for its messages:
//
//	thread <tid>                      the measuring thread; counters open on it, and it
//	                                  is pinned to BENCH_MUTATOR_CPU
//	all-threads                       count every thread of the process instead
//	begin <Engine>/<Object>/<Op>      the timed loop starts: counters on
//	pause / resume                    around a collection between batches: counters off and on
//	end <leaf> iters=<n> ns=<elapsed> [bytes=<B>] [allocs=<N>] [warmup=<k>]
//	                                  the loop ended: counters off and read, the line printed;
//	                                  bytes and allocs are totals over the iterations
//	fail <leaf> <message>             the adapter's own check failed: reported as the kit
//	                                  reports it, the run fails at the end
//	skip <leaf>                       the library has no such operation
//
// Anything the adapter writes to its stdout or stderr goes to stderr here,
// so the result lines on stdout stay clean.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	bench "benchkit"
)

func main() {
	var adapter, pattern, benchTime, timeout string
	flag.StringVar(&adapter, "adapter", "", "the adapter to run")
	flag.StringVar(&pattern, "test.bench", ".", "benchmark pattern")
	flag.StringVar(&benchTime, "test.benchtime", "1x", "iterations (Nx) or duration per leaf")
	flag.StringVar(&timeout, "test.timeout", "60m", "time limit of the whole run")
	flag.String("test.run", "", "ignored")
	flag.String("test.count", "", "ignored")
	flag.Bool("test.benchmem", false, "ignored")
	flag.Parse()
	if adapter == "" {
		fmt.Fprintln(os.Stderr, "benchwrap: -adapter is required")
		os.Exit(2)
	}
	limit, err := time.ParseDuration(timeout)
	if err != nil {
		limit = time.Hour
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	r, w, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchwrap:", err)
		os.Exit(2)
	}
	cmd := exec.CommandContext(ctx, adapter, flag.Args()...)
	cmd.Env = append(os.Environ(), "BENCH_PATTERN="+pattern, "BENCH_TIME="+benchTime)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	cmd.ExtraFiles = []*os.File{w}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "benchwrap:", err)
		os.Exit(2)
	}
	w.Close()
	s := &session{pid: cmd.Process.Pid, out: bufio.NewWriter(os.Stdout)}
	s.read(r)
	s.out.Flush()
	werr := cmd.Wait()
	if werr != nil {
		fmt.Fprintln(os.Stderr, "benchwrap: adapter:", werr)
		os.Exit(1)
	}
	if s.failed {
		os.Exit(1)
	}
}

// session is the state of one adapter run.
type session struct {
	pid      int
	tid      int
	all      bool
	counters *bench.ProcCounters
	out      *bufio.Writer
	failed   bool
	// The timed windows of the current leaf.
	leaf    string
	open    bool
	faults  int64 // page faults of the measuring thread in the windows
	sysNs   int64 // kernel time in the windows
	f0, s0  int64 // at the start of the current window
	started bool
}

func (s *session) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		cmd, rest, _ := strings.Cut(line, " ")
		switch cmd {
		case "thread":
			s.tid, _ = strconv.Atoi(rest)
			if cpu, err := strconv.Atoi(os.Getenv("BENCH_MUTATOR_CPU")); err == nil && s.tid > 0 {
				_ = bench.PinThread(s.tid, cpu)
			}
			s.openCounters()
		case "all-threads":
			s.all = true
			s.openCounters()
		case "begin":
			s.leaf = rest
			s.faults, s.sysNs = 0, 0
			s.started = true
			s.counters.Reset()
			s.resume()
		case "pause":
			s.pause()
		case "resume":
			s.resume()
		case "end":
			s.end(rest)
		case "fail":
			leaf, msg, _ := strings.Cut(rest, " ")
			fmt.Fprintf(s.out, "--- FAIL: BenchmarkReal/%s\n    %s\n", leaf, msg)
			s.failed = true
		case "skip":
		default:
			fmt.Fprintf(os.Stderr, "benchwrap: unknown message %q\n", line)
		}
	}
}

func (s *session) openCounters() {
	if s.counters != nil {
		s.counters.Close()
	}
	tid := s.tid
	if tid == 0 {
		tid = s.pid
	}
	s.counters = bench.OpenProcCounters(s.pid, tid, s.all || os.Getenv("BENCH_ALL_THREADS") != "")
}

func (s *session) resume() {
	if s.open || !s.started {
		return
	}
	s.f0, s.s0 = s.kernel()
	if s.f0 < 0 {
		s.f0, s.s0 = 0, 0
	}
	s.counters.Start()
	s.open = true
}

func (s *session) pause() {
	if !s.open {
		return
	}
	s.counters.Stop()
	// An adapter that has already exited leaves no figures to read: the
	// window's faults and kernel time are then unknown and left out,
	// rather than made negative.
	if f1, s1 := s.kernel(); f1 >= 0 {
		s.faults += f1 - s.f0
		s.sysNs += s1 - s.s0
	}
	s.open = false
}

// end prints the result line of a leaf from the adapter's figures and the
// counters.
func (s *session) end(rest string) {
	s.pause()
	s.started = false
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return
	}
	leaf := fields[0]
	kv := map[string]float64{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		kv[k], _ = strconv.ParseFloat(v, 64)
	}
	n := kv["iters"]
	if n <= 0 {
		fmt.Fprintf(os.Stderr, "benchwrap: %s: no iteration count\n", leaf)
		return
	}
	fmt.Fprintf(s.out, "BenchmarkReal/%s-1\t%d\t%.1f ns/op", leaf, int(n), kv["ns"]/n)
	if _, ok := kv["bytes"]; ok {
		fmt.Fprintf(s.out, "\t%.0f B/op\t%.0f allocs/op", kv["bytes"]/n, kv["allocs"]/n)
	}
	if s.counters.OK() {
		if s.counters.Complete() {
			vals := s.counters.Values()
			for _, unit := range counterUnits(vals) {
				fmt.Fprintf(s.out, "\t%.1f %s", float64(vals[unit])/n, unit)
			}
			if t := s.counters.Threads(); t > 0 {
				fmt.Fprintf(s.out, "\t%d threads", t)
			}
		} else {
			fmt.Fprintf(s.out, "\t1 thread-drift")
		}
	}
	fmt.Fprintf(s.out, "\t%d iters\t%.2f faults/op\t%.1f sys-ns/op", int(n), float64(s.faults)/n, float64(s.sysNs)/n)
	if w, ok := kv["warmup"]; ok {
		fmt.Fprintf(s.out, "\t%.0f warmup", w)
	}
	fmt.Fprintln(s.out)
	s.out.Flush()
}

// counterUnits orders the units: cycles and instructions first, then the
// pair, as the kit reports them.
func counterUnits(vals map[string]uint64) []string {
	order := []string{"cycles/op", "instrs/op"}
	var rest []string
	for u := range vals {
		if u != "cycles/op" && u != "instrs/op" {
			rest = append(rest, u)
		}
	}
	for i := 0; i < len(rest); i++ {
		for j := i + 1; j < len(rest); j++ {
			if rest[j] < rest[i] {
				rest[i], rest[j] = rest[j], rest[i]
			}
		}
	}
	var out []string
	for _, u := range append(order, rest...) {
		if _, ok := vals[u]; ok {
			out = append(out, u)
		}
	}
	return out
}

// clockTick is the kernel's clock tick for the times in /proc, in
// nanoseconds (CLK_TCK is 100 on this machine's kernel configuration).
const clockTick = 10_000_000

// kernel reads the page faults and the kernel time of the measuring
// thread from /proc, the figures the kit reads with getrusage for its own
// thread.
func (s *session) kernel() (faults, sysNs int64) {
	tid := s.tid
	if tid == 0 {
		tid = s.pid
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(s.pid) + "/task/" + strconv.Itoa(tid) + "/stat")
	if err != nil {
		return -1, -1
	}
	// The command name is in parentheses and may hold spaces: the fields
	// after it start with the state (field 3).
	i := strings.LastIndexByte(string(data), ')')
	if i < 0 {
		return -1, -1
	}
	f := strings.Fields(string(data[i+1:]))
	if len(f) < 13 {
		return -1, -1
	}
	minflt, _ := strconv.ParseInt(f[7], 10, 64)
	majflt, _ := strconv.ParseInt(f[9], 10, 64)
	stime, _ := strconv.ParseInt(f[12], 10, 64)
	return minflt + majflt, stime * clockTick
}
