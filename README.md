# Benchmark box

Continuous per-commit benchmarking of dynamic-ssz on a dedicated machine with
a fixed harness on a real, extended mainnet payload in two shapes (Fulu and
Gloas). The service (`benchd/`) mirrors the repository, measures every new
commit against its base on an isolated cpu, keeps every sample in SQLite and
serves per-operation comparisons, history and noise statistics with a JSON
API.

| path | content |
|---|---|
| `benchd/` | the service (Go, modernc sqlite, no other dependencies) |
| `harness/` | the benchmark module: `kit/` (module `benchkit`: operations, counters, pinning; shared with the baselines), `baselines/` (one module per reference library), `fulu/` and `gloas/` benchmark packages, `types/fulu`, `types/gloas`, `cmd/payload` (payload generator) |
| `deploy/` | systemd units, IRQ affinity script, `deploy.sh` |
| `ai_plans/PLAN.md` (local, not in the repository) | the design |

## Machine

- Host: the benchmark VM (`dom10-benchmark`, `$BENCH_BOX` for the deploy
  scripts), a PVH guest on a Xen host (Ryzen 5 3600: 6 cores / 12 threads in two CCXs
  with a private 16 MB L3 each; Xen 4.20, credit2). The guest has 5 vCPUs
  and 16 GB: vCPUs 0-1 share host cpus 0-5 (CCX0) with dom0 and the other
  domains; vCPUs 2, 3 and 4 are pinned onto host cpus 6, 8 and 10, one
  thread of each core of CCX1. The sibling threads 7, 9 and 11 carry no
  vCPU of any domain, so nothing can run next to a benchmark thread (a
  guest cannot be trusted to keep a sibling idle: it does not see the
  topology, and kernel threads wander). Host side
  (`/etc/xen/auto/dom10-benchmark.cfg`, `xen-bench-host.service`): every
  other domain is pinned to cpus 0-5, the clock is fixed (performance
  governor, turbo off, C-states capped at C1: a constant 3.6 GHz), Xen
  boots with `dom0_mem=4096M,max:4096M dom0_max_vcpus=6 dom0_vcpus_pin
  vpmu=1` and `autoballoon="off"` in `xl.conf` (a ballooning dom0
  fragments host memory, which decides how much of the guest gets 2 MB
  backing). A change to the domain config needs `xl shutdown` + `xl
  create` on the host; a reboot inside the guest restarts it with the old
  config. Before a host reboot shut the guest down: xendomains would
  otherwise save and restore it with its old config.
- Disks: `/srv/benchd` (database, payload, job logs, results) is on the
  mirrored volume group; `/srv/benchd/work` (worktrees, harness builds, Go
  caches; `wt` and `hb` are symlinks into it) is on a single NVMe that may
  fail and holds nothing that cannot be rebuilt. 8 GB swap on the same
  NVMe.
- Kernel command line (`extra=` in the domain config; the guest boots via
  pygrub): `isolcpus=nohz,managed_irq,2-7 nohz_full=2-7 rcu_nocbs=2-7
  transparent_hugepage=madvise` (no `domain` flag: the
  scheduler must still balance a benchmark process's threads across its
  cpuset, otherwise every thread stays on the cpu the process started on).
  Everything else is kept off cpus 2-7 by `CPUAffinity=0 1` in
  `/etc/systemd/system.conf` (every service and their children), the
  `bench-irq-affinity` unit (interrupts, unbound kernel workqueues and the
  memory-management threads kcompactd, kswapd, khugepaged on cpus 0-1) and `nohz_full`/`rcu_nocbs`. The benchmark binaries run on cpus
  2, 3 and 4: the measured goroutine pins its own thread to cpu 2
  (`BENCH_MUTATOR_CPU`, locked thread + `sched_setaffinity` inside the
  harness). The synchronous engines run with `GOMAXPROCS=1`, so the
  runtime has one thread to schedule (a concurrent runtime on other cores
  costs memory bandwidth at unpredictable points); the async engines run with `GOMAXPROCS=3` so their
  hashing workers get cpus 3 and 4.
  `GODEBUG=madvdontneed=0` keeps freed buffers mapped (MADV_FREE), so they
  are not re-faulted from the kernel between iterations.
