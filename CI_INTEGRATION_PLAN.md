# Benchmark box as an external CI check — plan

Status: implemented locally and tested (2026-10-03); the daemon side is
not deployed yet (a daemon restart interrupts the running job), the web
side is. Waiting for: the GitHub App (owner action), the word to deploy
the daemon and the sandbox, and the word to publish the code.

## Decisions

| topic | decision |
|---|---|
| Reporting | GitHub App; one check run "benchmark" per measured commit |
| Fork PRs | measured only after a maintainer opts in per PR; sandboxed |
| Verdict | informational: conclusion `neutral`, never a required check |
| Old workflows | `benchmark.yml` keeps the library micro-benchmarks; the block/state perftests go |

## Architecture

1. Pull model, outbound only. The box polls git and the pull request list
   (as today, every 60 s) and reports through the GitHub API. No inbound
   connection, no webhook.
2. GitHub App "dynamic-ssz benchmarks" installed on the repository.
   Permissions: Checks read/write, Pull requests read, Contents read,
   Metadata read. The box holds the App id and private key on the data
   disk, signs a JWT, exchanges it for an installation token (1 h) and
   refreshes it as needed.
3. Check run life cycle per job:
   - job queued       -> check `queued` (position in queue in the summary)
   - job starts       -> `in_progress`
   - after every pass -> summary updated with the provisional tables
   - job done         -> `completed`, conclusion `neutral`
   - job failed/box down -> `completed`, conclusion `skipped`/`cancelled`
     with the reason; a check left `queued` longer than a limit is closed
     by the next daemon start.
   The summary is self-contained Markdown (reviewers cannot open the
   private UI): per engine geomean, then per object a table of operations
   with base -> head, delta, and a marker for results beyond the noise
   floor; the reference libraries column; link to the UI for those who can
   reach it. `details_url` points at the UI job page.
4. Queue rules for pull requests:
   - one job per PR head against the PR base (merge base, as today);
   - a new head removes that PR's queued jobs (their checks are closed as
     `skipped: superseded by <sha>`); the running job finishes;
   - PR jobs run before backfill and idle work; priority jobs (noise,
     baseline) stay ahead.
5. Per-commit impact: every measured head stays in the history. The PR
   view lists the heads in order with head-vs-base results; consecutive
   heads are compared from stored values (compare page); an exact
   head-vs-previous-head job can be queued by hand or by the idle rotation.
6. Master commits get the same check on the merge commit (against its
   parent), which gives a per-merge history in GitHub's commit list.

## Fork pull requests

- Never measured automatically. Opt-in: a maintainer (owner, member or
  collaborator) submits a review on the pull request whose text contains
  `/benchmark`. GitHub binds a review to the commit it was written on, so
  the approval covers exactly that head; a later push needs a new review.
  (A label was the first idea; a label is not tied to a commit, which
  leaves a window between labelling and the next poll in which a push
  would be measured unreviewed.)
- The head is fetched from `refs/pull/<n>/head` and must equal the
  approved commit.
- Sandbox, a precondition (`-sandbox-user`; without it no fork is
  queued): builds run as an unprivileged user that can reach the internet
  on ports 80/443 and DNS but nothing in the private ranges; benchmark
  processes additionally get no network at all; neither sees the daemon's
  credentials, the database or the App key. `deploy/setup-sandbox.sh`
  prepares the VM.

## Changes to the repository

- `benchmark.yml`: remove the "Run perf benchmarks" step and what only it
  needs (test data download, codegen for perftests); keep the library
  micro-benchmarks, benchstat and the comment workflow.
- `tests/perftests`: stays as a manual tool unless removed separately.

## Changes to the box (when implemented)

- daemon: GitHub App auth, check-run reporter (create/update/close),
  supersede rule, PR head tracking incl. `refs/pull/*/head` for approved
  forks, label check, Markdown summary renderer.
- sandbox user + network rules in the guest.
- UI: PR page (heads in order, per-commit deltas).

## Open points

1. Home of the daemon and harness: today in the gitignored `ai_plans/`.
   A CI dependency needs a versioned place (own repository, or a
   directory in this one).
2. App creation and installation is done by the repository owner; the box
   needs the App id, installation id and private key.
3. Name of the opt-in label and who counts as maintainer (write access).
4. Whether the micro-benchmarks move to the box later.

## What exists now

| piece | where | state |
|---|---|---|
| GitHub App client (JWT, installation token) | `benchd/github.go` | tested against a fake API |
| check run reporter (queued, in progress per pass, completed, skipped, failed) | `benchd/github.go` | tested |
| Markdown summary (per engine geomean, per object tables, noise marking) | `benchd/github.go` | tested |
| supersede rule for queued jobs of a branch | `benchd/db.go`, `benchd/git.go` | tested |
| fork approval through reviews, fetch of the approved head | `benchd/git.go` | approval rule tested |
| sandbox wrapping of builds and benchmark processes | `benchd/runner.go`, `deploy/setup-sandbox.sh` | wrapping tested; VM setup not run |
| pull request history page and API | `benchd/web.go`, `benchd/ui` | deployed |
| public address | `https://sszbench.pk910.de` (Apache reverse proxy; `/runner` and `/admin` are not forwarded) | live |
| workflow change | `benchmark-workflow.patch` (removes the block/state perftests, keeps the library micro-benchmarks) | patch only |
| home of the code | orphan branch `benchbox` of pk910/ssz-benchmark, worktree `../ssz-benchmark-benchbox` | files copied, not committed |

## Configuration (in `/srv/benchd/env`, read by the daemon)

    PUBLIC_URL=https://sszbench.pk910.de
    GH_APP_ID=<app id>
    GH_INSTALLATION_ID=<installation id>
    GH_APP_KEY=/srv/benchd/github-app.pem      (mode 600, root)
    SANDBOX_USER=bench

## Creating the GitHub App (repository owner)

1. GitHub: Settings -> Developer settings -> GitHub Apps -> New GitHub App.
   Name e.g. "dynamic-ssz benchmarks", homepage `https://sszbench.pk910.de`,
   webhook inactive.
2. Repository permissions: Checks read and write, Pull requests read-only,
   Contents read-only, Metadata read-only. No account permissions, no
   events.
3. "Only on this account", create; note the App ID; generate a private key
   (downloads a `.pem`).
4. Install the App on `pk910/dynamic-ssz` only. The installation id is the
   number at the end of the installation's settings URL.
5. Put the `.pem` on the VM as `/srv/benchd/github-app.pem` (root, 600) and
   the ids into `/srv/benchd/env`.
