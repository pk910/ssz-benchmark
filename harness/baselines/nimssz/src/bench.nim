# The adapter of nim-ssz-serialization (Nimbus) for benchwrap: the harness
# objects decoded into the generated Nim types (gen_fulu.nim,
# gen_gloas.nim, both presets in one module), every operation measured
# with the kit's policy, the figures sent over the protocol
# (harness/benchwrap).
#
# Operations: Unmarshal (a fresh object, readSszBytes), SizeSSZ (sszSize),
# Marshal (SSZ.encode, a new buffer), MarshalTo (an SszWriter over a
# buffer kept across iterations) and HashTreeRoot (hash_tree_root).
# Objects of a fork: the state, the block, the block set, and (Gloas) the
# envelope, plus the minimal-preset state and block.
#
# Memory: ORC frees deterministically, so what an operation frees inside
# itself is part of it (as in Rust); the results of a batch are released
# between batches with the counters paused.

import std/[algorithm, os, strutils]
import faststreams/outputs, stew/ptrops
import ssz_serialization, ssz_serialization/merkleization
import ./protocol
import ./gen_fulu, ./gen_gloas

const
  engine = "Nimbus"
  ops = ["Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"]

type
  # A payload: the bytes of one or more objects with their roots.
  Item = object
    data: seq[byte]
    root: Digest

proc readBytes(path: string): seq[byte] =
  let s = readFile(path)
  result = newSeq[byte](s.len)
  if s.len > 0:
    copyMem(addr result[0], unsafeAddr s[0], s.len)

proc readRoot(path: string): Digest =
  var hex = readFile(path).strip()
  if hex.startsWith("0x"):
    hex = hex[2 .. ^1]
  for i in 0 ..< 32:
    result.data[i] = byte(parseHexInt(hex[2 * i .. 2 * i + 1]))

proc loadOne(dir, name: string): seq[Item] =
  @[Item(data: readBytes(dir / name & ".ssz"), root: readRoot(dir / name & ".root"))]

proc loadSet(dir, name: string): seq[Item] =
  var files: seq[string]
  for f in walkFiles(dir / name / "*.ssz"):
    files.add f
  files.sort()
  for f in files:
    result.add Item(data: readBytes(f), root: readRoot(f.changeFileExt("root")))

proc leaves(s: var Session, obj: string): seq[Leaf] =
  ## The leaves of an object the pattern selects.
  for op in ops:
    let l = newLeaf(engine, obj, op)
    if s.matches(l):
      result.add l

proc verify[T](items: seq[Item], decoded: var seq[ref T], rootOf: proc(v: T): Digest {.nimcall.}): string =
  ## Decodes every item and checks it as the kit does: re-encoded bytes
  ## equal the input, the size equals the input length, the root equals
  ## the stored one. Returns the message of the first difference, "" when
  ## all agree.
  for it in items:
    var v = new T
    try:
      readSszBytes(it.data, v[])
    except CatchableError as e:
      return "decode: " & e.msg
    if SSZ.encode(v[]) != it.data:
      return "decoded value does not encode back to the input"
    let size = sszSize(v[])
    if size != it.data.len:
      return "size " & $size & " want " & $it.data.len
    let got = rootOf(v[])
    if got != it.root:
      return "root mismatch: got " & $got & " want " & $it.root
    decoded.add v
  ""

