// Package bench holds the benchmark body shared by the fork packages: a
// codec abstracts one engine's entry points for one object type, and
// RunOne / RunSet measure every operation of a codec on a payload.
package bench

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Codec is one engine's set of entry points for one object type. A nil
// function means the engine has no such operation (baselines have no
// streaming).
type Codec struct {
	Only            map[string]bool // when set, only these operations are measured
	New             func() any
	Unmarshal       func(obj any, data []byte) error
	UnmarshalReader func(obj any, r io.Reader, size int) error
	Size            func(obj any) (int, error)
	Marshal         func(obj any) ([]byte, error)
	MarshalTo       func(obj any, buf []byte) ([]byte, error)
	MarshalWriter   func(obj any, w io.Writer) error
	Root            func(obj any) ([32]byte, error)
	Tree            func(obj any) ([]byte, error)
}

// Payload is one object file with its hash tree root, or a set of them.
func (c Codec) has(op string) bool { return c.Only == nil || c.Only[op] }

// run is b.Run gated on the codec's operation set; the measured goroutine
// is pinned before the loop starts.
func run(b *testing.B, c Codec, op string, f func(b *testing.B)) {
	if c.has(op) {
		b.Run(op, func(b *testing.B) {
			pinMutator()
			currentOp = op
			f(b)
		})
	}
}

// currentOp is the operation whose loop runs; fixedIters holds the
// iteration counts BENCH_ITERS prescribes per operation ("Op=N,Op=N").
var (
	currentOp  string
	fixedIters = parseIters(os.Getenv("BENCH_ITERS"))
)

func parseIters(spec string) map[string]int {
	out := map[string]int{}
	for _, part := range strings.Split(spec, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			out[strings.TrimSpace(name)] = n
		}
	}
	return out
}

type Payload struct {
	Data  []byte
	Root  [32]byte
	Set   [][]byte
	Roots [][32]byte
}

// DataDir is the payload root: REAL_DATA, default /srv/benchd/res/real.
// Fork packages read their own subdirectory of it.
func DataDir() string {
	if dir := os.Getenv("REAL_DATA"); dir != "" {
		return dir
	}
	return "/srv/benchd/res/real"
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}

func readRoot(path string) [32]byte {
	raw := strings.TrimPrefix(strings.TrimSpace(string(mustRead(path))), "0x")
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != 32 {
		panic("bad root file " + path)
	}
	var root [32]byte
	copy(root[:], b)
	return root
}

// LoadSpecs turns the beacon API config/spec response into the map a
// downstream user hands to NewDynSsz: every decimal value becomes a uint64,
// everything else (hex, names, lists) is left out.
func LoadSpecs(path string) map[string]any {
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(mustRead(path), &resp); err != nil {
		panic(err)
	}
	specs := make(map[string]any, len(resp.Data))
	for k, v := range resp.Data {
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				specs[k] = n
			}
		}
	}
	return specs
}

// LoadOne reads <dir>/<name>.ssz and its root.
func LoadOne(dir, name string) Payload {
	return Payload{Data: mustRead(filepath.Join(dir, name+".ssz")), Root: readRoot(filepath.Join(dir, name+".root"))}
}

// LoadSet reads every .ssz file of <dir>/<name>/ with its root, in name order.
func LoadSet(dir, name string) Payload {
	files, _ := filepath.Glob(filepath.Join(dir, name, "*.ssz"))
	sort.Strings(files)
	var p Payload
	for _, f := range files {
		p.Set = append(p.Set, mustRead(f))
		p.Roots = append(p.Roots, readRoot(strings.TrimSuffix(f, ".ssz")+".root"))
	}
	if len(p.Set) == 0 {
		panic(filepath.Join(dir, name) + ": no files")
	}
	return p
}

// gcBatch bounds the garbage allowed to pile up between the harness's own
// collections.
const gcBatch = 256 << 20

