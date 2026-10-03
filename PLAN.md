# Benchmark box: plan

Status: approved and built 2026-10-03; README.md is the operating manual.
Noise findings (same day): the hypervisor-granted clock, not the machine's
load, was behind the 5-12% spreads of identical binaries; the fix is to
compare cycles per op from the PMU (user mode) rather than wall time,
with the collector held off the timed path (GOGC off, forced collections
between iteration batches) and one scheduler thread for the synchronous
engines. Remaining: evaluate the floor in cycles from noise jobs.
Additions after approval: Gloas payloads converted from the Fulu ones (same
content, progressive types), a `GloasEnvelope` object, the harness split
into `fulu` and `gloas` packages (a commit without Gloas type support still
measures Fulu), first-run backfill of the last 10 master commits, every
master commit measured against its parent, a priority noise job every 6
hours, interrupts pinned to cpus 0-1, budget 12 minutes.

## 1. Goal

Per commit of dynamic-ssz, on a dedicated machine, a low-noise comparison of
every engine × object × operation against the commit's base, with time,
bytes and allocations per operation, next to fixed baselines of other SSZ
libraries on the identical payload, with the full history kept and shown per
operation.

## 2. Payload (built once, immutable on the box)

Source: real mainnet Fulu state at slot 15303838 (337 MB, 2,373,539
validators, 891,748 active) and the 32 real blocks 15303807–15303838, pulled
2026-09-27 (local copy `ai_plans/perf-v1.4.0/data/mainnet`).

Generator `ai_plans/benchbox/harness/cmd/payload` (written, not yet run):

- `state.ssz`: the real state with eth1_data_votes filled to 2048 (hard
  maximum) and pending_deposits / pending_partial_withdrawals /
  pending_consolidations filled to 65,536 each (real: 22,311 / 29 / 8,176;
  spec limits 134M / 134M / 262,144 are not realistic). Filled entries are
  seeded variations of real-shaped entries. ~355 MB.
