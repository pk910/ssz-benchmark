// The adapter side of benchwrap's protocol (harness/benchwrap): the
// pattern and the iteration counts the wrapper passes, the messages on
// file descriptor 3, and the measuring loop with the kit's policy on a
// JIT runtime.
import fs from "node:fs";
import v8 from "node:v8";

// A batch of iterations may leave this much behind before its results
// are collected between batches, as the kit's collections are spaced.
const DROP_BATCH = 256 * 1024 * 1024;

// The warm-up of a leaf: V8 compiles and optimizes the code paths on the
// first iterations, so a leaf runs untimed until it has both this many
// iterations and this much time behind it; a state object, whose single
// iteration takes seconds, stops at the time alone.
const WARM_ITERS = 50;
const WARM_NS = 3_000_000_000n;
// Discovery (BENCH_DISCOVER) wants the leaves and their checks, not
// figures: the warm-up is one call then.
const DISCOVER = Boolean(process.env.BENCH_DISCOVER);

export class Leaf {
  constructor(engine, object, op) {
    this.engine = engine;
    this.object = object;
    this.op = op;
  }

  name() {
    return `${this.engine}/${this.object}/${this.op}`;
  }

  // The state objects, whose warm-up is capped at its time.
  isState() {
    return this.object.endsWith("State");
  }
}

// used is the memory the program holds: the V8 heap and what it owns
// outside it (the ArrayBuffers of serialized output above all). V8 counts
// no allocations; the growth over an iteration estimates its garbage.
function used() {
  const s = v8.getHeapStatistics();
  return s.used_heap_size + s.external_memory;
}

export class Session {
  // Takes the pipe on file descriptor 3 (stderr when run by hand) and
  // the environment of the wrapper.
  constructor() {
    this.fd = 2;
    try {
      const st = fs.fstatSync(3);
      if (st.isFIFO() || st.isFile() || st.isSocket() || st.isCharacterDevice()) {
        this.fd = 3;
      }
    } catch {
      // No pipe: the messages go to stderr.
    }
    this.pattern = (process.env.BENCH_PATTERN || ".").split("/").map((p) => new RegExp(p));
    this.iters = new Map();
    for (const part of (process.env.BENCH_ITERS || "").split(",")) {
      const [name, val] = part.split("=");
      const n = Number.parseInt(val, 10);
      if (name && Number.isFinite(n)) {
        this.iters.set(name.trim(), n);
      }
    }
    const benchTime = process.env.BENCH_TIME || "1x";
    this.fixed = 0;
    this.target = null;
    if (benchTime.endsWith("x")) {
      this.fixed = Number.parseInt(benchTime, 10) || 1;
    } else {
      this.target = parseDuration(benchTime);
    }
  }

  // Go testing's matching: each element of the pattern against the
  // element of "BenchmarkReal/<Engine>/<Object>/<Op>" at its position.
  matches(l) {
    const name = ["BenchmarkReal", l.engine, l.object, l.op];
    return this.pattern.every((re, i) => i >= name.length || re.test(name[i]));
  }

  send(msg) {
    try {
      fs.writeSync(this.fd, msg + "\n");
    } catch (e) {
      // A descriptor 3 that is not ours (run by hand under a shell that
      // holds one): the messages go to stderr from here on.
      if (this.fd === 2) throw e;
      this.fd = 2;
      fs.writeSync(this.fd, msg + "\n");
    }
  }

  // The main thread runs every iteration: /proc/thread-self names it.
  thread() {
    const tid = fs.readlinkSync("/proc/thread-self").split("/")[2];
    this.send(`thread ${tid}`);
  }

  fail(l, msg) {
    this.send(`fail ${l.name()} ${String(msg).replace(/\n/g, " ")}`);
  }

  skip(l) {
    this.send(`skip ${l.name()}`);
  }

  // Measures one leaf: the untimed warm-up, then the fixed iterations, or
  // a count grown as Go's testing does until a batch reaches the
  // benchtime. The results of a batch are dropped and collected between
  // batches with the counters paused. An operation that needs a fresh
  // object every time (a tree whose roots are cached once hashed) gives
  // `prepare`: it runs before each iteration with the counters paused, the
  // previous object dropped and collected first, and f takes its result.
  // `check`, when given, examines the last iteration's result after the
  // loop, outside the timed window, as the kit checks its results, and
  // returns a message when it is wrong: the leaf then fails.
  run(l, f, prepare, check) {
    const fixed = this.iters.get(l.op) || 0;
    let n = fixed > 0 ? fixed : Math.max(this.fixed, 1);
    const {warm, grown} = this.warmup(l, f, prepare);
    for (;;) {
      const {elapsed, last} = prepare ? this.timedPrepared(l, n, f, prepare) : this.timed(l, n, f, grown);
      if (this.target !== null && fixed === 0 && elapsed < this.target) {
        const next = Math.floor((n * Number(this.target)) / Math.max(Number(elapsed), 1) * 1.2);
        n = roundUp(Math.min(Math.max(next, n + 1), 100 * n, 1e9));
        continue;
      }
      const wrong = check ? check(last) : null;
      if (wrong) {
        this.fail(l, wrong);
      } else {
        this.send(`end ${l.name()} iters=${n} ns=${elapsed} warmup=${warm}`);
      }
      return;
    }
  }