// iterate runs f b.N times with the collector off the timed path. The
// runtime's own cycles are disabled (GOGC=off, set by the runner); the
// harness collects between iterations with the timer stopped, every
// iteration when one allocates more than gcBatch, else often enough to keep
// the garbage of an iteration batch under gcBatch. The heap therefore stays
// warm and reused, no cycle lands inside a measurement, and the cost of the
// garbage shows in the B/op and allocs/op columns instead of in the time.
func iterate(b *testing.B, f func()) {
	if n, ok := fixedIters[currentOp]; ok {
		iterateFixed(b, n, f)
		return
	}
	every := 1
	pause := func() { b.StopTimer(); perf.stop() }
	resume := func() { perf.start(); b.StartTimer() }
	for i := 0; i < b.N; i++ {
		if i == 1 {
			// The first iteration tells how much one allocates.
			pause()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			perIter := ms.TotalAlloc - baseAlloc
			switch {
			case perIter == 0:
				every = math.MaxInt // no garbage, nothing to collect
			case perIter < gcBatch:
				every = int(gcBatch / perIter)
			}
			runtime.GC()
			resume()
		} else if i > 1 && i%every == 0 {
			pause()
			runtime.GC()
			resume()
		}
		f()
	}
	perf.stop()
	b.StopTimer()
	perf.report(b, b.N)
	b.StartTimer()
}

// iterateFixed runs f exactly n times regardless of b.N (the process is
// invoked with -test.benchtime 1x), keeping its own clock and allocation
// counters, and reports ns/op, B/op, allocs/op (and the hardware
// counters) itself. The clock and the counters run across the iterations
// and pause only around the collections between iteration batches, as in
// iterate: toggling the counters costs a hypervisor trap per call, which
// must not land in the per-iteration figures of a cheap operation. Several
// operations with different counts can so share one process.
//
// Allocations are read from runtime/metrics around the collections, so
// what the collector itself allocates is never attributed to the
// operation; for a loop of at most perIterStatsMax iterations they are
// read after every iteration and the operation's figure is the smallest
// iteration, since a one-off allocation of the runtime landing in one
// iteration can only add.
func iterateFixed(b *testing.B, n int, f func()) {
	b.StopTimer()
	// Untimed calls first: they grow the heap to what the operation needs
	// and fault those pages in, so the measured iterations reuse mapped
	// memory. Without them the first measured iteration pays the kernel
	// for every fresh page (a quarter of the wall time of a state decode
	// measured over two iterations). An allocating operation needs two:
	// the result of one call is still referenced while the next one runs,
	// so the loop alternates between two regions and both must exist.
	warm0, _ := readAllocs()
	f()
	runtime.GC()
	if warm1, _ := readAllocs(); warm1 != warm0 {
		f()
		runtime.GC()
	}
	if os.Getenv("BENCH_ALL_THREADS") != "" {
		// The warm-up calls made the runtime start the threads the
		// operation works on; all of them are counted from here.
		perf.close()
		perf = openCounters(true)
	}
	var elapsed time.Duration
	every := 1
	perIterStats := n <= perIterStatsMax
	bytes0, objs0 := readAllocs()
	minBytes, minObjs := ^uint64(0), ^uint64(0)
	var sumBytes, sumObjs uint64
	record := func() {
		bb, oo := readAllocs()
		db, do := bb-bytes0, oo-objs0
		sumBytes += db
		sumObjs += do
		if do < minObjs || (do == minObjs && db < minBytes) {
			minObjs, minBytes = do, db
		}
		bytes0, objs0 = bb, oo
	}
	// Page faults and kernel time of the measured thread inside the timed
	// windows: memory the operation touches for the first time is paid for
	// in the kernel, which the user-mode cycle counter does not see.
	var faults, sysNs int64
	f0, s0 := threadKernel()
	perf.reset()
	perf.start()
	start := time.Now()
	pause := func() {
		elapsed += time.Since(start)
		perf.stop()
		f1, s1 := threadKernel()
		faults += f1 - f0
		sysNs += s1 - s0
	}
	resume := func() {
		f0, s0 = threadKernel()
		perf.start()
		start = time.Now()
	}
	for i := 0; i < n; i++ {
		f()
		gc := i+1 < n && (i == 0 || (i+1)%every == 0)
		if !gc && !perIterStats && i+1 < n {
			continue
		}
		pause()
		if perIterStats || i == 0 || gc || i+1 == n {
			record()
		}
		if i == 0 {
			switch {
			case sumBytes == 0:
				every = math.MaxInt
			case sumBytes < gcBatch:
				every = int(gcBatch / sumBytes)
			}
			gc = n > 1
		}
		if gc {
			runtime.GC()
			bytes0, objs0 = readAllocs()
		}
		resume()
	}
	pause()
	b.ReportMetric(float64(elapsed.Nanoseconds())/float64(n), "ns/op")
	if perIterStats {
		b.ReportMetric(float64(minBytes), "B/op")
		b.ReportMetric(float64(minObjs), "allocs/op")
	} else {
		b.ReportMetric(float64(sumBytes)/float64(n), "B/op")
		b.ReportMetric(float64(sumObjs)/float64(n), "allocs/op")
	}
	b.ReportMetric(float64(n), "iters")
	b.ReportMetric(float64(faults)/float64(n), "faults/op")
	b.ReportMetric(float64(sysNs)/float64(n), "sys-ns/op")
	perf.report(b, n)
	if os.Getenv("BENCH_MEMORY") != "" {
		reportMemory(b, f)
	}
	b.StartTimer()
}

