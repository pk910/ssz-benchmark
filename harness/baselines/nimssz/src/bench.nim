# The adapter of nim-ssz-serialization (Nimbus) for benchwrap: the harness
# objects decoded into the generated Nim types (gen_fulu.nim,
# gen_gloas.nim, both presets in one module), every operation measured
# with the kit's policy, the figures sent over the protocol
# (harness/benchwrap).
#
# Operations: Unmarshal (SSZ.decode), SizeSSZ (sszSize), Marshal
# (SSZ.encode) and HashTreeRoot (hash_tree_root). Objects of a fork: the
# state, the block, the block set, and (Gloas) the envelope, plus the
# minimal-preset state and block.
#
# Memory: ORC frees deterministically, so what an operation frees inside
# itself is part of it (as in Rust); the results of a batch are released
# between batches with the counters paused.

import std/[algorithm, os, strutils]
import ssz_serialization, ssz_serialization/merkleization
import ./protocol
import ./gen_fulu, ./gen_gloas

const
  engine = "Nimbus"
  ops = ["Unmarshal", "SizeSSZ", "Marshal", "HashTreeRoot"]

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

proc verify[T](items: seq[Item], decoded: var seq[T], rootOf: proc(v: T): Digest {.nimcall.}): string =
  ## Decodes every item and checks it as the kit does: re-encoded bytes
  ## equal the input, the root equals the stored one. Returns the
  ## message of the first difference, "" when all agree.
  for it in items:
    var v: T
    try:
      v = SSZ.decode(it.data, T)
    except CatchableError as e:
      return "decode: " & e.msg
    if SSZ.encode(v) != it.data:
      return "decoded value does not encode back to the input"
    let got = rootOf(v)
    if got != it.root:
      return "root mismatch: got " & $got & " want " & $it.root
    decoded.add v
  ""

proc bench[T](s: var Session, ls: seq[Leaf], items: seq[Item], rootOf: proc(v: T): Digest {.nimcall.}) =
  ## Measures the operations of an object; one iteration of a set runs
  ## the operation on every item.
  var decoded: seq[T]
  let msg = verify[T](items, decoded, rootOf)
  if msg != "":
    for l in ls:
      s.fail(l, msg)
    return
  for l in ls:
    case l.op
    of "Unmarshal":
      s.run(l, proc(): seq[T] =
        result = newSeqOfCap[T](items.len)
        for it in items:
          try:
            result.add SSZ.decode(it.data, T)
          except CatchableError as e:
            raiseAssert "decode: " & e.msg)
    of "SizeSSZ":
      s.run(l, proc(): int =
        for v in decoded:
          result += sszSize(v))
    of "Marshal":
      s.run(l, proc(): seq[seq[byte]] =
        result = newSeqOfCap[seq[byte]](decoded.len)
        for v in decoded:
          result.add SSZ.encode(v))
    of "HashTreeRoot":
      s.run(l, proc(): seq[Digest] =
        result = newSeqOfCap[Digest](decoded.len)
        for v in decoded:
          result.add rootOf(v))
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

main()
