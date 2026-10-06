package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A subject is a library whose commits are measured. Every library has
// targets (refs kept measured) and jobs of the same kinds. The library
// the daemon keeps a mirror of additionally has every commit of its main
// branch and its pull requests measured against their base, and checks
// reported.
const subjectDynSSZ = "dynamic-ssz"

// Target names. The operations page shows one target per subject: master
// or release; a fixed target shows in both.
const (
	targetMaster  = "master"
	targetRelease = "release"
	targetFixed   = "fixed"
)

// target is a ref of a subject's repository that is kept measured.
type target struct {
	Name string
	// Exactly one rule resolves the target to a commit:
	Branch    string   // the head of this branch
	Newest    []string // the head of whichever of these branches has the newest commit
	Tag       string   // this tag
	TagMatch  string   // the highest version among the tags matching this pattern
	PinRepo   string   // the version PinModule has in the go.mod of this repository's latest release
	PinModule string
	Npm       string // the commit the latest version of this npm package was published from
	NpmTag    string // the tag of a version when npm publishes no commit, a format with the version ("ssz-v%s")
}

type subject struct {
	Name string
	Repo string // https clone URL
	// Adapter is the library's module under harness/baselines, built per
	// commit by its recipe. Empty: the harness packages themselves, built
	// against a checkout from the mirror.
	Adapter string
	// Paths are the directories of a repository that holds more than the
	// library (a client's monorepo): a branch target then resolves to the
	// newest commit of the branch that touched one of them, and the
	// repository page lists those commits only, not every commit of the
	// client.
	Paths []string
	// Exec marks an adapter of another language: its recipe is build.sh,
	// which leaves one launcher per fork, run through benchwrap (see
	// harness/benchwrap) instead of a Go test binary.
	Exec    bool
	Targets []target
	// Weight scales how much of the idle time the library's commits get
	// next to the others' (0 counts as 1).
	Weight float64
}

const semverTag = `^v\d+\.\d+\.\d+$`

// subjects are the measured libraries. The first is the one the daemon
// mirrors; its repository and main branch come from the configuration.
var subjects = []subject{
	{Name: subjectDynSSZ, Targets: []target{{Name: targetRelease, TagMatch: semverTag}, {Name: targetMaster}}},
	{Name: "fastssz-v1", Repo: "https://github.com/ferranbt/fastssz", Adapter: "fastssz1",
		Targets: []target{{Name: targetFixed, Tag: "v1.0.0"}}},
	{Name: "fastssz", Repo: "https://github.com/ferranbt/fastssz", Adapter: "fastssz2",
		Targets: []target{{Name: targetRelease, TagMatch: `^v2\.\d+\.\d+$`}, {Name: targetMaster, Branch: "main"}}},
	{Name: "karalabe-ssz", Repo: "https://github.com/karalabe/ssz", Adapter: "karalabessz",
		Targets: []target{{Name: targetRelease, TagMatch: semverTag}, {Name: targetMaster, Branch: "main"}}},
	// Prysm ships methodical-ssz from the branch "progression", which is
	// ahead of main (and has what the adapter uses). The master target
	// follows whichever of the two has the newer head, so it returns to
	// main once the work is merged there.
	{Name: "prysm-ssz", Repo: "https://github.com/OffchainLabs/methodical-ssz", Adapter: "prysmssz",
		Targets: []target{{Name: targetRelease, PinRepo: "https://github.com/OffchainLabs/prysm", PinModule: "github.com/OffchainLabs/methodical-ssz"}, {Name: targetMaster, Newest: []string{"main", "progression"}}}},
	// The libraries of other languages (adapters with build.sh, run through
	// benchwrap). The Rust crates of Sigma Prime are three repositories;
	// ethereum_ssz is the one followed, the adapter pins ssz_types and
	// tree_hash to the revisions Lighthouse builds with.
	{Name: "lighthouse-ssz", Repo: "https://github.com/sigp/ethereum_ssz", Adapter: "ethereumssz", Exec: true,
		Targets: []target{{Name: targetRelease, TagMatch: `^v\d+\.\d+\.\d+$`}, {Name: targetMaster, Branch: "main"}}},
	{Name: "lodestar-ssz", Repo: "https://github.com/ChainSafe/ssz", Adapter: "lodestarssz", Exec: true,
		Targets: []target{{Name: targetRelease, Npm: "@chainsafe/ssz", NpmTag: "ssz-v%s"}, {Name: targetMaster, Branch: "master"}}},
	{Name: "teku-ssz", Repo: "https://github.com/Consensys/teku", Adapter: "tekussz", Exec: true,
		Paths:   []string{"infrastructure/ssz", "infrastructure/bytes", "infrastructure/crypto"},
		Targets: []target{{Name: targetRelease, TagMatch: `^\d{2}\.\d+\.\d+$`}, {Name: targetMaster, Branch: "master"}}},
	{Name: "nim-ssz", Repo: "https://github.com/status-im/nim-ssz-serialization", Adapter: "nimssz", Exec: true,
		Targets: []target{{Name: targetMaster, Branch: "master"}}},
	{Name: "grandine-ssz", Repo: "https://github.com/grandinetech/grandine", Adapter: "grandinessz", Exec: true,
		Paths:   []string{"ssz", "ssz_derive", "hashing"},
		Targets: []target{{Name: targetRelease, TagMatch: `^\d+\.\d+\.\d+$`}, {Name: targetMaster, Branch: "develop"}}},
	{Name: "sszpp", Repo: "https://github.com/OffchainLabs/sszpp", Adapter: "sszpp", Exec: true,
		Targets: []target{{Name: targetMaster, Branch: "main"}}},
}