  // The warm-up iterations, collected when their garbage reaches a
  // batch; returns their count and the growth of the first iteration,
  // the estimate of what one iteration leaves behind. With `prepare`,
  // every iteration works on a fresh object, as the timed ones do.
  warmup(l, f, prepare) {
    const start = process.hrtime.bigint();
    global.gc();
    let arg = prepare ? prepare() : undefined;
    const h0 = used();
    f(arg);
    const grown = Math.max(used() - h0, 0);
    let k = 1;
    let garbage = grown;
    for (;;) {
      if (garbage >= DROP_BATCH || prepare) {
        arg = undefined;
        global.gc();
        garbage = 0;
      }
      const elapsed = process.hrtime.bigint() - start;
      if (DISCOVER || (elapsed >= WARM_NS && (l.isState() || k >= WARM_ITERS))) {
        break;
      }
      if (prepare) {
        arg = prepare();
      }
      f(arg);
      k++;
      garbage += grown;
    }
    arg = undefined;
    global.gc();
    return {warm: k, grown};
  }

  // The timed loop: n iterations in batches of as many as keep under a
  // batch of garbage by the estimate (every one when nothing grows), the
  // results of a batch kept until it ends and dropped with the clock
  // stopped. Returns the elapsed nanoseconds.
  timed(l, n, f, grown) {
    const every = grown > 0 ? Math.max(1, Math.floor(DROP_BATCH / grown)) : n;
    // The results of a batch, in an array sized once so that keeping them
    // allocates nothing inside the window.
    const kept = new Array(Math.min(n, every));
    let k = 0;
    let elapsed = 0n;
    let result;
    this.send(`begin ${l.name()}`);
    let start = process.hrtime.bigint();
    for (let i = 0; i < n; i++) {
      result = f();
      kept[k++] = result;
      const last = i + 1 === n;
      if ((i + 1) % every !== 0 && !last) {
        continue;
      }
      elapsed += process.hrtime.bigint() - start;
      this.send("pause");
      if (!last) {
        kept.fill(undefined);
        k = 0;
        global.gc();
        this.send("resume");
        start = process.hrtime.bigint();
      }
    }
    kept.fill(undefined);
    return {elapsed, last: result};
  }

  // The timed loop of an operation on a fresh object per iteration: the
  // previous object and the results dropped and collected, the object
  // prepared, all with the counters paused; the clock runs around f
  // alone. Returns the elapsed nanoseconds.
  timedPrepared(l, n, f, prepare) {
    let elapsed = 0n;
    let arg;
    let result;
    this.send(`begin ${l.name()}`);
    for (let i = 0; i < n; i++) {
      this.send("pause");
      arg = undefined;
      if (i + 1 < n) {
        result = undefined;
      }
      global.gc();
      arg = prepare();
      this.send("resume");
      const start = process.hrtime.bigint();
      result = f(arg);
      elapsed += process.hrtime.bigint() - start;
    }
    this.send("pause");
    arg = undefined;
    return {elapsed, last: result};
  }
}

// roundUp rounds an iteration count to 1, 2, 5, 10, ... as Go's testing
// does.
function roundUp(n) {
  let base = 1;
  while (base * 10 <= n) {
    base *= 10;
  }
  if (n <= base) return base;
  if (n <= 2 * base) return 2 * base;
  if (n <= 5 * base) return 5 * base;
  return 10 * base;
}

// parseDuration reads a Go duration ("2s", "500ms", "1m") in nanoseconds.
function parseDuration(s) {
  const m = /^\s*([\d.]+)\s*(ns|us|µs|ms|s|m)\s*$/.exec(s);
  if (!m) {
    return null;
  }
  const v = Number.parseFloat(m[1]);
  const unit = {ns: 1, us: 1e3, "µs": 1e3, ms: 1e6, s: 1e9, m: 60e9}[m[2]];
  return BigInt(Math.round(v * unit));
}