- `block.ssz`: real block 15303826 (largest of the epoch, 284 transactions)
  with every operation list at the mainnet-spec limit: 16 proposer
  slashings, 1 attester slashing (2 × 27,867 indices = one slot's attesters),
  8 attestations (64 committee bits, 27,867 aggregation bits, all set), 16
  deposits, 16 exits, 16 BLS changes, 21 blob commitments (BLOB_SCHEDULE
  maximum at that epoch), 8192 deposit requests, 16 withdrawal requests, 2
  consolidation requests, 16 withdrawals. ~2.1 MB.
- `blocks/000..031.ssz`: the 32 real blocks unchanged (one set = one
  iteration, covers a whole epoch of real variety).
- `*.root` next to each file (hash tree root), `spec.json`, `meta.json`
  (source slots, fill counts).

Open: pending fill 65,536 (proposed).

## 3. Harness (`ai_plans/benchbox/harness`, module `realbench`)

Lives outside the repository and is built against each checkout through a
`replace` directive, so every commit, including old bases, runs identical
benchmark code. Types: the Fulu/Electra types of the perf campaign harness
(`types/types.go`), generated code produced per checkout by that checkout's
`dynssz-gen` (`-legacy -with-streaming`).

Benchmark names `BenchmarkReal/<Engine>/<Object>/<Op>`:

| axis | values |
|---|---|
| Engine | `Codegen` (generated code), `Reflection` (`WithNoFastSsz` + `WithNoDelegation`) |
| Baseline (library reference, not a function of the commit) | `FastSSZ`: go-eth2-client v0.27.0 fastssz-generated methods on the same bytes; later candidates: karalabe/ssz (needs codec definitions for all Fulu types), prysm (heavy dependency) |
| Object | `State` (extended state), `Block` (extended block), `Blocks` (32 real blocks per iteration) |
| Op | `Unmarshal` (buffer), `UnmarshalReader` (stream, size known), `UnmarshalReaderUnknown` (stream, size unknown), `SizeSSZ`, `Marshal` (buffer, allocating), `MarshalTo` (buffer, reused), `MarshalWriter` (stream), `HashTreeRoot`, `GetTree` (treeproof) |

Baseline engines offer the operations their library has (fastssz: Unmarshal,
SizeSSZ, Marshal, MarshalTo, HashTreeRoot, GetTree).

Metrics per leaf: ns/op, B/op, allocs/op, MB/s.

Correctness is part of every leaf: decode results are re-encoded and must
equal the input bytes, roots must equal the stored roots. A commit that
produces wrong output fails its job visibly instead of producing numbers.

Per-process setup is read + decode only (no root check at setup), so each
leaf can run as its own process (needed for interleaving, below) at ~0.3 s
overhead for the state.

Async hashing rows are left out: the benchmark core is a single CPU.

## 4. Measurement method

- Per job: head and base checkouts, harness copied and built per side; one
  test binary per layout seed (101, 202, 303, 404; `-funcalign=64
  -randlayout=<seed>`), same flags both sides.
- Leaf discovery from the head binary (1 iteration each). Fixed iteration
  count per leaf, calibrated once on first sight (the count Go settles on
  for 500 ms, at least 2), stored in the database and reused for both sides
  and every later job: identical work per measurement across sides and
  across time. State HashTreeRoot (1.2 s) and GetTree (4.7 s) therefore run
  2 iterations per measurement.
- Interleaving per leaf: base then head back to back (order alternates per
  pass); passes over the seeds, then further rounds, until the budget is
  spent, at least two passes. Baseline leaves run once per pass on the head
  binary only.
- Process environment: `setarch -R` (no address space randomization),
  `taskset -c 2` with cpu 3 (SMT sibling) idle, `GOMAXPROCS=1`, `GOGC=100`;
  the service and builds on cpus 0–1. `GODEBUG=madvdontneed=0` is tested
  against the noise floor (large output buffers are re-faulted after the
  scavenger returns them; MADV_FREE avoids that) and kept if it helps.
- Steal guard: CPU steal ticks of cpu 2 are read before and after every
  measurement; a measurement with steal during it is repeated once and
  the steal count is stored with the sample.
- Statistics per leaf and metric: mean, median, CV per side; delta of
  means with 95% Welch interval and p-value; delta of medians. The noise
  floor comes from self-comparison jobs (same commit both sides), shown
  per leaf as p95 |delta|.
- Budget: 10 minutes of measurement per job (proposed; one pass ≈ 2–2.5
  min with 2 iterations for the state ops), so 4–5 passes.

Noise floor of the first method (identical binaries, 4 samples per side,
repository benchmarks): median |delta| 0.63%, p95 3.6%; outliers came from
single hiccups in sub-microsecond benchmarks, which this suite does not have.

## 5. Scheduling (unchanged from the first attempt, verified working)

Mirror fetched every minute; new branch tip → job against merge base with
the pull request base (GitHub lookup) or master; master commits against the
first parent; idle: master vs latest release, every 4th idle job master vs
itself (noise floor). First start queues only master and branches with an
open pull request. Interrupted jobs are queued again. Local-only admin
endpoint to queue jobs by hand with priority.

## 6. Web UI and API

- **Job page**: one matrix per object (State, Block, Blocks): rows =
  operations, column groups = engines (Codegen, Reflection) and baselines.
  Each cell: base → head ns/op, Δ% with 95% interval, B/op with Δ, allocs/op
  with Δ; baseline cells absolute, plus the ratio dynssz/baseline. Summary
  per engine × object (geomean of time ratios) instead of one number.
- **Operation page** `/op/<Object>/<Op>`: all engines and baselines side by
  side; master history charts for ns/op, B/op, allocs/op with trend and
  30-day projection; every job that measured it.
- **Compare page**: any two measured commits (or jobs) side by side per
  operation; cross-job comparisons are marked as such (not interleaved).
- **Noise page**: per-leaf noise floor from the self-comparison jobs, steal
  counts, CV distribution, so the method can be judged.
- **Jobs list, dashboard** with the running job and queue.
- JSON for everything (`/api/jobs`, `/api/job/<id>`, `/api/op/<obj>/<op>`,
  `/api/compare?a=<sha>&b=<sha>`, `/api/noise`); raw benchmark output per
  job.

## 7. Rollout

1. Run the generator locally, check the payload (decode with both engines
   and the fastssz types, roots agree), upload to `/srv/benchd/res/real`.
2. Build and test the harness locally against master (all leaves pass
   their checks).
3. Rewrite the runner for the harness and the method above; local
   end-to-end smoke run with a small budget.
4. Deploy; first job: noise floor (master vs master), then master vs
   v1.3.3, then the open pull requests.
5. Compare the noise floor with and without `madvdontneed=0`; keep the
   better setting.
