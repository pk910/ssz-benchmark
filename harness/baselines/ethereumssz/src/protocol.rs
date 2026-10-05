//! The adapter side of benchwrap's protocol (harness/benchwrap): the
//! pattern and the iteration counts the wrapper passes, the messages on
//! file descriptor 3, and the measuring loop with the kit's policy.

use std::fs::File;
use std::io::Write;
use std::os::fd::FromRawFd;
use std::time::{Duration, Instant};

/// A batch of iterations may leave this much behind before its results
/// are dropped between batches, as the kit's collections are spaced.
const DROP_BATCH: u64 = 256 << 20;

pub struct Leaf {
    pub engine: String,
    pub object: String,
    pub op: String,
}

impl Leaf {
    pub fn new(engine: &str, object: &str, op: &str) -> Leaf {
        Leaf { engine: engine.to_string(), object: object.to_string(), op: op.to_string() }
    }
    pub fn name(&self) -> String {
        format!("{}/{}/{}", self.engine, self.object, self.op)
    }
}

pub struct Session {
    w: Box<dyn Write>,
    pattern: Vec<regex::Regex>,
    iters: Vec<(String, usize)>,
    fixed: usize,
    target: Option<Duration>,
}

impl Session {
    /// Takes the pipe on file descriptor 3 (stderr when run by hand) and
    /// the environment of the wrapper.
    pub fn open() -> Session {
        let w: Box<dyn Write> = if unsafe { libc::fcntl(3, libc::F_GETFD) } != -1 {
            Box::new(unsafe { File::from_raw_fd(3) })
        } else {
            Box::new(std::io::stderr())
        };
        let pattern = std::env::var("BENCH_PATTERN").unwrap_or_else(|_| ".".to_string());
        let pattern = pattern
            .split('/')
            .map(|p| regex::Regex::new(p).unwrap_or_else(|e| panic!("pattern {p}: {e}")))
            .collect();
        let iters = std::env::var("BENCH_ITERS")
            .unwrap_or_default()
            .split(',')
            .filter_map(|part| {
                let (name, val) = part.trim().split_once('=')?;
                Some((name.trim().to_string(), val.trim().parse().ok()?))
            })
            .collect();
        let bench_time = std::env::var("BENCH_TIME").unwrap_or_else(|_| "1x".to_string());
        let (fixed, target) = if let Some(n) = bench_time.strip_suffix('x') {
            (n.parse().unwrap_or(1), None)
        } else {
            (0, parse_duration(&bench_time))
        };
        Session { w, pattern, iters, fixed, target }
    }

    /// Go testing's matching: each element of the pattern against the
    /// element of "BenchmarkReal/<Engine>/<Object>/<Op>" at its position.
    pub fn matches(&self, l: &Leaf) -> bool {
        let name = ["BenchmarkReal".to_string(), l.engine.clone(), l.object.clone(), l.op.clone()];
        self.pattern.iter().zip(name.iter()).all(|(re, elem)| re.is_match(elem))
    }

    fn send(&mut self, msg: &str) {
        let _ = writeln!(self.w, "{msg}");
        let _ = self.w.flush();
    }

    pub fn thread(&mut self) {
        let tid = unsafe { libc::syscall(libc::SYS_gettid) };
        self.send(&format!("thread {tid}"));
    }

    pub fn fail(&mut self, l: &Leaf, msg: &str) {
        self.send(&format!("fail {} {}", l.name(), msg.replace('\n', " ")));
    }

    pub fn skip(&mut self, l: &Leaf) {
        self.send(&format!("skip {}", l.name()));
    }

    /// Measures one leaf: the untimed warm-up calls (two when the
    /// operation allocates), then the fixed iterations, or a count grown as
    /// Go's testing does until a batch reaches the benchtime. The results
    /// of a batch are dropped between batches with the counters paused.
    pub fn run<R, F: FnMut() -> R>(&mut self, l: &Leaf, mut f: F) {
        let fixed = self.iters.iter().find(|(op, _)| *op == l.op).map(|(_, n)| *n).unwrap_or(0);
        let mut n = if fixed > 0 { fixed } else { self.fixed.max(1) };
        let (b0, _) = crate::allocs();
        drop(f());
        let (b1, _) = crate::allocs();
        let warm = if b1 != b0 {
            drop(f());
            2
        } else {
            1
        };
        loop {
            let (iters, elapsed, bytes, count) = self.timed(l, n, &mut f);
            match self.target {
                Some(t) if fixed == 0 && elapsed < t => {
                    let next = (n as f64 * t.as_secs_f64() / elapsed.as_secs_f64().max(1e-9) * 1.2) as usize;
                    n = round_up(next.max(n + 1));
                }
                _ => {
                    self.send(&format!(
                        "end {} iters={} ns={} bytes={} allocs={} warmup={}",
                        l.name(),
                        iters,
                        elapsed.as_nanos(),
                        bytes,
                        count,
                        warm
                    ));
                    return;
                }
            }
        }
    }

    fn timed<R, F: FnMut() -> R>(&mut self, l: &Leaf, n: usize, f: &mut F) -> (usize, Duration, u64, u64) {
        let mut kept: Vec<R> = Vec::with_capacity(n.min(1 << 16));
        let mut every = 1usize;
        let mut elapsed = Duration::ZERO;
        let (mut bytes0, mut count0) = crate::allocs();
        let (mut sum_bytes, mut sum_count) = (0u64, 0u64);
        self.send(&format!("begin {}", l.name()));
        let mut start = Instant::now();
        for i in 0..n {
            kept.push(f());
            let last = i + 1 == n;
            let batch = i == 0 || (i + 1) % every == 0;
            if !batch && !last {
                continue;
            }
            elapsed += start.elapsed();
            let (b1, c1) = crate::allocs();
            sum_bytes += b1 - bytes0;
            sum_count += c1 - count0;
            if i == 0 {
                every = if sum_bytes == 0 {
                    usize::MAX
                } else if sum_bytes < DROP_BATCH {
                    (DROP_BATCH / sum_bytes) as usize
                } else {
                    1
                };
            }
            if !last {
                self.send("pause");
                kept.clear();
                let (b, c) = crate::allocs();
                bytes0 = b;
                count0 = c;
                self.send("resume");
                start = Instant::now();
            }
        }
        drop(kept);
        (n, elapsed, sum_bytes, sum_count)
    }
}

fn round_up(n: usize) -> usize {
    let mut base = 1;
    while base * 10 <= n {
        base *= 10;
    }
    if n <= base {
        base
    } else if n <= 2 * base {
        2 * base
    } else if n <= 5 * base {
        5 * base
    } else {
        10 * base
    }
}

fn parse_duration(s: &str) -> Option<Duration> {
    let (num, unit) = s.trim().split_at(s.trim().find(|c: char| c.is_alphabetic())?);
    let v: f64 = num.parse().ok()?;
    Some(match unit {
        "ns" => Duration::from_nanos(v as u64),
        "us" | "µs" => Duration::from_micros(v as u64),
        "ms" => Duration::from_millis(v as u64),
        "s" => Duration::from_secs_f64(v),
        "m" => Duration::from_secs_f64(v * 60.0),
        _ => return None,
    })
}
