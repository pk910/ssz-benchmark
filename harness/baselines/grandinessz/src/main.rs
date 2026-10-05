//! The adapter of Grandine's SSZ for benchwrap: the harness objects decoded
//! into the generated Rust types (gen_fulu.rs, gen_gloas.rs, one module
//! per preset), every operation measured with the kit's policy, the
//! figures sent over the protocol (harness/benchwrap).
//!
//! Operations: Unmarshal (`SszRead::from_ssz`), Marshal (`SszWrite::to_ssz`),
//! MarshalTo (`write_variable` into a buffer kept across iterations) and
//! HashTreeRoot (`SszHash::hash_tree_root`). SizeSSZ is skipped: the crate
//! knows a type's size only as the compile-time `SszSize::SIZE` (fixed, or
//! variable with a minimum) and has no call that sizes a value. Objects of
//! a fork: the state, the block, the block set, and (Gloas) the envelope,
//! plus the minimal-preset state and block.
//!
//! The types are the crate's plain contiguous ones (`ContiguousList`,
//! `ContiguousVector`, `ProgressiveList`, ...), not the hash-caching
//! persistent collections Grandine's own `BeaconState` uses. A contiguous
//! vector is inline (a GenericArray), so a state is a few megabytes by
//! value and the decoder builds it on the stack: the work runs on a thread
//! with a large stack, which is also the measuring thread.
//!
//! Memory: a counting global allocator gives the bytes and allocations per
//! operation. Freeing is not part of an operation: as the kit collects
//! between batches with the clock stopped, the results of a batch are
//! dropped between batches with the counters paused.
#![allow(clippy::all)]

mod gen_fulu;
mod gen_gloas;
mod protocol;

use protocol::{Leaf, Session};
use ssz::{Size, SszHash, SszRead, SszReadDefault, SszWrite, H256};
use std::alloc::{GlobalAlloc, Layout, System};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

const ENGINE: &str = "Grandine";
const OPS: [&str; 5] = ["Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"];

/// The stack of the benchmark thread: a decoded state with its inline
/// vectors passes through several frames by value.
const STACK: usize = 1 << 30;

/// A counting allocator: bytes and allocations since the start.
struct Counting;

static ALLOC_BYTES: AtomicU64 = AtomicU64::new(0);
static ALLOC_COUNT: AtomicU64 = AtomicU64::new(0);

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        ALLOC_BYTES.fetch_add(layout.size() as u64, Ordering::Relaxed);
        ALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        System.alloc(layout)
    }
    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        System.dealloc(ptr, layout)
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        ALLOC_BYTES.fetch_add(layout.size() as u64, Ordering::Relaxed);
        ALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        System.alloc_zeroed(layout)
    }
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        if new_size > layout.size() {
            ALLOC_BYTES.fetch_add((new_size - layout.size()) as u64, Ordering::Relaxed);
        }
        ALLOC_COUNT.fetch_add(1, Ordering::Relaxed);
        System.realloc(ptr, layout, new_size)
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

pub fn allocs() -> (u64, u64) {
    (ALLOC_BYTES.load(Ordering::Relaxed), ALLOC_COUNT.load(Ordering::Relaxed))
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let fork = args
        .iter()
        .position(|a| a == "--fork")
        .and_then(|i| args.get(i + 1))
        .cloned()
        .unwrap_or_else(|| "fulu".to_string());
    let worker = std::thread::Builder::new()
        .name("bench".to_string())
        .stack_size(STACK)
        .spawn(move || run(&fork))
        .expect("benchmark thread");
    worker.join().expect("benchmark thread");
}

