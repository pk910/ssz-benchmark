package bench;

import java.io.FileOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintStream;
import java.lang.management.ManagementFactory;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Supplier;
import java.util.regex.Pattern;

/**
 * The adapter side of benchwrap's protocol (harness/benchwrap): the pattern and the iteration
 * counts the wrapper passes, the messages on file descriptor 3, and the measuring loop with the
 * kit's policy adapted to a JIT runtime: a warm-up until the compiler has settled (untimed,
 * reported as warmup=k), then the fixed iterations with the collector off the timed path.
 */
final class Protocol {
  /** A batch of iterations may leave this much behind before it is collected, as the kit's. */
  static final long DROP_BATCH = 256L << 20;

  /** The warm-up of a leaf: this many calls and this long; a state object only this long. */
  static final int WARM_ITERS = 50;

  static final long WARM_NS = 3_000_000_000L;

  static final class Leaf {
    final String engine;
    final String object;
    final String op;

    Leaf(final String engine, final String object, final String op) {
      this.engine = engine;
      this.object = object;
      this.op = op;
    }

    String name() {
      return engine + "/" + object + "/" + op;
    }
  }

  private final PrintStream w;
  private final List<Pattern> pattern = new ArrayList<>();
  private final Map<String, Integer> iters = new HashMap<>();
  private final int fixed;
  private final long targetNs;
  private final com.sun.management.ThreadMXBean threads =
      (com.sun.management.ThreadMXBean) ManagementFactory.getThreadMXBean();

  /** Takes the pipe on file descriptor 3 (stderr when run by hand) and the wrapper's environment. */
  Protocol() {
    OutputStream out;
    try {
      out = new FileOutputStream("/proc/self/fd/3");
    } catch (IOException e) {
      out = System.err;
    }
    w = new PrintStream(out, true);
    final String pat = env("BENCH_PATTERN", ".");
    for (String p : pat.split("/")) {
      pattern.add(Pattern.compile(p));
    }
    for (String part : env("BENCH_ITERS", "").split(",")) {
      final int eq = part.indexOf('=');
      if (eq > 0) {
        try {
          iters.put(part.substring(0, eq).trim(), Integer.parseInt(part.substring(eq + 1).trim()));
        } catch (NumberFormatException e) {
          // not a count: ignored, as the kit ignores it
        }
      }
    }
    final String time = env("BENCH_TIME", "1x");
    if (time.endsWith("x")) {
      int n;
      try {
        n = Integer.parseInt(time.substring(0, time.length() - 1));
      } catch (NumberFormatException e) {
        n = 1;
      }
      fixed = n;
      targetNs = -1;
    } else {
      fixed = 0;
      targetNs = parseDuration(time);
    }
  }

  private static String env(final String name, final String fallback) {
    final String v = System.getenv(name);
    return v == null || v.isEmpty() ? fallback : v;
  }

  /**
   * Go testing's matching: each element of the pattern against the element of
   * "BenchmarkReal/<Engine>/<Object>/<Op>" at its position; missing trailing elements match.
   */
  boolean matches(final Leaf l) {
    final String[] name = {"BenchmarkReal", l.engine, l.object, l.op};
    for (int i = 0; i < pattern.size() && i < name.length; i++) {
      if (!pattern.get(i).matcher(name[i]).find()) {
        return false;
      }
    }
    return true;
  }

  private void send(final String msg) {
    w.println(msg);
    w.flush();
  }

  /** Names the measuring thread: the tid of the current thread from /proc/thread-self. */
  void thread() {
    try {
      final String link = Files.readSymbolicLink(Path.of("/proc/thread-self")).toString();
      send("thread " + link.substring(link.lastIndexOf('/') + 1));
    } catch (IOException e) {
      send("thread " + ProcessHandle.current().pid());
    }
  }

  void fail(final Leaf l, final String msg) {
    send("fail " + l.name() + " " + msg.replace('\n', ' '));
  }