func subjectByName(name string) *subject {
	for i := range subjects {
		if subjects[i].Name == name {
			return &subjects[i]
		}
	}
	return nil
}

// configureSubjects fills in what the mirrored library takes from the
// configuration.
func configureSubjects(cfg *config) {
	sub := subjectByName(subjectDynSSZ)
	sub.Repo = "https://github.com/" + cfg.github
	for i := range sub.Targets {
		if sub.Targets[i].Name == targetMaster {
			sub.Targets[i].Branch = cfg.mainBranch
		}
	}
	// A library of another language is measured once its adapter has a
	// build recipe in the harness; until then it is left out, so that no
	// job of it is queued.
	kept := subjects[:0]
	for _, sub := range subjects {
		if sub.Exec {
			if _, err := os.Stat(filepath.Join(cfg.harnessDir, "baselines", sub.Adapter, "build.sh")); err != nil {
				log.Printf("library %s left out: no build.sh for its adapter %s in the harness", sub.Name, sub.Adapter)
				continue
			}
		}
		kept = append(kept, sub)
	}
	subjects = kept
}

func (sub *subject) weight() float64 {
	if sub.Weight <= 0 {
		return 1
	}
	return sub.Weight
}

func strconvI(n int64) string { return strconv.FormatInt(n, 10) }

