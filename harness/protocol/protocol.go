// Package protocol is the adapter side of benchwrap's protocol for a Go
// program: the pattern and iteration counts the wrapper passes, the
// messages on file descriptor 3, and the measuring loop with the kit's
// policy (untimed warm-up calls, forced collections between batches with
// the clock and the counters paused). The adapters of other languages
// implement the same in theirs; this one validates the wrapper against the
// kit.
package protocol

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Leaf is one benchmark: an engine, an object and an operation.
type Leaf struct{ Engine, Object, Op string }

func (l Leaf) String() string { return l.Engine + "/" + l.Object + "/" + l.Op }

// Matches reports whether the leaf matches the pattern with Go testing's
// semantics: the pattern is split at "/", each element is a regular
// expression matched against the element of the benchmark name
// "BenchmarkReal/<Engine>/<Object>/<Op>" at the same position, and
// elements the pattern does not have match everything.
func Matches(pattern string, l Leaf) bool {
	if pattern == "" {
		pattern = "."
	}
	name := []string{"BenchmarkReal", l.Engine, l.Object, l.Op}
	for i, elem := range strings.Split(pattern, "/") {
		if i >= len(name) {
			return true
		}
		re, err := regexp.Compile(elem)
		if err != nil || !re.MatchString(name[i]) {
			return false
		}
	}
	return true
}

// Pattern is the benchmark pattern the wrapper passed.
func Pattern() string { return os.Getenv("BENCH_PATTERN") }

// Iters is the fixed iteration count of an operation from BENCH_ITERS
// ("Op=N,Op=N"), 0 when the operation has none (calibrate instead).
func Iters(op string) int {
	for part := range strings.SplitSeq(os.Getenv("BENCH_ITERS"), ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(name) == op {
			n, _ := strconv.Atoi(strings.TrimSpace(val))
			return n
		}
	}
	return 0
}

// BenchTime is what BENCH_TIME asks for: a fixed number of iterations
// ("Nx"), or a duration to calibrate against.
func BenchTime() (iters int, d time.Duration) {
	v := os.Getenv("BENCH_TIME")
	if count, ok := strings.CutSuffix(v, "x"); ok {
		n, _ := strconv.Atoi(count)
		return max(n, 1), 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 1, 0
	}
	return 0, d
}

// Session writes the messages to the wrapper.
type Session struct{ w *os.File }

// Open takes the pipe on file descriptor 3. Without one (the adapter run
// by hand) the messages go to stderr.
func Open() *Session {
	w := os.NewFile(3, "benchwrap")
	if _, err := w.Stat(); err != nil {
		w = os.Stderr
	}
	return &Session{w: w}
}

func (s *Session) send(format string, args ...any) {
	fmt.Fprintf(s.w, format+"\n", args...)
}

// Thread names the calling thread as the measuring one and locks the
// goroutine to it.
func (s *Session) Thread() {
	runtime.LockOSThread()
	s.send("thread %d", unix.Gettid())
}

// AllThreads asks for every thread of the process to be counted.
func (s *Session) AllThreads() { s.send("all-threads") }

func (s *Session) Begin(l Leaf) { s.send("begin %s", l) }
func (s *Session) Pause()       { s.send("pause") }
func (s *Session) Resume()      { s.send("resume") }
func (s *Session) Fail(l Leaf, msg string) {
	s.send("fail %s %s", l, strings.ReplaceAll(msg, "\n", " "))
}
func (s *Session) Skip(l Leaf) { s.send("skip %s", l) }

// End reports a measured leaf: the iterations, the time of the timed
// windows, the bytes and allocations over all iterations (negative: not
// measured) and the untimed warm-up iterations.
func (s *Session) End(l Leaf, iters int, elapsed time.Duration, bytes, allocs int64, warmup int) {
	msg := fmt.Sprintf("end %s iters=%d ns=%d", l, iters, elapsed.Nanoseconds())
	if bytes >= 0 {
		msg += fmt.Sprintf(" bytes=%d allocs=%d", bytes, allocs)
	}
	if warmup > 0 {
		msg += fmt.Sprintf(" warmup=%d", warmup)
	}
	s.send("%s", msg)
}

// gcBatch bounds the garbage allowed to pile up between the collections
// the loop forces, as in the kit.
const gcBatch = 256 << 20

var allocSamples = []metrics.Sample{
	{Name: "/gc/heap/allocs:bytes"},
	{Name: "/gc/heap/allocs:objects"},
	{Name: "/gc/heap/tiny/allocs:objects"},
}

func readAllocs() (bytes, objects uint64) {
	metrics.Read(allocSamples)
	return allocSamples[0].Value.Uint64(), allocSamples[1].Value.Uint64() + allocSamples[2].Value.Uint64()
}

// Run measures one leaf with the kit's policy: untimed warm-up calls (two
// when the operation allocates, so the loop alternates between two heap
// regions), then the fixed iterations, or a count grown as Go's testing
// does until a batch reaches the benchtime, with a collection forced
// between batches of iterations with the clock and the counters paused.
// The collector is expected off (GOGC=off), as the runner sets it.
func (s *Session) Run(l Leaf, f func()) {
	n := Iters(l.Op)
	fixed, target := BenchTime()
	if n == 0 {
		n = fixed
	}
	warm := 1
	w0, _ := readAllocs()
	f()
	runtime.GC()
	if w1, _ := readAllocs(); w1 != w0 {
		f()
		runtime.GC()
		warm = 2
	}
	for {
		iters, elapsed, bytes, allocs := s.timed(l, n, f)
		if target == 0 || elapsed >= target {
			s.End(l, iters, elapsed, int64(bytes), int64(allocs), warm)
			return
		}
		// Grow as testing.B does: 1, 2, 5, 10, 20, 50, ...
		next := int(float64(n) * float64(target) / float64(max(elapsed, time.Nanosecond)) * 1.2)
		next = roundUp(max(next, n+1))
		n = next
	}
}

// timed runs f n times with collections between batches, counters and
// clock paused around them, and returns the figures.
func (s *Session) timed(l Leaf, n int, f func()) (int, time.Duration, uint64, uint64) {
	every := 1
	var elapsed time.Duration
	var sumBytes, sumObjs uint64
	b0, o0 := readAllocs()
	s.Begin(l)
	start := time.Now()
	for i := range n {
		f()
		gc := i+1 < n && (i == 0 || (i+1)%every == 0)
		if !gc && i+1 < n {
			continue
		}
		elapsed += time.Since(start)
		b1, o1 := readAllocs()
		sumBytes += b1 - b0
		sumObjs += o1 - o0
		if i == 0 {
			switch {
			case sumBytes == 0:
				every = int(^uint(0) >> 1)
			case sumBytes < gcBatch:
				every = int(gcBatch / sumBytes)
			}
			gc = n > 1
		}
		if gc {
			s.Pause()
			runtime.GC()
			b0, o0 = readAllocs()
			s.Resume()
		}
		start = time.Now()
	}
	return n, elapsed, sumBytes, sumObjs
}

// roundUp rounds an iteration count up to 1, 2, 5 times a power of ten.
func roundUp(n int) int {
	base := 1
	for base*10 <= n {
		base *= 10
	}
	switch {
	case n <= base:
		return base
	case n <= 2*base:
		return 2 * base
	case n <= 5*base:
		return 5 * base
	}
	return 10 * base
}
