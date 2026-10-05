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

proc timed[R](s: Session, l: Leaf, n: int, f: proc(): R): int64 =
  ## n iterations with the results kept and released between batches
  ## inside pause/resume: with ORC the release is the destruction of the
  ## batch, which so stays outside the timed window. The first iteration
  ## tells how much one retains (the allocator's occupied memory), which
  ## sizes the batch under dropBatch.
  var kept = newSeqOfCap[R](min(n, 1 shl 16))
  var every = 1
  var elapsed: int64
  s.send("begin " & l.name)
  let occupied0 = getOccupiedMem()
  var start = getMonoTime()
  for i in 0 ..< n:
    kept.add f()
    let last = i + 1 == n
    let batch = i == 0 or (i + 1) mod every == 0
    if not batch and not last:
      continue
    elapsed += inNanoseconds(getMonoTime() - start)
    if i == 0:
      let retained = getOccupiedMem() - occupied0
      every =
        if retained <= 0: high(int)
        elif retained < dropBatch: dropBatch div retained
        else: 1
    if not last:
      s.send("pause")
      kept.setLen(0)
      s.send("resume")
      start = getMonoTime()
  kept.setLen(0)
  elapsed

proc run*[R](s: Session, l: Leaf, f: proc(): R) =
  ## Measures one leaf: the untimed warm-up calls (two when the operation
  ## allocates), then the fixed iterations, or a count grown as Go's
  ## testing does until a batch reaches the benchtime. Bytes and
  ## allocations are left out: Nim's allocator counts allocations but not
  ## bytes, and the wrapper reports them as a pair.
  var fixed = 0
  for (op, n) in s.iters:
    if op == l.op:
      fixed = n
  var n = if fixed > 0: fixed else: max(1, s.fixed)
  let a0 = allocCount()
  discard f()
  var warm = 1
  if allocCount() != a0:
    discard f()
    warm = 2
  while true:
    let elapsed = timed(s, l, n, f)
    if s.target > 0 and fixed == 0 and elapsed < s.target:
      let next = int(float(n) * float(s.target) / max(float(elapsed), 1.0) * 1.2)
      n = roundUp(max(next, n + 1))
      continue
    s.send("end " & l.name & " iters=" & $n & " ns=" & $elapsed & " warmup=" & $warm)
    return
