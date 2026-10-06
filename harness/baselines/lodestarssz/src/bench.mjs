// The adapter of @chainsafe/ssz (Lodestar's SSZ) for benchwrap: the
// harness objects decoded with the generated type objects (gen_fulu.mjs,
// gen_gloas.mjs, one export per preset), every operation measured with
// the kit's policy, the figures sent over the protocol
// (harness/benchwrap).
//
// Two engines, one per representation of the library:
//
//   LodestarValue  the structural value: deserialize, serialize,
//                  value_serializeToBytes into a kept buffer (MarshalTo),
//                  value_serializedSize, hashTreeRoot(value)
//   LodestarTree   the deferred-update tree view: deserializeToViewDU,
//                  view.serialize, view.serializeToBytes (MarshalTo),
//                  tree_serializedSize; the view caches every root it
//                  computes, so HashTreeRoot hashes a fresh view per
//                  iteration, decoded with the counters paused: the timed
//                  work is the hash of an uncached tree alone
//
// The hasher is the one Lodestar runs with: the native hashtree binding
// (@chainsafe/hashtree, a dependency of persistent-merkle-tree), which
// Lodestar's command line sets at startup in place of the package's
// default, a pure-JavaScript SHA-256. V8 counts neither bytes nor
// allocations exactly: the end message carries none, and the garbage of
// a batch is estimated from the heap growth.
import fs from "node:fs";
import path from "node:path";
import {setHasher} from "../work/ssz/packages/persistent-merkle-tree/lib/index.js";
import {hasher as hashtreeHasher} from "../work/ssz/packages/persistent-merkle-tree/lib/hasher/hashtree.js";
import {Leaf, Session} from "./protocol.mjs";

setHasher(hashtreeHasher);

const OPS = ["Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"];

// The engines: the operations of each representation on a type, with
// `msg` set when the root wanted is the Message's (a signed object).
const ENGINES = {
  LodestarValue: {
    decode: (type, data) => type.deserialize(data),
    encode: (type, v) => type.serialize(v),
    encodeTo: (type, v, out) => type.value_serializeToBytes(out, 0, v),
    size: (type, v) => type.value_serializedSize(v),
    root: (type, v, msg) => (msg ? type.fields.Message.hashTreeRoot(v.Message) : type.hashTreeRoot(v)),
  },
  LodestarTree: {
    decode: (type, data) => type.deserializeToViewDU(data),
    encode: (_type, view) => view.serialize(),
    encodeTo: (_type, view, out) => view.serializeToBytes(out, 0),
    size: (type, view) => type.tree_serializedSize(view.node),
    root: (_type, view, msg) => (msg ? view.Message : view).hashTreeRoot(),
  },
};

// The objects of a fork: the harness name, the generated type, the
// payload file (a directory for a set), the preset, and whether the root
// is the Message's.
const OBJECTS = {
  fulu: [
    {name: "FuluState", type: "FuluBeaconState", file: "state", msg: false},
    {name: "FuluBlock", type: "ElectraSignedBeaconBlock", file: "block", msg: true},
    {name: "FuluBlocks", type: "ElectraSignedBeaconBlock", file: "blocks", set: true, msg: true},
    {name: "FuluMinState", type: "FuluBeaconState", file: "state", minimal: true, msg: false},
    {name: "FuluMinBlock", type: "ElectraSignedBeaconBlock", file: "block", minimal: true, msg: true},
  ],
  gloas: [
    {name: "GloasState", type: "GloasBeaconState", file: "state", msg: false},
    {name: "GloasBlock", type: "GloasSignedBeaconBlock", file: "block", msg: true},
    {name: "GloasBlocks", type: "GloasSignedBeaconBlock", file: "blocks", set: true, msg: true},
    {name: "GloasEnvelope", type: "GloasSignedExecutionPayloadEnvelope", file: "envelope", msg: true},
    {name: "GloasMinState", type: "GloasBeaconState", file: "state", minimal: true, msg: false},
    {name: "GloasMinBlock", type: "GloasSignedBeaconBlock", file: "block", minimal: true, msg: true},
  ],
};

async function main() {
  const args = process.argv.slice(2);
  const i = args.indexOf("--fork");
  const fork = i >= 0 && args[i + 1] ? args[i + 1] : "fulu";
  if (!OBJECTS[fork]) {
    console.error(`unknown fork ${fork}`);
    process.exit(2);
  }
  const types = await import(`./gen_${fork}.mjs`);
  const data = path.join(process.env.REAL_DATA || "/srv/benchd/res/real", fork);
  const s = new Session();
  s.thread();
  for (const obj of OBJECTS[fork]) {
    const leaves = {};
    let any = false;
    for (const engine of Object.keys(ENGINES)) {
      leaves[engine] = OPS.map((op) => new Leaf(engine, obj.name, op)).filter((l) => s.matches(l));
      any ||= leaves[engine].length > 0;
    }
    if (!any) {
      continue;
    }
    const dir = obj.minimal ? path.join(data, "minimal") : data;
    const payload = obj.set ? loadSet(dir, obj.file) : loadOne(dir, obj.file);
    const type = (obj.minimal ? types.minimal : types.mainnet)[obj.type];
    for (const engine of Object.keys(ENGINES)) {
      if (leaves[engine].length > 0) {
        bench(s, ENGINES[engine], leaves[engine], type, obj.msg, payload);
        global.gc();
      }
    }
  }
}