var memSamples = []metrics.Sample{
	{Name: "/memory/classes/heap/objects:bytes"},
	{Name: "/memory/classes/heap/stacks:bytes"},
}

// readMemory returns the bytes of live and not yet swept heap objects and
// the bytes of goroutine stacks.
func readMemory() (objects, stacks uint64) {
	metrics.Read(memSamples)
	return memSamples[0].Value.Uint64(), memSamples[1].Value.Uint64()
}

// reportMemory adds two untimed figures of the operation: the heap its
// result keeps alive (what is live after a collection beyond what was live
// before the loop), and the stack one call needs (the growth of the stack
// memory while a fresh goroutine, which starts with the smallest stack,
// runs it). Stacks below 32 KiB come from memory the runtime keeps reserved
// for stacks and read as zero.
func reportMemory(b *testing.B, f func()) {
	runtime.GC()
	objects, stacks := readMemory()
	retained := int64(objects) - int64(heapBase)
	if retained < 0 {
		retained = 0
	}
	b.ReportMetric(float64(retained), "retained-B/op")
	done := make(chan uint64)
	go func() {
		f()
		_, after := readMemory()
		done <- after
	}()
	grown := int64(<-done) - int64(stacks)
	if grown < 0 {
		grown = 0
	}
	b.ReportMetric(float64(grown), "stack-B/op")
}

// perIterStatsMax is the loop length up to which allocations are read
// after every iteration.
const perIterStatsMax = 64

var allocSamples = []metrics.Sample{
	{Name: "/gc/heap/allocs:bytes"},
	{Name: "/gc/heap/allocs:objects"},
	{Name: "/gc/heap/tiny/allocs:objects"},
}

// readAllocs returns the cumulative heap allocation bytes and objects
// (tiny allocations included, as in MemStats.Mallocs).
func readAllocs() (bytes, objects uint64) {
	metrics.Read(allocSamples)
	return allocSamples[0].Value.Uint64(), allocSamples[1].Value.Uint64() + allocSamples[2].Value.Uint64()
}

// baseAlloc is the allocation counter at the start of a measured loop,
// heapBase the live heap at that point; perf holds the hardware counters.
var (
	baseAlloc uint64
	heapBase  uint64
	perf      *counters
)

// startLoop resets the timer and the counters right before a measured
// loop. The counters are opened on the current thread every time: the
// testing package runs each benchmark function on a fresh goroutine (and
// so a fresh locked thread) per run.
func startLoop(b *testing.B) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	baseAlloc = ms.TotalAlloc
	heapBase, _ = readMemory()
	perf.close()
	perf = openCounters(false)
	perf.reset()
	b.ResetTimer()
	perf.start()
}

type countWriter struct {
	data []byte
}