- The collector is off the timed path: the binaries run with `GOGC=off`
  (`GOMEMLIMIT=12GiB`, `-memlimit`, as a safety net under the 16 GB) and the harness forces a collection
  between iterations with the timer stopped, every iteration for operations
  allocating more than 256 MB, else often enough to keep the garbage of a
  batch under 256 MB. The heap stays warm and reused, no cycle lands inside
  a measurement (one cycle over the decoded state costs as much as an
  Unmarshal of it, so its position used to swing samples by several
  percent), and the garbage an operation makes is reported exactly as B/op
  and allocs/op instead of as time.
- `apt-daily`/`apt-daily-upgrade` timers and unattended-upgrades disabled;
  `/etc/sysctl.d/90-bench.conf`: `kernel.sched_autogroup_enabled=0`,
  `vm.swappiness=1`, `kernel.perf_event_paranoid=1`, watchdogs off.
- Go 1.27.0 in `/usr/local/go`, `GOTOOLCHAIN=local`.

## Payload (`/srv/benchd/res/real`, immutable)

Generated once by `harness/cmd/payload` from the real mainnet Fulu state at
slot 15303838 (337 MB, 2,373,539 validators, 891,748 active) and the 32 real
blocks 15303807–15303838 (local copies in `ai_plans/perf-v1.4.0/data/mainnet`):

- `fulu/state.ssz`: eth1_data_votes filled to 2048, pending_deposits /
  pending_partial_withdrawals / pending_consolidations to 65,536 each
  (seeded variations of real-shaped entries); everything else real.
- `fulu/block.ssz`: block 15303826 with every operation list at the mainnet
  limit: 16 proposer slashings, 1 attester slashing (2 × 27,867 indices),
  8 attestations (64 committee bits, 27,867 aggregation bits), 16 deposits,
  16 exits, 16 BLS changes, 21 blob commitments, 8192 deposit requests, 16
  withdrawal requests, 2 consolidation requests, 16 withdrawals, the real
  284 transactions.
- `fulu/blocks/000..031`: the real blocks unchanged.
- `gloas/`: the same content in the Gloas types (progressive lists and
  containers): `state` (plus full payload availability, 64 empty builder
  payments, a PTC window of 96 × 512 real validator indices, the latest
  bid from the latest payload header), `block` (bid built from the payload,
  4 payload attestations, execution requests as parent requests),
  `envelope` (the execution payload with its transactions and requests),
  `blocks/` (the real blocks converted, one payload attestation each).
- `*.root` next to each file, `spec.json` per shape (the Gloas one carries
  the preset values the beacon API does not report), `meta.json`.
- `fulu/minimal/` and `gloas/minimal/`: the same objects cut to the
  minimal preset (`-minimal <consensus-specs checkout>`), with the
  minimal `spec.json` and `meta-minimal.json` at the top. The registry is
  every eighth validator (296,693) with its balance, participation and
  inactivity score; the vectors are at their minimal lengths with the
  newest slots and epochs in their places (64 block roots, state roots,
  randao mixes, slashings), 32 eth1 votes, sync committees of 32, 16
  lookahead entries, the pending partial withdrawals and consolidations
  capped at 64, the pending deposits unchanged. The block keeps everything
  the preset allows: the aggregation bits and slashing indices are cut to
  a slot's committees (8,192), the committee bits to 4, the sync bits to
  32, the withdrawals to 4. Validator indices the objects refer to are
  divided by eight so they stay inside the registry. The mainnet files do
  not depend on this step and are never rewritten by it.

## Suite

`BenchmarkReal/<Engine>/<Object>/<Op>` in two packages, built against each
commit with that commit's `dynssz-gen` (`-legacy -with-streaming`):