/// The benchmark thread: opens the session, names itself as the measuring
/// thread and runs the objects of the fork.
fn run(fork: &str) {
    let data = PathBuf::from(std::env::var("REAL_DATA").unwrap_or_else(|_| "/srv/benchd/res/real".to_string()));
    let mut s = Session::open();
    s.thread();
    let dir = data.join(fork);
    let min = dir.join("minimal");
    match fork {
        "fulu" => {
            use gen_fulu::{mainnet as m, minimal as n};
            one::<m::FuluBeaconState>(&mut s, "FuluState", &dir, "state", |v| v.hash_tree_root());
            one::<m::ElectraSignedBeaconBlock>(&mut s, "FuluBlock", &dir, "block", |v| v.Message.hash_tree_root());
            set::<m::ElectraSignedBeaconBlock>(&mut s, "FuluBlocks", &dir, "blocks", |v| v.Message.hash_tree_root());
            one::<n::FuluBeaconState>(&mut s, "FuluMinState", &min, "state", |v| v.hash_tree_root());
            one::<n::ElectraSignedBeaconBlock>(&mut s, "FuluMinBlock", &min, "block", |v| v.Message.hash_tree_root());
        }
        "gloas" => {
            use gen_gloas::{mainnet as m, minimal as n};
            one::<m::GloasBeaconState>(&mut s, "GloasState", &dir, "state", |v| v.hash_tree_root());
            one::<m::GloasSignedBeaconBlock>(&mut s, "GloasBlock", &dir, "block", |v| v.Message.hash_tree_root());
            set::<m::GloasSignedBeaconBlock>(&mut s, "GloasBlocks", &dir, "blocks", |v| v.Message.hash_tree_root());
            one::<m::GloasSignedExecutionPayloadEnvelope>(&mut s, "GloasEnvelope", &dir, "envelope", |v| v.Message.hash_tree_root());
            one::<n::GloasBeaconState>(&mut s, "GloasMinState", &min, "state", |v| v.hash_tree_root());
            one::<n::GloasSignedBeaconBlock>(&mut s, "GloasMinBlock", &min, "block", |v| v.Message.hash_tree_root());
        }
        other => {
            eprintln!("unknown fork {other}");
            std::process::exit(2);
        }
    }
    s.close();
}

/// The leaves of an object the pattern selects.
fn leaves(s: &Session, object: &str) -> Vec<Leaf> {
    OPS.iter()
        .map(|op| Leaf::new(ENGINE, object, op))
        .filter(|l| s.matches(l))
        .collect()
}

fn read_root(path: &Path) -> H256 {
    let text = fs::read_to_string(path).unwrap_or_else(|e| panic!("{}: {e}", path.display()));
    let hex = text.trim().trim_start_matches("0x");
    let mut out = [0u8; 32];
    for i in 0..32 {
        out[i] = u8::from_str_radix(&hex[2 * i..2 * i + 2], 16).expect("root hex");
    }
    H256::from(out)
}

/// A payload: the bytes of one or more objects with their roots.
struct Payload {
    items: Vec<(Vec<u8>, H256)>,
}

fn load_one(dir: &Path, name: &str) -> Payload {
    let data = fs::read(dir.join(format!("{name}.ssz"))).unwrap_or_else(|e| panic!("{name}.ssz: {e}"));
    let root = read_root(&dir.join(format!("{name}.root")));
    Payload { items: vec![(data, root)] }
}

fn load_set(dir: &Path, name: &str) -> Payload {
    let mut files: Vec<PathBuf> = fs::read_dir(dir.join(name))
        .unwrap_or_else(|e| panic!("{name}: {e}"))
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| p.extension().map(|x| x == "ssz").unwrap_or(false))
        .collect();
    files.sort();
    let items = files
        .iter()
        .map(|f| (fs::read(f).expect("block"), read_root(&f.with_extension("root"))))
        .collect();
    Payload { items }
}

/// Encodes into a buffer the caller keeps: the fixed-size path writes into
/// a slice of the type's size, the variable-size path appends.
fn write_to<T: SszWrite>(v: &T, buf: &mut Vec<u8>) {
    buf.clear();
    match T::SIZE {
        Size::Fixed { size } => {
            buf.resize(size, 0);
            v.write_fixed(buf.as_mut_slice());
        }
        Size::Variable { .. } => v.write_variable(buf).expect("encode"),
    }
}

/// Decodes every item of the payload and checks it as the kit does:
/// re-encoded bytes (to a new buffer and into a kept one) equal the
/// input, the root equals the stored one.
fn verify<T: SszRead<()> + SszWrite + SszHash>(p: &Payload, root_of: &dyn Fn(&T) -> H256) -> Result<Vec<T>, String> {
    let mut out = Vec::with_capacity(p.items.len());
    for (data, root) in &p.items {
        let v = T::from_ssz_default(data).map_err(|e| format!("decode: {e:?}"))?;
        if v.to_ssz().map_err(|e| format!("encode: {e:?}"))? != *data {
            return Err("decoded value does not encode back to the input".to_string());
        }
        let mut buf = Vec::with_capacity(data.len());
        write_to(&v, &mut buf);
        if buf != *data {
            return Err("decoded value does not encode into a kept buffer back to the input".to_string());
        }
        let got = root_of(&v);
        if got != *root {
            return Err(format!("root mismatch: got {got:x} want {root:x}"));
        }
        out.push(v);
    }
    Ok(out)
}