function readRoot(file) {
  const hex = fs.readFileSync(file, "utf8").trim().replace(/^0x/, "");
  return Uint8Array.from(Buffer.from(hex, "hex"));
}

// readBytes gives the file as a plain Uint8Array of its own buffer (a
// Buffer may be a slice of a pool).
function readBytes(file) {
  return Uint8Array.from(fs.readFileSync(file));
}

// A payload: the bytes of one or more objects with their roots.
function loadOne(dir, name) {
  return {items: [{data: readBytes(path.join(dir, `${name}.ssz`)), root: readRoot(path.join(dir, `${name}.root`))}]};
}

function loadSet(dir, name) {
  const files = fs
    .readdirSync(path.join(dir, name))
    .filter((f) => f.endsWith(".ssz"))
    .sort();
  return {
    items: files.map((f) => ({
      data: readBytes(path.join(dir, name, f)),
      root: readRoot(path.join(dir, name, f.replace(/\.ssz$/, ".root"))),
    })),
  };
}

function equal(a, b) {
  return a.length === b.length && Buffer.from(a.buffer, a.byteOffset, a.length).equals(Buffer.from(b.buffer, b.byteOffset, b.length));
}

function hex(a) {
  return Buffer.from(a.buffer, a.byteOffset, a.length).toString("hex");
}

// Decodes every item of the payload and checks it as the kit does:
// re-encoded bytes equal the input, the root equals the stored one.
function verify(eng, type, msg, payload) {
  const values = [];
  for (const {data, root} of payload.items) {
    let v;
    try {
      v = eng.decode(type, data);
    } catch (e) {
      return {err: `decode: ${e.message}`};
    }
    let enc;
    let got;
    try {
      enc = eng.encode(type, v);
      got = eng.root(type, v, msg);
    } catch (e) {
      return {err: `encode: ${e.message}`};
    }
    if (!equal(enc, data)) {
      return {err: "decoded value does not encode back to the input"};
    }
    if (!equal(got, root)) {
      return {err: `root mismatch: got ${hex(got)} want ${hex(root)}`};
    }
    values.push(v);
  }
  return {values};
}

// Measures the operations of an object under one engine; one iteration
// of a set runs the operation on every item.
function bench(s, eng, leaves, type, msg, payload) {
  // Only the verdict of the verification is kept: its values (a hashed
  // tree view of a state is over 4 GB of heap) must be gone before an
  // operation that builds fresh objects (Unmarshal, the tree's
  // HashTreeRoot) runs. The operations on decoded values decode them
  // again, once, when they come.
  const {err} = verify(eng, type, msg, payload);
  if (err) {
    for (const l of leaves) {
      s.fail(l, err);
    }
    return;
  }
  let values = null;
  const decoded = () => (values ??= payload.items.map(({data}) => eng.decode(type, data)));
  const dropDecoded = () => {
    values = null;
    global.gc();
  };
  dropDecoded();
  const items = payload.items;
  const total = items.reduce((sum, {data}) => sum + data.length, 0);
  // The checks of the last result after each loop, as the kit makes them.
  const encodesBack = (vs) => (vs.every((v, i) => equal(eng.encode(type, v), items[i].data)) ? null : "decoded value does not encode back to the input");
  const equalsInput = (outs) => (outs.every((out, i) => equal(out, items[i].data)) ? null : "marshal output differs from input");
  const rootsEqual = (roots) => (roots.every((r, i) => equal(r, items[i].root)) ? null : "root mismatch after the loop");
  for (const l of leaves) {
    switch (l.op) {
      case "Unmarshal":
        s.run(l, () => items.map(({data}) => eng.decode(type, data)), undefined, encodesBack);
        break;
      case "SizeSSZ": {
        const vs = decoded();
        s.run(l, () => vs.reduce((sum, v) => sum + eng.size(type, v), 0), undefined, (size) => (size === total ? null : `size ${size} want ${total}`));
        break;
      }
      case "Marshal": {
        const vs = decoded();
        s.run(l, () => vs.map((v) => eng.encode(type, v)), undefined, equalsInput);
        break;
      }
      case "MarshalTo": {
        // One buffer kept across the iterations, large enough for the
        // largest item; each item is written from its start.
        const uint8Array = new Uint8Array(Math.max(...items.map(({data}) => data.length)));
        const out = {uint8Array, dataView: new DataView(uint8Array.buffer)};
        const vs = decoded();
        s.run(
          l,
          () => {
            let n = 0;
            for (const v of vs) {
              n = eng.encodeTo(type, v, out);
            }
            return n;
          },
          undefined,
          (n) => {
            const want = items[items.length - 1].data;
            return n === want.length && equal(uint8Array.subarray(0, n), want) ? null : "marshalTo output differs from input";
          }
        );
        break;
      }
      case "HashTreeRoot":
        if (eng === ENGINES.LodestarTree) {
          // A view keeps every root it computed: every iteration hashes a
          // fresh view (the one of the Message for a signed object),
          // decoded with the counters paused so that only the walk of the
          // uncached tree is timed.
          dropDecoded();
          s.run(
            l,
            (views) => views.map((v) => v.hashTreeRoot()),
            () =>
              items.map(({data}) => {
                const v = eng.decode(type, data);
                return msg ? v.Message : v;
              }),
            rootsEqual
          );
        } else {
          const vs = decoded();
          s.run(l, () => vs.map((v) => eng.root(type, v, msg)), undefined, rootsEqual);
        }
        break;
      default:
        s.skip(l);
    }
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