| axis | values |
|---|---|
| Engine | `Codegen` (generated code), `Reflection` (`WithNoFastSsz` + `WithNoDelegation`); `CodegenAsync` and `ReflectionAsync` measure HashTreeRoot with `WithAsyncHashing(3)` (time and cycles only: the async hasher materializes worker buffers as scheduling demands, so its allocation counts are not a property of the code) |
| Reference libraries (their own job, see below) | `FastSSZv1` (ferranbt/fastssz v1.0.0), `FastSSZv2` (its v2 development branch), `PrysmSSZ` (methodical-ssz, the generator and runtime Prysm uses), `KaralabeSSZ` and `KaralabeSSZAsync` (karalabe/ssz v0.3.0, sequential and concurrent hasher) |
| Object | `FuluState`, `FuluBlock`, `FuluBlocks`, `GloasState`, `GloasBlock`, `GloasBlocks`, `GloasEnvelope`; `FuluMinState`, `FuluMinBlock`, `GloasMinState`, `GloasMinBlock` (the minimal-preset objects, measured with the minimal spec values on `Unmarshal`, `Marshal` and `HashTreeRoot` only: every spec value the types depend on then differs from its compiled default, which the mainnet objects never exercise) |
| Op | `Unmarshal`, `UnmarshalReader`, `UnmarshalReaderUnknown`, `SizeSSZ`, `Marshal`, `MarshalTo`, `MarshalWriter`, `HashTreeRoot`, `GetTree` |

Every leaf verifies its output (re-encoded bytes equal the input, roots
equal the stored roots); a commit producing wrong output fails its job. A
commit whose generator cannot handle a package (an old base without the
Gloas type features) is measured on the packages it builds.

## Method

- Head and base checkouts, the harness copied and built per side, one test
  binary per package and layout seed (101, 202, 303, 404; `-funcalign=64
  -randlayout=<seed>`).
- Leaves are discovered once per harness version. Each leaf has a fixed
  iteration count, calibrated on first sight (the count Go settles on for
  300 ms on the base side, at least 2) and reused by both sides and every
  later job, so every measurement does identical work.
- Per pass (one seed), every engine and object is one process per side
  (`setarch -R`, `taskset -c 2,3,4`, `GOMAXPROCS=1`, or 3 for the async
  engines, `GOGC=off`) that runs all its operations with their fixed
  iteration counts (`BENCH_ITERS`, the harness keeps its own clock and
  reports ns/op, B/op, allocs/op, cycles/op and instrs/op itself; clock
  and counters run across the iterations and pause only around the forced
  collections, since every counter toggle is a hypervisor trap; untimed calls and
  collections come before the loop (two for an allocating operation: the
  previous result is still referenced while the next call runs, so the
  loop alternates between two heap regions), so the measured iterations reuse mapped
  memory instead of paying the kernel for fresh pages (a state decode
  spent 12-25% of its wall time there); `faults/op` and `sys-ns/op` in the
  raw logs show the page faults and kernel time left inside the timed
  windows); base
  and head alternate per group, the side order per pass. Steal ticks of the measured cpu
  (cpu 2; the collector and the async workers on 4 and 6 incur wake-up
  latencies that are not disturbances) are read around every run; a run
  with steal is repeated once and the count is stored. An operation whose cycles (time, without counters)
  deviate from the median of the same side's earlier runs of it under the
  same layout seed by more than `-outlier` (4%) is repeated once on its
  own: with identical instruction counts such a deviation is a disturbance
  that lasted the whole run (host side, or kernel work on the core), and
  the second run is kept whatever it shows. The per-cpu watchdog timers
  are off (`/etc/sysctl.d/91-bench-watchdog.conf`).
- Passes over the seeds, then further rounds, continue while the 15 minute
  measurement budget (+25%) allows; at least one pass per layout seed
  (`-min-passes`, default the number of seeds). Layout effects are real
  and large: the same binary differs by 2-8% between seeds for a typical
  operation, and single layouts double the cost of one. In a comparison
  the two sides are different binaries and each gets its own luck per
  seed, so the per-pass deltas of a pair scatter by several percent while
  two runs of one binary under one seed agree within 0.2%.
