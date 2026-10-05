//! The adapter of ethereum_ssz for benchwrap: the harness objects decoded
//! into the generated Rust types (gen_fulu.rs, gen_gloas.rs, one module
//! per preset), every operation measured with the kit's policy, the
//! figures sent over the protocol (harness/benchwrap).
//!
//! Operations: Unmarshal (`from_ssz_bytes`), SizeSSZ (`ssz_bytes_len`),
//! Marshal (`as_ssz_bytes`), MarshalTo (`ssz_append` into a buffer kept
//! across iterations) and HashTreeRoot (`tree_hash_root`). Objects of a
//! fork: the state, the block, the block set, and (Gloas) the envelope,
//! plus the minimal-preset state and block.
//!
//! Memory: a counting global allocator gives the bytes and allocations per
//! operation. Freeing is not part of an operation: as the kit collects
//! between batches with the clock stopped, the results of a batch are
//! dropped between batches with the counters paused. The heap is kept
//! warm as the kit keeps Go's (`keep_heap`): freed memory stays mapped
//! for the next iteration instead of being faulted in again.
#![allow(clippy::all)]

mod gen_fulu;
mod gen_gloas;
mod protocol;

use protocol::{Leaf, Session};
use ssz::{Decode, Encode};
use std::alloc::{GlobalAlloc, Layout, System};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use tree_hash::{Hash256, TreeHash};

const ENGINE: &str = "EthereumSSZ";
const OPS: [&str; 5] = ["Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"];

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

/// Keeps the heap warm as the kit's does for Go. By default glibc serves
/// the large buffers of a state from mappings of their own and returns
/// them to the kernel when they are dropped, so that every iteration
/// faults its memory in again (half the measured time of a state decode
/// was kernel time), where the Go runtime reuses the pages its collector
/// freed. No mapping per allocation and no trimming of the heap: what an
/// iteration frees stays mapped for the next one, as the warm-up calls
/// intend.
fn keep_heap() {
    let ok = unsafe { libc::mallopt(libc::M_MMAP_MAX, 0) == 1 && libc::mallopt(libc::M_TRIM_THRESHOLD, -1) == 1 };
    if !ok {
        eprintln!("mallopt failed: the heap may be returned to the kernel between iterations");
    }
}

fn main() {
    keep_heap();
    let args: Vec<String> = std::env::args().collect();
    let fork = args
        .iter()
        .position(|a| a == "--fork")
        .and_then(|i| args.get(i + 1))
        .cloned()
        .unwrap_or_else(|| "fulu".to_string());
    let data = PathBuf::from(std::env::var("REAL_DATA").unwrap_or_else(|_| "/srv/benchd/res/real".to_string()));
    let mut s = Session::open();
    s.thread();
    let dir = data.join(&fork);
    let min = dir.join("minimal");
    match fork.as_str() {
        "fulu" => {
            use gen_fulu::{mainnet as m, minimal as n};
            one::<m::FuluBeaconState>(&mut s, "FuluState", &dir, "state", |v| v.tree_hash_root());
            one::<m::ElectraSignedBeaconBlock>(&mut s, "FuluBlock", &dir, "block", |v| v.Message.tree_hash_root());
            set::<m::ElectraSignedBeaconBlock>(&mut s, "FuluBlocks", &dir, "blocks", |v| v.Message.tree_hash_root());
            one::<n::FuluBeaconState>(&mut s, "FuluMinState", &min, "state", |v| v.tree_hash_root());
            one::<n::ElectraSignedBeaconBlock>(&mut s, "FuluMinBlock", &min, "block", |v| v.Message.tree_hash_root());
        }
        "gloas" => {
            use gen_gloas::{mainnet as m, minimal as n};
            one::<m::GloasBeaconState>(&mut s, "GloasState", &dir, "state", |v| v.tree_hash_root());
            one::<m::GloasSignedBeaconBlock>(&mut s, "GloasBlock", &dir, "block", |v| v.Message.tree_hash_root());
            set::<m::GloasSignedBeaconBlock>(&mut s, "GloasBlocks", &dir, "blocks", |v| v.Message.tree_hash_root());
            one::<m::GloasSignedExecutionPayloadEnvelope>(&mut s, "GloasEnvelope", &dir, "envelope", |v| v.Message.tree_hash_root());
            one::<n::GloasBeaconState>(&mut s, "GloasMinState", &min, "state", |v| v.tree_hash_root());
            one::<n::GloasSignedBeaconBlock>(&mut s, "GloasMinBlock", &min, "block", |v| v.Message.tree_hash_root());
        }
        other => {
            eprintln!("unknown fork {other}");
            std::process::exit(2);
        }
    }
}

/// The leaves of an object the pattern selects.
fn leaves(s: &Session, object: &str) -> Vec<Leaf> {
    OPS.iter()
        .map(|op| Leaf::new(ENGINE, object, op))
        .filter(|l| s.matches(l))
        .collect()
}

fn read_root(path: &Path) -> Hash256 {
    let text = fs::read_to_string(path).unwrap_or_else(|e| panic!("{}: {e}", path.display()));
    let hex = text.trim().trim_start_matches("0x");
    let mut out = [0u8; 32];
    for i in 0..32 {
        out[i] = u8::from_str_radix(&hex[2 * i..2 * i + 2], 16).expect("root hex");
    }
    Hash256::from(out)
}

