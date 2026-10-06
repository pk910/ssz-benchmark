# The adapter side of benchwrap's protocol (harness/benchwrap): the
# pattern and the iteration counts the wrapper passes, the messages on
# file descriptor 3, and the measuring loop with the kit's policy.

import std/[monotimes, os, posix, strutils, times]

const
  # A batch of iterations may keep this much of results before they are
  # released between batches, as the kit's collections are spaced.
  dropBatch = 256 shl 20

type
  Leaf* = object
    engine*, obj*, op*: string

  # glibc's POSIX regular expressions (REG_EXTENDED) match the patterns
  # the runner builds (quoted names, anchors, alternation), and need no
  # library beyond libc, which std/re (libpcre) would.
  Regex {.importc: "regex_t", header: "<regex.h>", bycopy.} = object

  Session* = object
    fd: cint
    pattern: seq[Regex]
    iters: seq[(string, int)]
    fixed: int
    target: int64 # nanoseconds, 0 when the count is fixed

var REG_EXTENDED {.importc, header: "<regex.h>".}: cint
var REG_NOSUB {.importc, header: "<regex.h>".}: cint
proc regcomp(preg: ptr Regex, pattern: cstring, cflags: cint): cint {.importc, header: "<regex.h>".}
proc regexec(preg: ptr Regex, s: cstring, nmatch: csize_t, pmatch: pointer, eflags: cint): cint {.importc, header: "<regex.h>".}

func newLeaf*(engine, obj, op: string): Leaf =
  Leaf(engine: engine, obj: obj, op: op)

func name*(l: Leaf): string =
  l.engine & "/" & l.obj & "/" & l.op

proc parseDuration(s: string): int64 =
  ## A Go duration ("2s", "500ms", "1m") in nanoseconds; 0 when not one.
  let t = s.strip()
  var i = 0
  while i < t.len and (t[i].isDigit or t[i] == '.'):
    inc i
  if i == 0:
    return 0
  let v = parseFloat(t[0 ..< i])
  let unit = t[i .. ^1]
  let scale =
    case unit
    of "ns": 1.0
    of "us", "µs": 1e3
    of "ms": 1e6
    of "s": 1e9
    of "m": 60e9
    else: return 0
  int64(v * scale)

proc open*(): Session =
  ## Takes the pipe on file descriptor 3 (stderr when run by hand) and the
  ## environment of the wrapper.
  result.fd = if fcntl(3, F_GETFD) != -1: 3.cint else: 2.cint
  for p in getEnv("BENCH_PATTERN", ".").split('/'):
    var re: Regex
    if regcomp(addr re, p.cstring, REG_EXTENDED or REG_NOSUB) != 0:
      stderr.writeLine("bad pattern element: " & p)
      quit(2)
    result.pattern.add re
  for part in getEnv("BENCH_ITERS").split(','):
    let kv = part.split('=', 1)
    if kv.len == 2:
      try:
        result.iters.add((kv[0].strip(), parseInt(kv[1].strip())))
      except ValueError:
        discard
  let benchTime = getEnv("BENCH_TIME", "1x")
  if benchTime.endsWith("x"):
    result.fixed = max(1, parseInt(benchTime[0 .. ^2]))
  else:
    result.target = parseDuration(benchTime)
    if result.target == 0:
      result.fixed = 1

proc matches*(s: var Session, l: Leaf): bool =
  ## Go testing's matching: each element of the pattern against the
  ## element of "BenchmarkReal/<Engine>/<Object>/<Op>" at its position.
  let name = ["BenchmarkReal", l.engine, l.obj, l.op]
  for i in 0 ..< min(s.pattern.len, name.len):
    if regexec(addr s.pattern[i], name[i].cstring, 0, nil, 0) != 0:
      return false
  true

proc send(s: Session, msg: string) =
  let line = msg & "\n"
  var off = 0
  while off < line.len:
    let n = write(s.fd, unsafeAddr line[off], line.len - off)
    if n <= 0:
      return
    off += n

proc thread*(s: Session) =
  ## Names the measuring thread: /proc/thread-self links to
  ## "<pid>/task/<tid>".
  var buf: array[64, char]
  let n = readlink("/proc/thread-self", cast[cstring](addr buf[0]), buf.len - 1)
  var tid = getpid().int
  if n > 0:
    var link = newString(n)
    copyMem(addr link[0], addr buf[0], n)
    let i = link.rfind('/')
    try:
      tid = parseInt(link[i + 1 .. ^1])
    except ValueError:
      discard
  s.send("thread " & $tid)

proc close*(s: Session) =
  ## Closes the pipe once every leaf is sent, then keeps the measuring
  ## thread alive a moment: the wrapper reads the thread's figures from
  ## /proc when it handles the last line.
  if s.fd == 3:
    discard close(s.fd)
  sleep(100)