- A result is therefore judged by pass: base and head of one pass are
  linked with the same seed and measured minutes apart. The headline delta
  is the median of the per-pass deltas, so one layout under which a side
  runs far off does not move it. A result is a change when that median
  lies outside its band and at least three quarters of the passes point
  the same way; the band is the larger of the operation's noise floor and
  twice the standard error of the per-pass deltas. Engine geomeans are
  built from the medians. The mean, the 95% interval and the range of the
  passes stay available per result.
- A rerun of a pair (idle refinement, the periodic noise and reference
  jobs) measures under the next of four sets of seeds (each seed moved on
  by a thousand) and pools with the earlier runs, so the result of a pair
  covers more layouts with every run. A cached build links the new seeds
  on demand.
- Per leaf and metric (ns/op, cycles/op, instructions/op, B/op,
  allocs/op): mean, median and CV per side, delta of means with 95% Welch
  interval and p-value, delta of medians. Cycles and instructions come from
  the hardware counters of the measured thread (user mode only, switched
  with the benchmark timer, so the untimed collections are excluded). On
  this VM the clock of the benchmark core wanders between about 3.0 and
  3.65 GHz with the moment and the instruction mix (the hypervisor grants
  it), so wall time of the same binary moves by several percent while
  cycles per op repeat within about 0.5% for the large operations and
  instructions per op are identical to five digits. Cycles are therefore
  the headline metric in the UI; time stays a tab away.

### Further figures per run

- **Kind of a change.** A marked change is labelled `code` when the
  instructions per operation moved by 0.5% or more (the code does
  different work) and `same work` when only its cycles did (layout, cache
  or branch behaviour).
- **Counter pairs.** The machine runs four hardware counters at once.
  Next to cycles and instructions every run counts one of two pairs,
  alternating by pass: branch misses and L2 data misses, or frontend
  stall cycles and L1 data misses. A rerun of a pair of commits starts
  with the other pair. `-counter-pairs=false` turns this off.
- **Async engines** are counted over all threads of the process (one
  counter per thread, opened after the warm-up calls; a run during which
  a thread started is repeated). Their cycles and instructions are the
  total work, not the latency.
- **Memory beyond allocations**, in the first pass: the heap the result
  keeps alive, and the stack one call needs (growth of the stack memory
  while a fresh goroutine runs it; a stack below 32 KiB reads as zero).
- **Diagnostic runs.** When the two sides of an operation executed the
  same instructions in cycles that differ by 5% or more under one layout,
  each side is run again after the passes, untimed, with the counters
  outside the rotation (decoded uops, instruction cache and TLB misses).
  At most six operations per job; the runs take no part in the
  statistics and show on the operation's page of the job.
- **Build facts** per job side and harness package: size of the binary,
  of its code and of the generated source, and the build time.

## Libraries and targets

Every measured library is defined in `benchd/subjects.go`: its
repository, how it is built, and its targets (refs that are kept
measured). All libraries have the same kinds of jobs: a **commit** job
measures a commit of the main branch, a **release** job a release; a job
measures its head, and its base when it has one.

| library | built from | fixed | release | master |
|---|---|---|---|---|
| dynamic-ssz | the harness packages against a checkout from the mirror | | latest tag | the main branch |
| fastssz-v1 | adapter `baselines/fastssz1` | `v1.0.0` | | |
| fastssz | adapter `baselines/fastssz2` | | latest `v2.x.y` tag | `main` |
| karalabe-ssz | adapter `baselines/karalabessz` | | latest tag | `main` |
| prysm-ssz (methodical-ssz) | adapter `baselines/prysmssz` | | the version the latest Prysm release pins | `main` or `progression`, whichever has the newer head (Prysm ships from `progression`, which is ahead of `main` today) |