func (w *countWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// RunOne benchmarks every operation of a codec on one payload. Every leaf
// checks its own output: decoded values must encode back to the input,
// encoded bytes must equal the input, roots must equal the stored root.
func RunOne(b *testing.B, c Codec, p Payload) {
	b.Helper()
	data, root := p.Data, p.Root
	decoded := c.New()
	if err := c.Unmarshal(decoded, data); err != nil {
		b.Fatalf("decode: %v", err)
	}
	checkEncodes := func(v any) {
		b.Helper()
		out, err := c.Marshal(v)
		if err != nil {
			b.Fatalf("re-encode: %v", err)
		}
		if !bytes.Equal(out, data) {
			b.Fatal("decoded value does not encode back to the input")
		}
	}
	checkRoot := func(got []byte) {
		b.Helper()
		if !bytes.Equal(got, root[:]) {
			b.Fatalf("root mismatch: got %x want %x", got, root)
		}
	}

	run(b, c, "Unmarshal", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		var v any
		startLoop(b)
		iterate(b, func() {
			v = c.New()
			if err := c.Unmarshal(v, data); err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		checkEncodes(v)
	})
	if c.UnmarshalReader != nil {
		run(b, c, "UnmarshalReader", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			var v any
			startLoop(b)
			iterate(b, func() {
				v = c.New()
				if err := c.UnmarshalReader(v, bytes.NewReader(data), len(data)); err != nil {
					b.Fatal(err)
				}
			})
			b.StopTimer()
			checkEncodes(v)
		})
		run(b, c, "UnmarshalReaderUnknown", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			var v any
			startLoop(b)
			iterate(b, func() {
				v = c.New()
				if err := c.UnmarshalReader(v, bytes.NewReader(data), -1); err != nil {
					b.Fatal(err)
				}
			})
			b.StopTimer()
			checkEncodes(v)
		})
	}
	run(b, c, "SizeSSZ", func(b *testing.B) {
		var size int
		startLoop(b)
		iterate(b, func() {
			var err error
			size, err = c.Size(decoded)
			if err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		if size != len(data) {
			b.Fatalf("size %d want %d", size, len(data))
		}
	})
	run(b, c, "Marshal", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		var out []byte
		startLoop(b)
		iterate(b, func() {
			var err error
			out, err = c.Marshal(decoded)
			if err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		if !bytes.Equal(out, data) {
			b.Fatal("marshal output differs from input")
		}
	})
	run(b, c, "MarshalTo", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		buf := make([]byte, 0, len(data))
		var out []byte
		startLoop(b)
		iterate(b, func() {
			var err error
			out, err = c.MarshalTo(decoded, buf[:0])
			if err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		if !bytes.Equal(out, data) {
			b.Fatal("marshalTo output differs from input")
		}
	})
	if c.MarshalWriter != nil {
		run(b, c, "MarshalWriter", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			w := &countWriter{data: make([]byte, 0, len(data))}
			startLoop(b)
			iterate(b, func() {
				w.data = w.data[:0]
				if err := c.MarshalWriter(decoded, w); err != nil {
					b.Fatal(err)
				}
			})
			b.StopTimer()
			if !bytes.Equal(w.data, data) {
				b.Fatal("writer output differs from input")
			}
		})
	}
	run(b, c, "HashTreeRoot", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		var got [32]byte
		startLoop(b)
		iterate(b, func() {
			var err error
			got, err = c.Root(decoded)
			if err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		checkRoot(got[:])
	})
	run(b, c, "GetTree", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		var got []byte
		startLoop(b)
		iterate(b, func() {
			var err error
			got, err = c.Tree(decoded)
			if err != nil {
				b.Fatal(err)
			}
		})
		b.StopTimer()
		checkRoot(got)
	})
}

// RunSet benchmarks every operation over a set of payloads per iteration.
func RunSet(b *testing.B, c Codec, p Payload) {
	b.Helper()
	set, roots := p.Set, p.Roots
	var total int64
	decoded := make([]any, len(set))
	for i, data := range set {
		total += int64(len(data))
		decoded[i] = c.New()
		if err := c.Unmarshal(decoded[i], data); err != nil {
			b.Fatalf("decode %d: %v", i, err)
		}
	}
	checkEncodes := func(vs []any) {
		b.Helper()
		for i, v := range vs {
			out, err := c.Marshal(v)
			if err != nil {
				b.Fatalf("re-encode %d: %v", i, err)
			}
			if !bytes.Equal(out, set[i]) {
				b.Fatalf("decoded value %d does not encode back to the input", i)
			}
		}
	}
	checkRoots := func(got [][]byte) {
		b.Helper()
		for i := range got {
			if !bytes.Equal(got[i], roots[i][:]) {
				b.Fatalf("root %d mismatch: got %x want %x", i, got[i], roots[i])
			}
		}
	}

	run(b, c, "Unmarshal", func(b *testing.B) {
		b.SetBytes(total)
		vs := make([]any, len(set))
		startLoop(b)
		iterate(b, func() {
			for j, data := range set {
				vs[j] = c.New()
				if err := c.Unmarshal(vs[j], data); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.StopTimer()
		checkEncodes(vs)
	})
	if c.UnmarshalReader != nil {
		run(b, c, "UnmarshalReader", func(b *testing.B) {
			b.SetBytes(total)
			vs := make([]any, len(set))
			startLoop(b)
			iterate(b, func() {
				for j, data := range set {
					vs[j] = c.New()
					if err := c.UnmarshalReader(vs[j], bytes.NewReader(data), len(data)); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.StopTimer()
			checkEncodes(vs)
		})
		run(b, c, "UnmarshalReaderUnknown", func(b *testing.B) {
			b.SetBytes(total)
			vs := make([]any, len(set))
			startLoop(b)
			iterate(b, func() {
				for j, data := range set {
					vs[j] = c.New()
					if err := c.UnmarshalReader(vs[j], bytes.NewReader(data), -1); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.StopTimer()
			checkEncodes(vs)
		})
	}
	run(b, c, "SizeSSZ", func(b *testing.B) {
		var size int
		startLoop(b)
		iterate(b, func() {
			size = 0
			for _, v := range decoded {
				n, err := c.Size(v)
				if err != nil {
					b.Fatal(err)
				}
				size += n
			}
		})
		b.StopTimer()
		if int64(size) != total {
			b.Fatalf("size %d want %d", size, total)
		}
	})
	run(b, c, "Marshal", func(b *testing.B) {
		b.SetBytes(total)
		outs := make([][]byte, len(set))
		startLoop(b)
		iterate(b, func() {
			for j, v := range decoded {
				var err error
				outs[j], err = c.Marshal(v)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.StopTimer()
		for j := range outs {
			if !bytes.Equal(outs[j], set[j]) {
				b.Fatalf("item %d marshal output differs", j)
			}
		}
	})
	run(b, c, "MarshalTo", func(b *testing.B) {
		b.SetBytes(total)
		buf := make([]byte, 0, 4<<20)
		var out []byte
		checked := false
		startLoop(b)
		iterate(b, func() {
			for j, v := range decoded {
				var err error
				out, err = c.MarshalTo(v, buf[:0])
				if err != nil {
					b.Fatal(err)
				}
				if !checked && !bytes.Equal(out, set[j]) {
					b.Fatalf("item %d marshalTo output differs", j)
				}
			}
			checked = true
		})
	})
	if c.MarshalWriter != nil {
		run(b, c, "MarshalWriter", func(b *testing.B) {
			b.SetBytes(total)
			w := &countWriter{data: make([]byte, 0, 4<<20)}
			checked := false
			startLoop(b)
			iterate(b, func() {
				for j, v := range decoded {
					w.data = w.data[:0]
					if err := c.MarshalWriter(v, w); err != nil {
						b.Fatal(err)
					}
					if !checked && !bytes.Equal(w.data, set[j]) {
						b.Fatalf("item %d writer output differs", j)
					}
				}
				checked = true
			})
		})
	}
	run(b, c, "HashTreeRoot", func(b *testing.B) {
		b.SetBytes(total)
		got := make([][]byte, len(set))
		startLoop(b)
		iterate(b, func() {
			for j, v := range decoded {
				r, err := c.Root(v)
				if err != nil {
					b.Fatal(err)
				}
				got[j] = r[:]
			}
		})
		b.StopTimer()
		checkRoots(got)
	})
	run(b, c, "GetTree", func(b *testing.B) {
		b.SetBytes(total)
		got := make([][]byte, len(set))
		startLoop(b)
		iterate(b, func() {
			for j, v := range decoded {
				var err error
				got[j], err = c.Tree(v)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.StopTimer()
		checkRoots(got)
	})
}