proc fail*(s: Session, l: Leaf, msg: string) =
  s.send("fail " & l.name & " " & msg.replace('\n', ' '))

proc skip*(s: Session, l: Leaf) =
  s.send("skip " & l.name)

proc allocCount(): int =
  ## Allocations so far, from Nim's allocator statistics when the build
  ## counts them (-d:nimAllocStats); the allocator counts no bytes.
  when defined(nimAllocStats):
    # The fields of AllocStats are private; it is two ints.
    cast[(int, int)](getAllocStats())[0]
  else:
    0

proc roundUp(n: int): int =
  ## Go testing's rounding of the next count: 1, 2, 5, 10, 20, 50, ...
  var base = 1
  while base * 10 <= n:
    base *= 10
  if n <= base: base
  elif n <= 2 * base: 2 * base
  elif n <= 5 * base: 5 * base
  else: 10 * base

type
  Run*[R] = object
    ## A measured leaf before its result line is sent: the last
    ## iteration's results, for the caller's check, and the line.
    last*: seq[R]
    endLine: string

proc timed[R](s: Session, l: Leaf, n, per: int, f: proc(kept: var seq[R]), last: var seq[R]): int64 =
  ## n iterations; every iteration appends its per results to kept. With
  ## ORC a result is freed when it is dropped, so the results of a batch
  ## are kept and released between batches inside pause/resume, outside
  ## the timed and counted windows (the final batch's after the last
  ## pause, the last iteration's moved to last). The first iteration tells
  ## how much one retains (the allocator's occupied memory), which sizes
  ## the batch under dropBatch so that it never grows in a timed window;
  ## results that hold no heap memory are overwritten in place, as the
  ## kit's loop overwrites its result variable.
  var kept = newSeqOfCap[R](per)
  var every = 1
  var overwrite = false
  var elapsed: int64
  s.send("begin " & l.name)
  let occupied0 = getOccupiedMem()
  var start = getMonoTime()
  for i in 0 ..< n:
    if overwrite:
      kept.setLen(0)
    f(kept)
    let isLast = i + 1 == n
    let batch = i == 0 or (i + 1) mod every == 0
    if not batch and not isLast:
      continue
    elapsed += inNanoseconds(getMonoTime() - start)
    s.send("pause")
    if isLast:
      last.setLen(0)
      for j in kept.len - per ..< kept.len:
        last.add move(kept[j])
      kept.setLen(0)
      break
    if i == 0:
      let retained = getOccupiedMem() - occupied0
      if retained <= 0:
        every = high(int)
        overwrite = true
        kept.setLen(0)
      else:
        every = if retained < dropBatch: dropBatch div retained else: 1
        # Releases the first results and holds a batch without growing.
        kept = newSeqOfCap[R](per * min(every, n))
    else:
      kept.setLen(0)
    s.send("resume")
    start = getMonoTime()
  elapsed

proc run*[R](s: Session, l: Leaf, per: int, f: proc(kept: var seq[R])): Run[R] =
  ## Measures one leaf, f appending the per results of one iteration: the
  ## untimed warm-up calls (two when the operation allocates), then the
  ## fixed iterations, or a count grown as Go's testing does until a batch
  ## reaches the benchtime. Returns the last iteration's results with the
  ## result line, which finish sends once the caller has checked them, as
  ## the kit checks a leaf's output after its loop. Bytes and allocations
  ## are left out: Nim's allocator counts allocations but not bytes, and
  ## the wrapper reports them as a pair.
  var fixed = 0
  for (op, n) in s.iters:
    if op == l.op:
      fixed = n
  var n = if fixed > 0: fixed else: max(1, s.fixed)
  var warm = newSeqOfCap[R](per)
  let a0 = allocCount()
  f(warm)
  var warmups = 1
  if allocCount() != a0:
    warm.setLen(0)
    f(warm)
    warmups = 2
  warm = @[]
  while true:
    let elapsed = timed(s, l, n, per, f, result.last)
    if s.target > 0 and fixed == 0 and elapsed < s.target:
      # Go's testing: the count the benchtime predicts, a fifth more, at
      # most a hundredfold and at least one more, at most 1e9, rounded up
      # to 1, 2, 5 times a power of ten.
      var next = int(float(n) * float(s.target) / max(float(elapsed), 1.0) * 1.2)
      next = min(next, 100 * n)
      next = max(next, n + 1)
      n = roundUp(min(next, 1_000_000_000))
      continue
    result.endLine = "end " & l.name & " iters=" & $n & " ns=" & $elapsed & " warmup=" & $warmups
    return

proc finish*[R](s: Session, l: Leaf, r: sink Run[R], ok: bool, msg: string) =
  ## Ends a measured leaf: the result line when the check of its output
  ## passed, else the failure. The last results are dropped here, after
  ## the line.
  if ok:
    s.send(r.endLine)
  else:
    s.fail(l, msg)