- Targets are resolved from the mirror for the mirrored library on every
  tick, and with `git ls-remote` (and Prysm's go.mod) every 15 minutes
  (`-target-poll`) for the others. A commit a target points to that has
  no job with the current harness version gets one, ahead of the queue.
- The mirrored library additionally has every commit of its main branch
  measured against its parent, its pull requests against their base, and
  GitHub checks reported.
- A library with an adapter is built without a checkout: the job copies
  the harness and runs `baselines/<adapter>/generate.sh <commit>` in the
  sandbox (it fetches the library at that commit, runs that commit's
  generator with the Go toolchain it needs, and tidies the module), then
  links one binary per fork and seed.
- The reflection engines run on the plain types (`harness/plain`): the
  same type definitions without generated methods, so that the library
  has nothing to delegate to, in every version of it. The runner renews
  that copy from `types/<fork>/types.go` at every build.
- An option of the library that not every version has (background
  hashing) is used through the harness package `feat`, with a variant
  behind a build tag. When a checkout lacks it the runner builds with that
  tag, and the engines that need it (the async ones) are left out of that
  job. A newer version that has the option is measured in full without
  any change here.
- An operation that fails its own check on a release (that version
  cannot do it on this payload) is left out of the release job with a
  line in its log. On a commit job such a failure fails the job.
- A harness change gives its jobs a new harness version, and runs of
  different versions are not combined. When a change did not touch what
  is measured, `curl localhost/admin/harness?subject=<library>` shows how
  the same commits compare under the two versions, and a POST with
  `from` and `to` takes the earlier version's jobs over into the newer.
- A build that fails fails the job with its error; the commit is not
  tried again until the harness version changes or the target moves.
- `baselines/gen.sh` is the offline step: it converts the harness types
  for every adapter (needs Python) and runs the recipes at their pinned
  default versions. Its output is checked in.
- Adding a library: an adapter directory with its types, benchmark file
  and `generate.sh`, and an entry in `subjects`.
- Libraries of other languages (`harness/baselines/FOREIGN.md`): an
  adapter with `build.sh` instead of Go packages, marked `Exec` in
  `subjects`. The build sources a toolchain recipe
  (`harness/toolchains/<lang>.sh`), which installs or updates the pinned
  toolchain under `-toolchains` (`/srv/benchd/work/toolchains`, the work
  disk) on first use, fetches the library at the commit, generates the
  type definitions from the harness types with
  `baselines/convert_foreign.py` (one set per preset) and leaves one
  launcher per fork (and per layout seed where the linker can shuffle
  sections: Rust, C++). The runner runs the launcher through `benchwrap`
  (`harness/benchwrap`), which passes the pattern and the iteration counts
  on, switches the hardware counters of the adapter's measuring thread on
  its messages (a pipe on file descriptor 3: `thread`, `begin`, `pause`,
  `resume`, `end`, `fail`, `skip`), pins that thread to the benchmark cpu,
  and prints the Go benchmark lines the runner reads. An adapter verifies
  every object as the kit does, warms up untimed (a JIT runtime until it
  has settled, reported as `warmup`), and frees or collects between
  batches with the counters paused, so the figures mean what the kit's
  mean: time and counters of the operation itself, memory where the
  runtime can count it. A library without a `build.sh` in the deployed
  harness is left out of the subjects, so nothing is queued for it. The
  wrapper was validated against the kit with a Go adapter speaking the
  protocol (`harness/adapters/dynssz`): same time, bytes, allocations and
  page faults on the same leaves.
- Idle time refines what was measured before (`benchd/idle.go`). Every
  commit worth another run is a candidate with a weight: the head of an
  open pull request 8, each of its earlier heads half of the one above
  (4, 2, 1, then none); a library's latest release 6 and the head of its
  main branch 4, with the commits below that head of the mirrored library
  halving in the same way; times the library's own weight (`Weight` in
  its definition, 1 for all today). The scheduler takes the candidate with
  the fewest runs in the last 48 hours per unit of weight; among equals
  the library that ran longest ago, and of it the commit that ran longest
  ago. A commit without a finished job of the current harness version is
  no candidate (its first job is still to come, or it does not build).
- The noise floor comes from self-comparisons (`benchd/noise.go`): two
  runs of one commit under the same layout seeds in different jobs of the
  same harness version, machine and boot, which idle refinement produces
  when a seed set comes round again, and jobs that measured one commit on
  both sides. The newest 40 count. No job is scheduled for it.
