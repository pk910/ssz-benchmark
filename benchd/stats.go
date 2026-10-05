package main

import (
	"math"
	"sort"
	"strings"
	"time"
)

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func variance(xs []float64, m float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += (x - m) * (x - m)
	}
	return s / float64(len(xs)-1)
}

func median(xs []float64) float64 {
	return percentile(xs, 0.5)
}

// percentile returns the p-th percentile (0..1) of xs.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	idx := p * float64(len(s)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return s[lo]
	}
	return s[lo] + (s[hi]-s[lo])*(idx-float64(lo))
}

// middleHalf returns the bounds of the middle half of the values by
// rank: the lowest and the highest quarter (one value each at four) are
// left out, without interpolating towards them, so a single value far off
// does not stretch the range. Fewer than four values keep their full range.
func middleHalf(xs []float64) (lo, hi float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	k := len(s) / 4
	return s[k], s[len(s)-1-k]
}

// welch compares two samples: the difference of means with its 95% interval
// and the two-sided p-value of Welch's t-test.
func welch(a, b []float64) (diff, lo, hi, p float64) {
	ma, mb := mean(a), mean(b)
	diff = mb - ma
	if len(a) < 2 || len(b) < 2 {
		return diff, diff, diff, 1
	}
	va, vb := variance(a, ma), variance(b, mb)
	se2 := va/float64(len(a)) + vb/float64(len(b))
	if se2 == 0 {
		if diff == 0 {
			return diff, diff, diff, 1
		}
		return diff, diff, diff, 0
	}
	se := math.Sqrt(se2)
	na, nb := float64(len(a)), float64(len(b))
	df := se2 * se2 / (va*va/(na*na*(na-1)) + vb*vb/(nb*nb*(nb-1)))
	if math.IsNaN(df) || df < 1 {
		df = 1
	}
	t := diff / se
	p = 2 * (1 - tCDF(math.Abs(t), df))
	q := tQuantile(0.975, df)
	return diff, diff - q*se, diff + q*se, p
}

// tCDF is the cumulative distribution of Student's t with df degrees of
// freedom, through the regularized incomplete beta function.
func tCDF(t, df float64) float64 {
	x := df / (df + t*t)
	ib := incBeta(df/2, 0.5, x)
	if t >= 0 {
		return 1 - 0.5*ib
	}
	return 0.5 * ib
}