fn one<T: SszRead<()> + SszWrite + SszHash>(s: &mut Session, object: &str, dir: &Path, file: &str, root_of: impl Fn(&T) -> H256) {
    let ls = leaves(s, object);
    if ls.is_empty() {
        return;
    }
    let p = load_one(dir, file);
    bench::<T>(s, &ls, &p, &root_of);
}

fn set<T: SszRead<()> + SszWrite + SszHash>(s: &mut Session, object: &str, dir: &Path, name: &str, root_of: impl Fn(&T) -> H256) {
    let ls = leaves(s, object);
    if ls.is_empty() {
        return;
    }
    let p = load_set(dir, name);
    bench::<T>(s, &ls, &p, &root_of);
}

/// Measures the operations of an object; one iteration of a set runs the
/// operation on every item. A decoded object lives in the vector the
/// iteration returns (the one allocation that holds it, as the kit's
/// `New` does); the encodings of a set likewise. A single object's
/// encoding and root are returned bare, the roots of a set go into a
/// vector allocated once, so that no holder allocation is counted where
/// the kit has none.
fn bench<T: SszRead<()> + SszWrite + SszHash>(s: &mut Session, ls: &[Leaf], p: &Payload, root_of: &dyn Fn(&T) -> H256) {
    let decoded = match verify::<T>(p, root_of) {
        Ok(v) => v,
        Err(msg) => {
            for l in ls {
                s.fail(l, &msg);
            }
            return;
        }
    };
    let total: usize = p.items.iter().map(|(d, _)| d.len()).sum();
    let single = decoded.len() == 1;
    let encodes = |vs: &[T]| vs.iter().zip(&p.items).all(|(v, (d, _))| v.to_ssz().map(|out| out == *d).unwrap_or(false));
    for l in ls {
        match l.op.as_str() {
            "Unmarshal" => {
                let r = s.run(l, || {
                    p.items
                        .iter()
                        .map(|(d, _)| T::from_ssz_default(d).expect("decode"))
                        .collect::<Vec<T>>()
                });
                let ok = encodes(&r.last);
                s.finish(l, r, ok, "decoded value does not encode back to the input");
            }
            "Marshal" if single => {
                let r = s.run(l, || decoded[0].to_ssz().expect("encode"));
                let ok = r.last == p.items[0].0;
                s.finish(l, r, ok, "marshal output differs from input");
            }
            "Marshal" => {
                let r = s.run(l, || decoded.iter().map(|v| v.to_ssz().expect("encode")).collect::<Vec<Vec<u8>>>());
                let ok = r.last.iter().zip(&p.items).all(|(out, (d, _))| out == d);
                s.finish(l, r, ok, "marshal output differs from input");
            }
            "MarshalTo" => {
                let mut buf: Vec<u8> = Vec::with_capacity(total);
                let r = s.run(l, || {
                    for v in &decoded {
                        write_to(v, &mut buf);
                    }
                    buf.len()
                });
                // The buffer holds the last item after the loop.
                let ok = buf == p.items[decoded.len() - 1].0;
                s.finish(l, r, ok, "marshalTo output differs from input");
            }
            "HashTreeRoot" if single => {
                let r = s.run(l, || root_of(&decoded[0]));
                let ok = r.last == p.items[0].1;
                s.finish(l, r, ok, "root mismatch");
            }
            "HashTreeRoot" => {
                let mut roots: Vec<H256> = Vec::with_capacity(decoded.len());
                let r = s.run(l, || {
                    roots.clear();
                    roots.extend(decoded.iter().map(|v| root_of(v)));
                });
                let ok = roots.iter().zip(&p.items).all(|(got, (_, root))| got == root);
                s.finish(l, r, ok, "root mismatch");
            }
            _ => s.skip(l),
        }
    }
}