- A refinement run measures one pass per layout seed, with the next seed
  set of that commit or pair; its runs are pooled with the earlier ones.
- One-sided jobs (`-one-sided`): a job with a base measures its head and
  takes each base run from an earlier run of the base commit with the same
  seed (same harness and Go version, machine and boot, at most three days
  old, not on a pathological layout) when the head run lies within
  `-base-threshold` (1%) of it; otherwise the base is measured in the job
  and checked against the head. A base that is not measured more often
  than the head is always measured by the job, so the base never falls
  behind.
- Pooled values: when a job finishes, the values of the commits it
  measured are recomputed over every job of the same harness version that
  had the commit on either side (`commit_values`). The Operations page
  shows them per library at its master or its release target, in master
  mode with the change against the release; a job's page shows the other
  libraries' release values next to its own results.

## Scheduling

- Mirror fetched every minute. First run: the last 10 first-parent commits
  of master (each squash-merged pull request is one), each against its
  parent, and the tips of branches with an open pull request; every other
  existing tip is taken as seen.
- Afterwards: every new first-parent commit on master against its parent
  (up to 10 per push); any other branch tip against its merge base with the
  pull request base (open pull requests are looked up on GitHub) or master.
- Idle: the machine never waits. The rotation is master against itself
  (noise floor), a refinement run, master against the latest release tag,
  another refinement run. A refinement run repeats the finished commit
  comparison with the fewest runs so far; its samples are stored as a new
  job and the pair's results are recomputed from all runs pooled (the
  fixed iteration counts make the samples compatible), so every rerun
  tightens the intervals of that comparison and spreads it over time.
- Idle time is planned ahead: when nothing waits, the refinement runs of
  the next six hours are queued together (behind any job that arrives
  meanwhile), so the queue shows what the machine will do. No run follows
  one of the same commit, and none one of the same pull request or
  library while another candidate is there: a commit that is far behind
  catches up in turns with the others. A planned run whose commit is no
  candidate any more when its turn comes is dropped.
- A refinement run gives way: when a job that is no refinement waits and
  the run is less than half through, it stops between two groups of
  benchmarks, its runs so far are dropped and it goes back into the queue
  behind every job that is no refinement; the waiting job starts at once.
  Past the half it finishes first.
- Other libraries: see "Libraries and targets". What each can express:

  | library | Fulu state | Fulu block(s) | Gloas block(s), envelope | Gloas state |
  |---|---|---|---|---|
  | PrysmSSZ (prysm-ssz) | everything | everything | everything, hashing included | everything |
  | FastSSZv1, FastSSZv2 | everything | everything | serialization only | not expressible |
  | KaralabeSSZ | not expressible | everything | serialization only | not expressible |

  "Serialization only": no progressive merkleization. "Not expressible":
  the generator has no vector of containers (Gloas state), or, for
  karalabe/ssz, no vector of 64 uint64 among its closed list of vector
  lengths (Fulu's proposer lookahead). Bounds given to progressive lists
  for these generators are nominal (they have none) and do not affect
  serialization.
- Interrupted jobs are queued again. Jobs by hand (local only):
  `curl -X POST localhost/admin/queue -d kind=commit -d head=<ref> -d base=<ref> -d priority=2`
  (`kind=noise` / `kind=release` for idle-style jobs).

## Web UI and API (port 80)

One page (`benchd/ui`, embedded; Chart.js vendored) over the JSON API, hash
routes:

- `#/` overview, with two tabs: the running job with progress, the queue
  in execution order and the newest finished jobs; and `#/jobs`, every
  job with filters.
- Links lead to pages here; every link to GitHub stands behind the small
  GitHub mark next to a commit, a repository or a pull request.
- `#/repos`: every library in a box of its own with its repository, its
  targets, the five newest commits of its main branch and the number of
  its open pull requests. `#/repo/<library>`: all of them,
  200 a page, each with its age, its tags and, when measured, the change per engine
  against the measured commit before it (from the pooled values of both,
  within one harness version), and charts with one line per operation: its
  time (or cycles) per call at every measured commit, aggregated over the
  payload types (geometric mean, or sum), operations of one kind
  (unmarshalling, marshalling, hashing) sharing a chart. The change column shows one badge per engine or one
  per engine and operation, and engines can be hidden. For the mirrored
  library the page also lists its open pull requests (the daemon stores
  what it polls from GitHub), each head compared with the head of the main
  branch from the pooled values of both; the pull request's own page has
  every measured head against the job's base. The list is the history of the branch as the repository has it:
  the commits on the branch itself, a merge as one commit. It is read from
  a local repository when the page is built: the mirror, or, for another
  library, `work/history/<library>.git`, into which the target poller
  fetches the commits of the branch without their files. Commits without a
  measurement are listed as such.
- As text, for an agent given a link: `/llms.txt` explains how a page
  link maps to text and JSON and how to read the numbers; `/job/<id>.md`,
  `/commit/<library>/<sha>.md` (`?base=`) and `/repo/<library>.md`
  (`?page=`) give a job, a commit and a library's commits as Markdown,
  with the verdict, band and kind of change per operation and links to
  the JSON, the single runs and the log files. The pages link their text
  version; the page source and a `noscript` note point to the guide.
- `#/job/<id>`: per object a delta chart (bars = 95% intervals, ticks =
  means) and a matrix operations × engines, each cell base → head with the
  delta and its interval; a metric switch (time / memory / allocations)
  applies to the whole page; async engines appear as an "async" line inside
  the parent engine's HashTreeRoot cell; one "other libraries" column with
  each reference library's value and ours ÷ theirs per engine (from the
  latest baseline job); a running job shows provisional results and
  refreshes itself.