/// A payload: the bytes of one or more objects with their roots.
struct Payload {
    items: Vec<(Vec<u8>, Hash256)>,
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

/// Decodes every item of the payload and checks it as the kit does:
/// re-encoded bytes equal the input, the root equals the stored one.
fn verify<T: Encode + Decode + TreeHash>(p: &Payload, root_of: &dyn Fn(&T) -> Hash256) -> Result<Vec<T>, String> {
    let mut out = Vec::with_capacity(p.items.len());
    for (data, root) in &p.items {
        let v = T::from_ssz_bytes(data).map_err(|e| format!("decode: {e:?}"))?;
        if v.as_ssz_bytes() != *data {
            return Err("decoded value does not encode back to the input".to_string());
        }
        let got = root_of(&v);
        if got != *root {
            return Err(format!("root mismatch: got {got:x} want {root:x}"));
        }
        out.push(v);
    }
    Ok(out)
}

fn one<T: Encode + Decode + TreeHash>(s: &mut Session, object: &str, dir: &Path, file: &str, root_of: impl Fn(&T) -> Hash256) {
    let ls = leaves(s, object);
    if ls.is_empty() {
        return;
    }
    let p = load_one(dir, file);
    bench::<T>(s, &ls, &p, &root_of);
}

fn set<T: Encode + Decode + TreeHash>(s: &mut Session, object: &str, dir: &Path, name: &str, root_of: impl Fn(&T) -> Hash256) {
    let ls = leaves(s, object);
    if ls.is_empty() {
        return;
    }
    let p = load_set(dir, name);
    bench::<T>(s, &ls, &p, &root_of);
}

/// Measures the operations of an object; one iteration of a set runs the
/// operation on every item, and every operation checks its last result
/// as the kit does once the timer is stopped: decoded values encode back
/// to the input, encoded bytes equal the input, sizes equal the input
/// length, roots equal the stored roots. A decoded object lives in the
/// vector the iteration returns (the one allocation that holds it, as the
/// kit's `New` does); the encodings of a set likewise. A single object's
/// encoding and root are returned bare, the roots of a set go into a
/// vector allocated once, so that no holder allocation is counted where
/// the kit has none.
fn bench<T: Encode + Decode + TreeHash>(s: &mut Session, ls: &[Leaf], p: &Payload, root_of: &dyn Fn(&T) -> Hash256) {
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
    for l in ls {
        match l.op.as_str() {
            "Unmarshal" => {
                let d = s.run(l, || {
                    p.items
                        .iter()
                        .map(|(data, _)| T::from_ssz_bytes(data).expect("decode"))
                        .collect::<Vec<T>>()
                });
                let check = encodes_back(&d.last, p);
                s.finish(d, check);
            }
            "SizeSSZ" => {
                let d = s.run(l, || decoded.iter().map(|v| v.ssz_bytes_len()).sum::<usize>());
                let check = if d.last == total { Ok(()) } else { Err(format!("size {} want {total}", d.last)) };
                s.finish(d, check);
            }
            "Marshal" if single => {
                let d = s.run(l, || decoded[0].as_ssz_bytes());
                let check = same_bytes(std::slice::from_ref(&d.last), p, "marshal");
                s.finish(d, check);
            }
            "Marshal" => {
                let d = s.run(l, || decoded.iter().map(|v| v.as_ssz_bytes()).collect::<Vec<Vec<u8>>>());
                let check = same_bytes(&d.last, p, "marshal");
                s.finish(d, check);
            }
            "MarshalTo" => {
                // One buffer across the iterations: the input's size for
                // one object, the kit's 4 MB for the items of a set.
                let mut buf: Vec<u8> = Vec::with_capacity(if single { total } else { 4 << 20 });
                let d = s.run(l, || {
                    for v in &decoded {
                        buf.clear();
                        v.ssz_append(&mut buf);
                    }
                });
                let (want, _) = &p.items[p.items.len() - 1];
                let check = if buf == *want { Ok(()) } else { Err("marshalTo output differs from input".to_string()) };
                s.finish(d, check);
            }
            "HashTreeRoot" if single => {
                let d = s.run(l, || root_of(&decoded[0]));
                let check = same_roots(std::slice::from_ref(&d.last), p);
                s.finish(d, check);
            }
            "HashTreeRoot" => {
                let mut roots: Vec<Hash256> = Vec::with_capacity(decoded.len());
                let d = s.run(l, || {
                    roots.clear();
                    roots.extend(decoded.iter().map(|v| root_of(v)));
                });
                let check = same_roots(&roots, p);
                s.finish(d, check);
            }
            _ => s.skip(l),
        }
    }
}

/// The kit's check of a decode: every value encodes back to its input.
fn encodes_back<T: Encode>(vs: &[T], p: &Payload) -> Result<(), String> {
    for (i, (v, (data, _))) in vs.iter().zip(&p.items).enumerate() {
        if v.as_ssz_bytes() != *data {
            return Err(format!("decoded value {i} does not encode back to the input"));
        }
    }
    Ok(())
}

/// The kit's check of an encode: every output equals its input.
fn same_bytes(outs: &[Vec<u8>], p: &Payload, op: &str) -> Result<(), String> {
    for (i, (out, (data, _))) in outs.iter().zip(&p.items).enumerate() {
        if out != data {
            return Err(format!("item {i} {op} output differs from input"));
        }
    }
    Ok(())
}

/// The kit's check of a hash: every root equals the stored one.
fn same_roots(got: &[Hash256], p: &Payload) -> Result<(), String> {
    for (i, (g, (_, want))) in got.iter().zip(&p.items).enumerate() {
        if g != want {
            return Err(format!("root {i} mismatch: got {g:x} want {want:x}"));
        }
    }
    Ok(())
}