proc bench[T](s: var Session, ls: seq[Leaf], items: seq[Item], rootOf: proc(v: T): Digest {.nimcall.}) =
  ## Measures the operations of an object; one iteration of a set runs
  ## the operation on every item.
  var decoded: seq[ref T]
  let msg = verify[T](items, decoded, rootOf)
  if msg != "":
    for l in ls:
      s.fail(l, msg)
    return
  var total = 0
  for it in items:
    total += it.data.len
  proc encodes(vs: seq[ref T]): bool =
    for i, v in vs:
      if SSZ.encode(v[]) != items[i].data:
        return false
    true
  for l in ls:
    case l.op
    of "Unmarshal":
      # Bytes into a fresh object: a zeroed heap object, decoded in
      # place, as the kit's New and Unmarshal.
      let r = s.run(l, items.len, proc(kept: var seq[ref T]) =
        for it in items:
          var v = new T
          try:
            readSszBytes(it.data, v[])
          except CatchableError as e:
            raiseAssert "decode: " & e.msg
          kept.add v)
      s.finish(l, r, encodes(r.last), "decoded value does not encode back to the input")
    of "SizeSSZ":
      let r = s.run(l, 1, proc(kept: var seq[int]) =
        var size = 0
        for v in decoded:
          size += sszSize(v[])
        kept.add size)
      s.finish(l, r, r.last[0] == total, "size " & $r.last[0] & " want " & $total)
    of "Marshal":
      let r = s.run(l, items.len, proc(kept: var seq[seq[byte]]) =
        for v in decoded:
          kept.add SSZ.encode(v[]))
      var ok = true
      for i, enc in r.last:
        if enc != items[i].data:
          ok = false
      s.finish(l, r, ok, "marshal output differs from input")
    of "MarshalTo":
      # One buffer kept across iterations, an unbuffered output stream
      # over it rewound to its start for every item (the buffer cleared),
      # the library's writer encoding into it. Every item's encoding is
      # checked once before the loop; the buffer holds the last item after
      # it.
      var buf = newSeq[byte](total)
      let base = addr buf[0]
      let stream = unsafeMemoryOutput(base, buf.len).s
      var w = SszWriter.init(stream)
      proc rewind() =
        stream.span = PageSpan(startAddr: base, endAddr: offset(base, buf.len))
        stream.spanEndPos = buf.len
      proc holds(it: Item): bool =
        stream.pos == it.data.len and equalMem(base, unsafeAddr it.data[0], stream.pos)
      var ok = true
      for i, v in decoded:
        rewind()
        w.writeValue(v[])
        if not holds(items[i]):
          ok = false
      if not ok:
        s.fail(l, "marshalTo output differs from input")
        continue
      let r = s.run(l, 1, proc(kept: var seq[int]) =
        var size = 0
        for v in decoded:
          rewind()
          w.writeValue(v[])
          size += stream.pos
        kept.add size)
      s.finish(l, r, r.last[0] == total and holds(items[^1]), "marshalTo output differs from input")
    of "HashTreeRoot":
      let r = s.run(l, items.len, proc(kept: var seq[Digest]) =
        for v in decoded:
          kept.add rootOf(v[]))
      var ok = true
      for i, got in r.last:
        if got != items[i].root:
          ok = false
      s.finish(l, r, ok, "root mismatch")
    else:
      s.skip(l)

proc one(s: var Session, obj, dir, file: string, T: typedesc, rootOf: proc(v: T): Digest {.nimcall.}) =
  let ls = leaves(s, obj)
  if ls.len == 0:
    return
  bench[T](s, ls, loadOne(dir, file), rootOf)

proc set(s: var Session, obj, dir, name: string, T: typedesc, rootOf: proc(v: T): Digest {.nimcall.}) =
  let ls = leaves(s, obj)
  if ls.len == 0:
    return
  bench[T](s, ls, loadSet(dir, name), rootOf)

template own(T: typedesc): untyped =
  proc(v: T): Digest {.nimcall.} = hash_tree_root(v)

template message(T: typedesc): untyped =
  proc(v: T): Digest {.nimcall.} = hash_tree_root(v.Message)

proc main() =
  var fork = "fulu"
  let args = commandLineParams()
  for i in 0 ..< args.len:
    if args[i] == "--fork" and i + 1 < args.len:
      fork = args[i + 1]
  let data = getEnv("REAL_DATA", "/srv/benchd/res/real")
  var s = protocol.open()
  s.thread()
  let dir = data / fork
  let min = dir / "minimal"
  case fork
  of "fulu":
    one(s, "FuluState", dir, "state", gen_fulu.FuluBeaconState, own(gen_fulu.FuluBeaconState))
    one(s, "FuluBlock", dir, "block", gen_fulu.ElectraSignedBeaconBlock, message(gen_fulu.ElectraSignedBeaconBlock))
    set(s, "FuluBlocks", dir, "blocks", gen_fulu.ElectraSignedBeaconBlock, message(gen_fulu.ElectraSignedBeaconBlock))
    one(s, "FuluMinState", min, "state", gen_fulu.FuluBeaconStateMinimal, own(gen_fulu.FuluBeaconStateMinimal))
    one(s, "FuluMinBlock", min, "block", gen_fulu.ElectraSignedBeaconBlockMinimal, message(gen_fulu.ElectraSignedBeaconBlockMinimal))
  of "gloas":
    one(s, "GloasState", dir, "state", gen_gloas.GloasBeaconState, own(gen_gloas.GloasBeaconState))
    one(s, "GloasBlock", dir, "block", gen_gloas.GloasSignedBeaconBlock, message(gen_gloas.GloasSignedBeaconBlock))
    set(s, "GloasBlocks", dir, "blocks", gen_gloas.GloasSignedBeaconBlock, message(gen_gloas.GloasSignedBeaconBlock))
    one(s, "GloasEnvelope", dir, "envelope", gen_gloas.GloasSignedExecutionPayloadEnvelope, message(gen_gloas.GloasSignedExecutionPayloadEnvelope))
    one(s, "GloasMinState", min, "state", gen_gloas.GloasBeaconStateMinimal, own(gen_gloas.GloasBeaconStateMinimal))
    one(s, "GloasMinBlock", min, "block", gen_gloas.GloasSignedBeaconBlockMinimal, message(gen_gloas.GloasSignedBeaconBlockMinimal))
  else:
    stderr.writeLine("unknown fork " & fork)
    quit(2)
  s.close()

main()