- `#/job/<id>/leaf/<Engine>/<Object>/<Op>`: every sample of one leaf as a
  scatter per side with the side means, pooled over the runs of the pair.
- `#/ops`, `#/op/<Object>/<Op>`: latest values per engine, master history
  chart with the metric switch, trend and 30-day projection, every job that
  measured the operation.
- `#/compare?a=<ref>&b=<ref>`: any two measured commits per operation.
- `#/noise`: per-leaf noise floor (|Δ| of identical binaries) per metric,
  sortable by p95 or CV, steal counts.
- JSON: `/api/status`, `/api/dashboard`, `/api/jobs?kind=&limit=`,
  `/api/job/<id>`, `/api/job/<id>/samples`, `/api/job/<id>/leaf/<e>/<o>/<op>`,
  `/api/ops`, `/api/op/<Object>/<Op>`, `/api/compare?a=&b=`, `/api/noise`;
  raw files `/raw/<job>/<file>`.
- UI work without the box: `go test -run TestWriteDemoDB` with
  `BENCHD_DEMO_DB=<dir>/benchd.db` writes a synthetic database, then
  `benchd -web -data <dir> -listen 127.0.0.1:8099`.

### Jobs and commits

- A **job** page (`#/job/<id>`) shows what that job measured, nothing
  else.