  void skip(final Leaf l) {
    send("skip " + l.name());
  }

  /**
   * Measures one leaf: the warm-up calls until the JIT has settled (at least WARM_ITERS calls and
   * WARM_NS; a state object, whose single call is long, WARM_NS only), then the fixed iterations,
   * or a count grown as Go's testing does until a batch reaches the benchtime. The results of a
   * batch are dropped and collected between batches with the counters paused.
   */
  void run(final Leaf l, final boolean state, final Supplier<Object> f) {
    final int count = iters.getOrDefault(l.op, 0);
    int n = count > 0 ? count : Math.max(fixed, 1);
    int warm = 0;
    final long t0 = System.nanoTime();
    do {
      sink = f.get();
      warm++;
    } while ((!state && warm < WARM_ITERS) || System.nanoTime() - t0 < WARM_NS);
    sink = null;
    System.gc();
    while (true) {
      final long[] r = timed(l, n, f);
      final long elapsed = r[0];
      if (targetNs > 0 && count == 0 && elapsed < targetNs) {
        final long next = (long) (n * ((double) targetNs / Math.max(elapsed, 1)) * 1.2);
        n = roundUp((int) Math.min(Math.max(next, n + 1L), Integer.MAX_VALUE));
        continue;
      }
      send("end " + l.name() + " iters=" + n + " ns=" + elapsed + " bytes=" + r[1] + " warmup=" + warm);
      return;
    }
  }

  /** Keeps the warm-up results alive until the loop is over, so that the calls are not elided. */
  private static volatile Object sink;

  /** Runs n iterations: {elapsed ns, bytes allocated by the measuring thread}. */
  private long[] timed(final Leaf l, final int n, final Supplier<Object> f) {
    final ArrayList<Object> kept = new ArrayList<>(Math.min(n, 1 << 16));
    long every = 1;
    long elapsed = 0;
    long sumBytes = 0;
    // The counter is read after the messages, whose strings are not the operation's.
    send("begin " + l.name());
    long bytes0 = threads.getCurrentThreadAllocatedBytes();
    long start = System.nanoTime();
    for (int i = 0; i < n; i++) {
      final Object r = f.get();
      if (r != null) {
        kept.add(r);
      }
      final boolean last = i + 1 == n;
      final boolean batch = i == 0 || (i + 1) % every == 0;
      if (!batch && !last) {
        continue;
      }
      elapsed += System.nanoTime() - start;
      sumBytes += threads.getCurrentThreadAllocatedBytes() - bytes0;
      if (i == 0) {
        if (sumBytes == 0) {
          every = Long.MAX_VALUE;
        } else if (sumBytes < DROP_BATCH) {
          every = DROP_BATCH / sumBytes;
        } else {
          every = 1;
        }
      }
      if (!last) {
        send("pause");
        kept.clear();
        System.gc();
        send("resume");
        bytes0 = threads.getCurrentThreadAllocatedBytes();
        start = System.nanoTime();
      }
    }
    kept.clear();
    return new long[] {elapsed, sumBytes};
  }

  private static int roundUp(final int n) {
    int base = 1;
    while (base * 10L <= n) {
      base *= 10;
    }
    if (n <= base) {
      return base;
    }
    if (n <= 2 * base) {
      return 2 * base;
    }
    if (n <= 5 * base) {
      return 5 * base;
    }
    return 10 * base;
  }

  private static long parseDuration(final String s) {
    final String t = s.trim();
    int i = 0;
    while (i < t.length() && !Character.isLetter(t.charAt(i))) {
      i++;
    }
    final double v = Double.parseDouble(t.substring(0, i));
    switch (t.substring(i)) {
      case "ns":
        return (long) v;
      case "us":
      case "µs":
        return (long) (v * 1e3);
      case "ms":
        return (long) (v * 1e6);
      case "s":
        return (long) (v * 1e9);
      case "m":
        return (long) (v * 60e9);
      default:
        return -1;
    }
  }
}