// tQuantile inverts tCDF by bisection.
func tQuantile(pr, df float64) float64 {
	lo, hi := 0.0, 1000.0
	for range 200 {
		mid := (lo + hi) / 2
		if tCDF(mid, df) < pr {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// incBeta is the regularized incomplete beta function I_x(a, b) by Lentz's
// continued fraction.
func incBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	if x > (a+1)/(a+b+2) {
		return 1 - incBeta(b, a, 1-x)
	}
	lbeta := lgamma(a) + lgamma(b) - lgamma(a+b)
	front := math.Exp(math.Log(x)*a+math.Log(1-x)*b-lbeta) / a
	const eps = 1e-14
	f, c, d := 1.0, 1.0, 0.0
	for i := 0; i <= 400; i++ {
		m := float64(i / 2)
		var num float64
		switch {
		case i == 0:
			num = 1
		case i%2 == 0:
			num = (m * (b - m) * x) / ((a + 2*m - 1) * (a + 2*m))
		default:
			num = -((a + m) * (a + b + m) * x) / ((a + 2*m) * (a + 2*m + 1))
		}
		d = 1 + num*d
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		d = 1 / d
		c = 1 + num/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		cd := c * d
		f *= cd
		if math.Abs(1-cd) < eps {
			break
		}
	}
	return front * (f - 1)
}

func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// compareMetric folds the per-side samples of one quantity into a metric.
func compareMetric(base, head []float64) metric {
	m := metric{Base: mean(base), Head: mean(head), MedBase: median(base), MedHead: median(head)}
	for i, v := range head {
		if i == 0 || v < m.HMin {
			m.HMin = v
		}
		if i == 0 || v > m.HMax {
			m.HMax = v
		}
	}
	if len(base) == 0 {
		// Baseline leaf: head only.
		m.CVHead = cv(head, m.Head)
		m.P = 1
		return m
	}
	diff, lo, hi, p := welch(base, head)
	m.P = p
	m.CVBase = cv(base, m.Base)
	m.CVHead = cv(head, m.Head)
	if m.Base != 0 {
		m.Delta = diff / m.Base * 100
		m.Lo = lo / m.Base * 100
		m.Hi = hi / m.Base * 100
	}
	if m.MedBase != 0 {
		m.MedDelta = (m.MedHead - m.MedBase) / m.MedBase * 100
	}
	// The samples of both sides are in pass order, so pairs are the two
	// sides of one pass: their deltas show how far single runs diverged.
	m.HQ1, m.HQ3 = middleHalf(head)
	m.DMin, m.DMax = m.Delta, m.Delta
	m.PMed, m.PQ1, m.PQ3 = m.Delta, m.Delta, m.Delta
	if len(base) == len(head) {
		var ds []float64
		for i := range base {
			if base[i] == 0 {
				continue
			}
			d := (head[i] - base[i]) / base[i] * 100
			m.DMin = math.Min(m.DMin, d)
			m.DMax = math.Max(m.DMax, d)
			ds = append(ds, d)
		}
		if len(ds) > 0 {
			// The median over the passes: one layout under which one side
			// runs far off its usual cost does not move it.
			m.PMed = median(ds)
			m.PQ1, m.PQ3 = middleHalf(ds)
			// The spread is taken from the median absolute deviation (scaled
			// to a standard deviation), so that one pass far off does not
			// widen the band of the result either.
			dev := make([]float64, len(ds))
			for i, d := range ds {
				dev[i] = math.Abs(d - m.PMed)
			}
			m.PSpread = 1.4826 * median(dev)
			m.PN = float64(len(ds))
			for _, d := range ds {
				if (d > 0) == (m.PMed > 0) && d != 0 {
					m.PAgree++
				}
			}
		}
	}
	return m
}

// band is how far a result's median may lie from zero without being a
// change: the larger of the given floor and twice the standard error of
// the per-pass deltas (from their robust spread), which carries what the
// layouts did to this pair.
func (m metric) band(floor float64) float64 {
	b := floor
	if m.PN > 1 {
		b = math.Max(b, 2*m.PSpread/math.Sqrt(m.PN))
	}
	return b
}

// changed reports whether the result is a change: the median beyond the
// band, and at least three quarters of the passes pointing its way.
func (m metric) changed(floor float64) bool {
	if math.Abs(m.PMed) <= m.band(floor) {
		return false
	}
	return m.PN == 0 || m.PAgree >= math.Ceil(0.75*m.PN)
}

// instrMoved is the change of the instructions per op from which an
// operation counts as doing different work.
const instrMoved = 0.5

// workChanged reports whether the instructions per op of a result moved:
// the code does different work. When they did not, a change of its cycles
// comes from how the same work executes (layout, cache, branches). ok is
// false without counters.
func (r result) workChanged() (changed, ok bool) {
	if r.Instrs.Base <= 0 || r.Instrs.Head <= 0 {
		return false, false
	}
	d := r.Instrs.PMed
	if r.Instrs.PN == 0 {
		d = r.Instrs.Delta
	}
	return math.Abs(d) >= instrMoved, true
}

func cv(xs []float64, m float64) float64 {
	if m == 0 {
		return 0
	}
	return math.Sqrt(variance(xs, m)) / m * 100
}

// summarize folds the samples of a job into one result per leaf.
func summarize(samples []sample) []result {
	type group struct {
		l                                        leaf
		nsB, nsH, bB, bH, aB, aH, cB, cH, iB, iH []float64
		iters, steal                             int
		extraB, extraH                           map[string][]float64
	}
	groups := map[string]*group{}
	for _, sm := range samples {
		if sm.isDiag() {
			continue
		}
		k := sm.Engine + "/" + sm.Object + "/" + sm.Op
		g, ok := groups[k]
		if !ok {
			g = &group{l: leaf{Engine: sm.Engine, Object: sm.Object, Op: sm.Op}, iters: sm.Iters, extraB: map[string][]float64{}, extraH: map[string][]float64{}}
			groups[k] = g
		}
		if sm.Iters > 0 && (g.iters == 0 || sm.Iters < g.iters) {
			g.iters = sm.Iters
		}
		g.steal += sm.Steal
		for k, v := range sm.Extra {
			if sm.Side == "head" {
				g.extraH[k] = append(g.extraH[k], v)
			} else {
				g.extraB[k] = append(g.extraB[k], v)
			}
		}
		if sm.Side == "head" {
			g.nsH = append(g.nsH, sm.Ns)
			g.bH = append(g.bH, sm.Bytes)
			g.aH = append(g.aH, sm.Allocs)
			g.cH = append(g.cH, sm.Cycles)
			g.iH = append(g.iH, sm.Instrs)
		} else {
			g.nsB = append(g.nsB, sm.Ns)
			g.bB = append(g.bB, sm.Bytes)
			g.aB = append(g.aB, sm.Allocs)
			g.cB = append(g.cB, sm.Cycles)
			g.iB = append(g.iB, sm.Instrs)
		}
	}
	var results []result
	for _, g := range groups {
		if len(g.nsH) == 0 {
			continue
		}
		r := result{Engine: g.l.Engine, Object: g.l.Object, Op: g.l.Op, Baseline: len(g.nsB) == 0, Iters: g.iters, Steal: g.steal,
			N: len(g.nsH), Ns: compareMetric(g.nsB, g.nsH), Bytes: compareMetric(g.bB, g.bH), Allocs: compareMetric(g.aB, g.aH),
			Cycles: compareMetric(g.cB, g.cH), Instrs: compareMetric(g.iB, g.iH)}
		for k, hs := range g.extraH {
			if r.Extra == nil {
				r.Extra = map[string]extraStat{}
			}
			st := extraStat{Head: median(hs), N: len(hs)}
			if bs := g.extraB[k]; len(bs) > 0 {
				st.Base = median(bs)
			}
			r.Extra[k] = st
		}
		// All threads were counted when every run says so.
		if t := g.extraH["threads"]; len(t) == len(g.nsH) && len(t) > 0 && (len(g.nsB) == 0 || len(g.extraB["threads"]) == len(g.nsB)) {
			r.Threads = int(median(t))
		}
		if !r.Baseline {
			r.N = min(len(g.nsB), len(g.nsH))
		}
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return leafLess(results[i].Object, results[i].Op, results[i].Engine, results[j].Object, results[j].Op, results[j].Engine)
	})
	return results
}

// Fixed display order of objects, operations and engines.
var (
	objectOrder = []string{"FuluState", "FuluBlock", "FuluBlocks", "FuluMinState", "FuluMinBlock", "GloasState", "GloasBlock", "GloasBlocks", "GloasEnvelope", "GloasMinState", "GloasMinBlock"}
	opOrder     = []string{"Unmarshal", "UnmarshalReader", "UnmarshalReaderUnknown", "SizeSSZ", "Marshal", "MarshalTo", "MarshalWriter", "HashTreeRoot", "GetTree"}
	engineOrder = []string{"Codegen", "Reflection", "CodegenAsync", "ReflectionAsync", "FastSSZ", "FastSSZv1", "FastSSZv2", "PrysmSSZ", "KaralabeSSZ", "KaralabeSSZAsync",
		"EthereumSSZ", "Grandine", "Teku", "LodestarValue", "LodestarTree", "Nimbus", "SszPP"}
)

// An engine named *Async is its parent engine hashing with background
// workers. The harness measures it as an engine of its own; the pages show
// it as an operation of the parent, "HashTreeRoot (async)".
func isAsync(engine string) bool { return strings.HasSuffix(engine, "Async") }

// baseEngine is the engine an async variant belongs to.
func baseEngine(engine string) string { return strings.TrimSuffix(engine, "Async") }

// opLabel names an operation on the pages: the async variant of an
// engine is its operation with "(async)".
func opLabel(engine, op string) string {
	if isAsync(engine) {
		return op + " (async)"
	}
	return op
}

func rank(order []string, v string) int {
	for i, o := range order {
		if o == v {
			return i
		}
	}
	return len(order)
}

func leafLess(o1, p1, e1, o2, p2, e2 string) bool {
	if a, b := rank(objectOrder, o1), rank(objectOrder, o2); a != b {
		return a < b
	}
	if o1 != o2 {
		return o1 < o2
	}
	if a, b := rank(opOrder, p1), rank(opOrder, p2); a != b {
		return a < b
	}
	if p1 != p2 {
		return p1 < p2
	}
	if a, b := rank(engineOrder, e1), rank(engineOrder, e2); a != b {
		return a < b
	}
	return e1 < e2
}

// sortLeafList orders leaves engine-major so a pass walks one engine's
// objects and operations in sequence (state loads stay grouped).
func sortLeafList(xs []leaf) {
	sort.Slice(xs, func(i, j int) bool {
		if a, b := rank(engineOrder, xs[i].Engine), rank(engineOrder, xs[j].Engine); a != b {
			return a < b
		}
		if xs[i].Engine != xs[j].Engine {
			return xs[i].Engine < xs[j].Engine
		}
		return leafLess(xs[i].Object, xs[i].Op, "", xs[j].Object, xs[j].Op, "")
	})
}

// geomeanDelta is the geometric mean of head/base time ratios of the
// non-baseline results, in percent.
func geomeanDelta(results []result) float64 {
	return geomeanOf(results, func(r result) metric { return r.Ns })
}

// geomeanCycles is the same over cycles per op; zero when the results
// carry no cycle counts.
func geomeanCycles(results []result) float64 {
	return geomeanOf(results, func(r result) metric { return r.Cycles })
}

func geomeanOf(results []result, pick func(result) metric) float64 {
	s, n := 0.0, 0
	for _, r := range results {
		m := pick(r)
		if r.Baseline || m.Base <= 0 || m.Head <= 0 {
			continue
		}
		// The ratio of a leaf is its median over the passes.
		ratio := 1 + m.PMed/100
		if m.PN == 0 || ratio <= 0 {
			ratio = m.Head / m.Base
		}
		s += math.Log(ratio)
		n++
	}
	if n == 0 {
		return 0
	}
	return (math.Exp(s/float64(n)) - 1) * 100
}

// trendFit is a line through (time, value) points: the relative slope per
// 30 days, the projected value 30 days past the last point, and R².
type trendFit struct {
	SlopePct30d  float64
	Projected30d float64
	R2           float64
	N            int
}

func fitTrend(times []time.Time, values []float64) trendFit {
	n := len(values)
	if n < 3 {
		return trendFit{N: n}
	}
	t0 := times[0]
	xs := make([]float64, n)
	for i := range times {
		xs[i] = times[i].Sub(t0).Hours() / 24
	}
	mx, my := mean(xs), mean(values)
	sxx, sxy, syy := 0.0, 0.0, 0.0
	for i := range xs {
		sxx += (xs[i] - mx) * (xs[i] - mx)
		sxy += (xs[i] - mx) * (values[i] - my)
		syy += (values[i] - my) * (values[i] - my)
	}
	if sxx == 0 || my == 0 {
		return trendFit{N: n}
	}
	slope := sxy / sxx
	intercept := my - slope*mx
	r2 := 0.0
	if syy > 0 {
		r2 = sxy * sxy / (sxx * syy)
	}
	return trendFit{SlopePct30d: slope * 30 / my * 100, Projected30d: intercept + slope*(xs[n-1]+30), R2: r2, N: n}
}

func joinKey(parts ...string) string { return strings.Join(parts, "/") }