- A **commit** page (`#/commit/<library>/<sha>`) shows everything measured
  of one commit: the runs of every job of the same harness version that
  had it on either side, compared with the runs of a base commit chosen on
  the page. The default base is the base of the pull request the commit
  was measured for; for a release the release before it; otherwise the
  library's latest release. The runs of the two commits are paired by
  layout seed (per seed the median of each side's runs); a seed only one
  of them was run under has no pair. Refinement runs of either commit
  sharpen the page of every commit compared with it.

## Processes

Two processes from one binary, two units:

- `benchd.service` (`benchd -no-web ...`): the daemon. Mirror, scheduler,
  job runner. It publishes its live state (job, phase, progress, fetch
  status) into the database (`kv` table, key `status`) a few times per
  minute and has no HTTP.
- `benchweb.service` (`benchweb -web ...`): the web UI and API, reading the
  database only (SQLite in WAL mode: readers never block the writer).
  Queuing by hand (`/admin/queue`) also goes through this process; the
  daemon picks the row up. Restarting it never touches a running job. The
  dashboard says so when the daemon has not reported for two minutes.

## Worker machines

Extra machines run benchmarks for the controller during busy times:
`benchd -controller http://<controller>` with `RUNNER_TOKEN` (the secret in
`/srv/benchd/env` on the controller; `benchweb` serves the runner API under
`/runner/` with it). A worker keeps its own git mirror, the harness source
and the payload; it claims jobs, builds the harness itself, streams samples
and progress to the controller and never stores results or serves anything.
`deploy/deploy-worker.sh root@<worker>` sets one up (after the same kernel
tuning as the controller; see the script header).

Qualification: a new worker gets only master-vs-master noise jobs until the
controller has checked it: the p95 of |Δ time| over the leaves of its noise
job must be under `-qualify-p95` (1.5%), the geomean of its per-leaf times
over the controller's latest noise run of the same harness within
`-qualify-ratio` (±15%), and the spread of those ratios under
`-qualify-spread` (8%). A failed check queues another noise job; after
`-qualify-retries` (3) failures the worker is rejected and gets nothing.
Every job records its runner, results pool only with reruns on the same
machine, and the noise page shows the floor per machine. The dashboard
lists every runner with its state.

## Housekeeping on the box

- After every job the runner prunes the work disk: harness builds made
  with a harness that is no longer current are removed at once; of the
  rest the sixteen most recently used worktrees and builds stay.
- The Go build cache trims itself (entries unused for five days); the
  module cache only grows with new dependencies.
- Job logs (`/srv/benchd/jobs/<id>`) are gzipped when the job finishes
  (about 55 KB per job) and kept; the UI serves them as plain text.
- The database keeps, per job and operation, the values the pages
  across jobs read. The single runs of a finished job are one gzipped
  blob (`job_samples`); the job page computes its detailed statistics
  from it. A running job has its samples as rows until it finishes.

## GitHub integration

- A GitHub App reports every commit job as a check run named `benchmark`
  on its head commit: queued, in progress with the provisional tables
  after every pass, completed with conclusion `neutral` (the check
  informs, it never fails a commit). A job superseded by a newer push
  closes as `skipped`. Settings come from the daemon's environment file:
  `PUBLIC_URL`, `GH_APP_ID`, `GH_INSTALLATION_ID`, `GH_APP_KEY` (path of
  the private key). Without them no check is written.
- A new head of a branch marks that branch's queued jobs as skipped; the
  running one finishes. `#/pr/<n>` lists every measured head of a pull
  request against the base and against the head before it.
- Pull requests from forks are measured only inside the sandbox
  (`SANDBOX_USER`, `deploy/setup-sandbox.sh`): builds run as an
  unprivileged user without access to private address ranges, benchmark
  processes without any network. A head is measured when the repository's
  own rule for forks let it through: GitHub ran the workflows for it (the
  author is known to the repository, or a maintainer approved the run
  there). A run that still waits for that approval does not count. A
  maintainer can also approve a specific head directly:
  - apply the label `benchmark` (`-approve-label`). The App's webhook
    (`/webhook`, signed with `GH_APP_WEBHOOK_SECRET`, event "Pull
    request") delivers the head the label was applied to; the daemon
    removes the label once the job is queued, so a new push needs it
    again;
  - submit a review whose text contains `/benchmark`; GitHub binds the
    review to the commit it was written on.
- The web service behind a reverse proxy must not forward `/runner` and
  `/admin`.

## Operate

```
deploy/deploy.sh ui              # frontend or API change: restarts only benchweb
deploy/deploy.sh daemon          # runner or harness change: restarts benchd (the running job is queued again)
deploy/deploy.sh                 # both
ssh $BENCH_BOX journalctl -u benchd -f
```

Data under `/srv/benchd`: `repo.git`, `wt/<sha>`, `hb/<harness>-<sha>`
(harness builds), `jobs/<id>/` (job.log, report.txt, raw benchmark output),
`res/real/`, `benchd.db`. `/srv/benchd/env` may carry `GITHUB_TOKEN=` for
the pull request lookup.