// splitTrim splits a list and drops empty parts.
func splitTrim(s, sep string) []string {
	var out []string
	for _, part := range strings.Split(s, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// mirrored reports whether the daemon keeps a mirror of the subject's
// repository.
func (sub *subject) mirrored() bool { return sub.Adapter == "" }

// repoURL is where the commits of a subject can be looked at.
func repoURL(name string) string {
	if s := subjectByName(name); s != nil {
		return s.Repo
	}
	return ""
}

// subjectHash identifies what a subject's jobs are built from besides the
// library itself: the harness version of its jobs.
func subjectHash(dir string, sub *subject) (string, error) {
	if sub.mirrored() {
		return harnessHash(dir)
	}
	return libHash(dir, sub)
}

// libHash identifies a library adapter and the kit it shares with the
// harness: the harness version of that library's jobs. An adapter of
// another language is identified by every source file of its directory,
// the toolchain recipes, the wrapper and the protocol.
func libHash(dir string, sub *subject) (string, error) {
	adapter := "baselines/" + sub.Adapter + "/"
	return hashTree(dir, func(rel string) bool {
		switch {
		case strings.HasPrefix(rel, "kit/"):
			return harnessSource(rel)
		case sub.Exec && (strings.HasPrefix(rel, "toolchains/") || strings.HasPrefix(rel, "benchwrap/") || strings.HasPrefix(rel, "protocol/")):
			return true
		case strings.HasPrefix(rel, adapter):
			if !sub.Exec {
				return harnessSource(rel)
			}
			return !builtOutput(strings.TrimPrefix(rel, adapter))
		}
		return false
	})
}

// builtOutput reports whether a path inside an adapter's directory is
// what a build leaves behind rather than a source of it.
func builtOutput(rel string) bool {
	for _, dir := range []string{"target/", "node_modules/", "build/", ".gradle/", "out/", "lib/", "dist/", "nimcache/", "work/"} {
		if strings.HasPrefix(rel, dir) {
			return true
		}
	}
	return false
}

// targetState is what a target of a subject points to.
type targetState struct {
	Subject, Name string
	SHA           string // commit, or the version a pin names
	Label         string // branch or tag the commit was found under
	Checked       time.Time
}

func (s *store) setTarget(t targetState) error {
	_, err := s.db.Exec(`INSERT INTO targets(subject, name, sha, label, checked) VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(subject, name) DO UPDATE SET sha = excluded.sha, label = excluded.label, checked = excluded.checked`,
		t.Subject, t.Name, t.SHA, t.Label, time.Now().Unix())
	return err
}

func (s *store) targets() ([]targetState, error) {
	rows, err := s.db.Query(`SELECT subject, name, sha, label, checked FROM targets ORDER BY subject, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []targetState
	for rows.Next() {
		var t targetState
		var checked int64
		if err := rows.Scan(&t.Subject, &t.Name, &t.SHA, &t.Label, &checked); err != nil {
			return nil, err
		}
		t.Checked = time.Unix(checked, 0)
		out = append(out, t)
	}
	return out, rows.Err()
}

// remoteRefs lists the branches and tags of a repository: name -> commit.
// An annotated tag resolves to the commit it points at.
func remoteRefs(ctx context.Context, repo string) (branches, tags map[string]string, err error) {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "--tags", repo).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("ls-remote %s: %w", repo, err)
	}
	branches, tags = map[string]string{}, map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || len(sha) != 40 {
			continue
		}
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			branches[strings.TrimPrefix(ref, "refs/heads/")] = sha
		case strings.HasSuffix(ref, "^{}"):
			tags[strings.TrimSuffix(strings.TrimPrefix(ref, "refs/tags/"), "^{}")] = sha
		case strings.HasPrefix(ref, "refs/tags/"):
			name := strings.TrimPrefix(ref, "refs/tags/")
			if _, peeled := tags[name]; !peeled {
				tags[name] = sha
			}
		}
	}
	return branches, tags, nil
}

// versionLess orders dotted version tags numerically.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, _ := strconv.Atoi(pa[i])
		nb, _ := strconv.Atoi(pb[i])
		if na != nb {
			return na < nb
		}
	}
	return len(pa) < len(pb)
}

// highestTag returns the highest version among the tags matching pattern.
func highestTag(tags map[string]string, pattern string) (string, bool) {
	re := regexp.MustCompile(pattern)
	var names []string
	for name := range tags {
		if re.MatchString(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", false
	}
	sort.Slice(names, func(i, k int) bool { return versionLess(names[i], names[k]) })
	return names[len(names)-1], true
}

var pseudoVersion = regexp.MustCompile(`-([0-9a-f]{12})$`)

// pinnedVersion reads the version a module has in the go.mod of a GitHub
// repository at a ref. A pseudo-version yields its commit prefix.
func pinnedVersion(ctx context.Context, repo, ref, module string) (string, error) {
	raw := strings.Replace(strings.TrimSuffix(repo, ".git"), "https://github.com/", "https://raw.githubusercontent.com/", 1) + "/" + ref + "/go.mod"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", raw, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == module && !strings.Contains(line, "=>") {
			if m := pseudoVersion.FindStringSubmatch(f[1]); m != nil {
				return m[1], nil
			}
			return f[1], nil
		}
	}
	return "", fmt.Errorf("%s does not require %s at %s", repo, module, ref)
}

// commitTimes remembers the commit times looked up; a commit's time does
// not change.
var commitTimes sync.Map

// commitTime returns when a commit of a GitHub repository was committed.
func commitTime(ctx context.Context, repo, sha string) (time.Time, error) {
	if v, ok := commitTimes.Load(sha); ok {
		return v.(time.Time), nil
	}
	api := strings.Replace(strings.TrimSuffix(repo, ".git"), "https://github.com/", "https://api.github.com/repos/", 1) + "/commits/" + sha
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("%s: %s", api, resp.Status)
	}
	var out struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return time.Time{}, err
	}
	commitTimes.Store(sha, out.Commit.Committer.Date)
	return out.Commit.Committer.Date, nil
}

// resolveTargets finds the commit of every target of a library; dataDir
// holds the history repositories a path-restricted branch is read from.
func resolveTargets(ctx context.Context, dataDir string, sub *subject) ([]targetState, error) {
	branches, tags, err := remoteRefs(ctx, sub.Repo)
	if err != nil {
		return nil, err
	}
	var out []targetState
	for _, t := range sub.Targets {
		st := targetState{Subject: sub.Name, Name: t.Name}
		switch {
		case t.Branch != "" && len(sub.Paths) > 0:
			sha, err := lastTouching(ctx, historyDir(dataDir, sub), sub, t.Branch)
			if err != nil {
				return nil, err
			}
			st.SHA, st.Label = sha, t.Branch
		case t.Branch != "":
			st.SHA, st.Label = branches[t.Branch], t.Branch
		case len(t.Newest) > 0:
			var newest time.Time
			for _, b := range t.Newest {
				sha, ok := branches[b]
				if !ok {
					continue
				}
				when, err := commitTime(ctx, sub.Repo, sha)
				if err != nil {
					return nil, err
				}
				if st.SHA == "" || when.After(newest) {
					st.SHA, st.Label, newest = sha, b, when
				}
			}
		case t.Tag != "":
			st.SHA, st.Label = tags[t.Tag], t.Tag
		case t.TagMatch != "":
			if name, ok := highestTag(tags, t.TagMatch); ok {
				st.SHA, st.Label = tags[name], name
			}
		case t.Npm != "":
			version, sha, err := npmLatest(ctx, t.Npm)
			if err != nil {
				return nil, err
			}
			if sha == "" && t.NpmTag != "" {
				sha = tags[fmt.Sprintf(t.NpmTag, version)]
			}
			if sha == "" {
				return nil, fmt.Errorf("npm %s %s: no commit published with it", t.Npm, version)
			}
			st.SHA, st.Label = sha, "npm "+version
		case t.PinRepo != "":
			_, pinTags, err := remoteRefs(ctx, t.PinRepo)
			if err != nil {
				return nil, err
			}
			release, ok := highestTag(pinTags, semverTag)
			if !ok {
				return nil, fmt.Errorf("%s has no release tag", t.PinRepo)
			}
			version, err := pinnedVersion(ctx, t.PinRepo, release, t.PinModule)
			if err != nil {
				return nil, err
			}
			// A commit prefix is completed from the branches when one of
			// them points at it, so that it equals the master target.
			st.SHA, st.Label = version, "pinned by "+strings.TrimPrefix(t.PinRepo, "https://github.com/")+" "+release
			for _, sha := range branches {
				if strings.HasPrefix(sha, version) {
					st.SHA = sha
				}
			}
		}
		if st.SHA == "" {
			return nil, fmt.Errorf("%s: target %s resolves to nothing", sub.Name, t.Name)
		}
		out = append(out, st)
	}
	return out, nil
}

// targetJobExists reports whether a commit has a job as its head with this
// harness version already (in any state: a commit that failed to build is
// not tried again until the harness changes), or one waiting.
func (s *store) targetJobExists(subject, sha, harness string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM jobs WHERE subject = ? AND head_sha = ? AND (harness = ? OR state IN (?, ?))`,
		subject, sha, harness, stateQueued, stateRunning).Scan(&n)
	return n > 0, err
}

// targetJob measures the commit one or more targets of a library point
// to, on its own: a commit job when the main branch is among them, a
// release job otherwise.
func targetJob(sub *subject, sha string, names, labels []string, desc, note string) *job {
	kind := kindRelease
	for _, n := range names {
		if n == targetMaster {
			kind = kindCommit
		}
	}
	if desc == "" {
		short := sha
		if len(short) > 12 {
			short = short[:12]
		}
		desc = short + " " + sub.Name + " " + strings.Join(labels, ", ")
	}
	return &job{Kind: kind, Subject: sub.Name, Targets: strings.Join(names, "+"), Branch: strings.Join(labels, ", "), HeadSHA: sha, HeadDesc: desc, Note: note}
}

// localTargets resolves the targets of the mirrored library from the
// mirror.
func (s *scheduler) localTargets(sub *subject) ([]targetState, error) {
	var out []targetState
	for _, t := range sub.Targets {
		switch {
		case t.Branch != "":
			if sha, ok := s.git.branches2(t.Branch); ok {
				out = append(out, targetState{Subject: sub.Name, Name: t.Name, SHA: sha, Label: t.Branch})
			}
		case t.TagMatch != "":
			if tag, sha, err := s.git.latestRelease(); err == nil {
				out = append(out, targetState{Subject: sub.Name, Name: t.Name, SHA: sha, Label: tag})
			}
		}
	}
	return out, nil
}

// pollTargets resolves the targets of every library (those of other
// repositories only when remote is set) and queues a job for a commit
// that has none with the current harness version.
func (s *scheduler) pollTargets(ctx context.Context, remote bool) {
	for i := range subjects {
		sub := &subjects[i]
		var states []targetState
		var err error
		if sub.mirrored() {
			states, err = s.localTargets(sub)
		} else if remote {
			states, err = resolveTargets(ctx, s.cfg.dataDir, sub)
		} else {
			continue
		}
		if err != nil {
			log.Printf("targets of %s: %v", sub.Name, err)
			continue
		}
		hash, err := subjectHash(s.cfg.harnessDir, sub)
		if err != nil {
			log.Printf("targets of %s: %v", sub.Name, err)
			continue
		}
		bySHA := map[string][]targetState{}
		var order []string
		for _, st := range states {
			if err := s.db.setTarget(st); err != nil {
				log.Printf("targets of %s: %v", sub.Name, err)
			}
			if st.Name == targetMaster {
				s.updateHistory(ctx, sub, st.Label)
			}
			if _, ok := bySHA[st.SHA]; !ok {
				order = append(order, st.SHA)
			}
			bySHA[st.SHA] = append(bySHA[st.SHA], st)
		}
		// A queued job for a commit the targets have moved away from is
		// superseded: the one queued for the new commit replaces it.
		if len(order) > 0 {
			if skipped, err := s.db.supersedeTargetJobs(sub.Name, order); err != nil {
				log.Printf("targets of %s: %v", sub.Name, err)
			} else if len(skipped) > 0 {
				log.Printf("targets of %s: %d queued job(s) superseded", sub.Name, len(skipped))
			}
		}
		for _, sha := range order {
			if ok, err := s.db.targetJobExists(sub.Name, sha, hash); err != nil || ok {
				continue
			}
			var names, labels []string
			for _, st := range bySHA[sha] {
				names, labels = append(names, st.Name), append(labels, st.Label)
			}
			j := targetJob(sub, sha, names, labels, s.describe(sub, sha), "")
			j.Priority = 1
			if err := s.db.insertJob(j); err != nil {
				log.Printf("targets of %s: %v", sub.Name, err)
				continue
			}
			log.Printf("queue %s %s (%s)", sub.Name, sha, strings.Join(labels, ", "))
		}
	}
}

// describe returns the one-line description of a commit where the mirror
// has it.
func (s *scheduler) describe(sub *subject, sha string) string {
	if sub.mirrored() {
		return s.git.describe(sha)
	}
	return ""
}

// migrateJobKinds brings jobs queued before every library had the same
// kinds of jobs to the current form: a job of the former kind "lib"
// becomes a commit or release job, and the commit jobs of the mirrored
// library's main branch get their target.
func (s *store) migrateJobKinds(mainBranch string) error {
	rows, err := s.db.Query(`SELECT id, subject, branch, head_sha, head_desc FROM jobs WHERE kind = 'lib'`)
	if err != nil {
		return err
	}
	type old struct {
		id                         int64
		subject, branch, sha, desc string
	}
	var jobs []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.subject, &o.branch, &o.sha, &o.desc); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, o)
	}
	rows.Close()
	for _, o := range jobs {
		sub := subjectByName(o.subject)
		if sub == nil {
			continue
		}
		// The description was "<library> <labels> (<commit>)".
		labels := strings.TrimPrefix(o.desc, o.subject+" ")
		if i := strings.LastIndex(labels, " ("); i >= 0 {
			labels = labels[:i]
		}
		j := targetJob(sub, o.sha, strings.Split(o.branch, "+"), strings.Split(labels, ", "), "", "")
		if _, err := s.db.Exec(`UPDATE jobs SET kind = ?, targets = ?, branch = ?, head_desc = ? WHERE id = ?`, j.Kind, j.Targets, j.Branch, j.HeadDesc, o.id); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`UPDATE jobs SET targets = ? WHERE subject = ? AND kind = ? AND branch = ? AND targets = ''`, targetMaster, subjectDynSSZ, kindCommit, mainBranch)
	return err
}

// npmLatest resolves the latest version of an npm package to the commit it
// was published from (the registry's gitHead of that version).
func npmLatest(ctx context.Context, pkg string) (version, sha string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://registry.npmjs.org/"+pkg, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("npm %s: HTTP %s", pkg, resp.Status)
	}
	var meta struct {
		DistTags map[string]string `json:"dist-tags"`
		Versions map[string]struct {
			GitHead string `json:"gitHead"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&meta); err != nil {
		return "", "", fmt.Errorf("npm %s: %w", pkg, err)
	}
	version = meta.DistTags["latest"]
	if version == "" {
		return "", "", fmt.Errorf("npm %s: no latest version", pkg)
	}
	// The commit is published with the version when the publisher's
	// checkout was a git one; empty otherwise, for the caller's tag.
	if sha = meta.Versions[version].GitHead; len(sha) != 40 {
		sha = ""
	}
	return version, sha, nil
}
