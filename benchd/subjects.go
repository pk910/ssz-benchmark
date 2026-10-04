package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A subject is a library whose commits are measured. dynamic-ssz is the
// subject with branches, pull requests and two-sided jobs; the other
// libraries are measured one-sided at the commits their targets point to.
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
	Branch    string // the head of this branch
	Tag       string // this tag
	TagMatch  string // the highest version among the tags matching this pattern
	PinRepo   string // the version PinModule has in the go.mod of this repository's latest release
	PinModule string
}

type subject struct {
	Name    string
	Repo    string // https clone URL
	Adapter string // the library's module under harness/baselines
	Targets []target
}

const semverTag = `^v\d+\.\d+\.\d+$`

// librarySubjects are the other SSZ libraries.
var librarySubjects = []subject{
	{Name: "fastssz-v1", Repo: "https://github.com/ferranbt/fastssz", Adapter: "fastssz1",
		Targets: []target{{Name: targetFixed, Tag: "v1.0.0"}}},
	{Name: "fastssz", Repo: "https://github.com/ferranbt/fastssz", Adapter: "fastssz2",
		Targets: []target{{Name: targetRelease, TagMatch: `^v2\.\d+\.\d+$`}, {Name: targetMaster, Branch: "main"}}},
	{Name: "karalabe-ssz", Repo: "https://github.com/karalabe/ssz", Adapter: "karalabessz",
		Targets: []target{{Name: targetRelease, TagMatch: semverTag}, {Name: targetMaster, Branch: "main"}}},
	// Prysm ships methodical-ssz from the branch "progression"; its main
	// branch is older and lacks what the adapter uses.
	{Name: "methodical-ssz", Repo: "https://github.com/OffchainLabs/methodical-ssz", Adapter: "prysmssz",
		Targets: []target{{Name: targetRelease, PinRepo: "https://github.com/OffchainLabs/prysm", PinModule: "github.com/OffchainLabs/methodical-ssz"}, {Name: targetMaster, Branch: "progression"}}},
}

func subjectByName(name string) *subject {
	for i := range librarySubjects {
		if librarySubjects[i].Name == name {
			return &librarySubjects[i]
		}
	}
	return nil
}

// repoURL is where the commits of a subject can be looked at.
func repoURL(cfg *config, name string) string {
	if s := subjectByName(name); s != nil {
		return s.Repo
	}
	return "https://github.com/" + cfg.github
}

// libHash identifies a library adapter and the kit it shares with the
// harness: the harness version of that library's jobs.
func libHash(dir, adapter string) (string, error) {
	return hashTree(dir, func(rel string) bool {
		return strings.HasPrefix(rel, "baselines/"+adapter+"/") || strings.HasPrefix(rel, "kit/")
	})
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

// resolveTargets finds the commit of every target of a library.
func resolveTargets(ctx context.Context, sub *subject) ([]targetState, error) {
	branches, tags, err := remoteRefs(ctx, sub.Repo)
	if err != nil {
		return nil, err
	}
	var out []targetState
	for _, t := range sub.Targets {
		st := targetState{Subject: sub.Name, Name: t.Name}
		switch {
		case t.Branch != "":
			st.SHA, st.Label = branches[t.Branch], t.Branch
		case t.Tag != "":
			st.SHA, st.Label = tags[t.Tag], t.Tag
		case t.TagMatch != "":
			if name, ok := highestTag(tags, t.TagMatch); ok {
				st.SHA, st.Label = tags[name], name
			}
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

// libJobExists reports whether a library commit has a job with this
// harness version already (in any state: a commit that failed to build is
// not tried again until the adapter changes), or one waiting.
func (s *store) libJobExists(subject, sha, harness string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM jobs WHERE kind = ? AND subject = ? AND head_sha = ? AND (harness = ? OR state IN (?, ?))`,
		kindLib, subject, sha, harness, stateQueued, stateRunning).Scan(&n)
	return n > 0, err
}

// libJob is a one-sided measurement of a library commit.
func libJob(sub *subject, sha string, names, labels []string, note string) *job {
	short := sha
	if len(short) > 12 {
		short = short[:12]
	}
	return &job{Kind: kindLib, Subject: sub.Name, Branch: strings.Join(names, "+"), HeadSHA: sha,
		HeadDesc: sub.Name + " " + strings.Join(labels, ", ") + " (" + short + ")", Note: note}
}

// pollLibraries resolves the targets of every library and queues a job for
// a commit that has none with the current adapter.
func (s *scheduler) pollLibraries(ctx context.Context) {
	for i := range librarySubjects {
		sub := &librarySubjects[i]
		states, err := resolveTargets(ctx, sub)
		if err != nil {
			log.Printf("library %s: %v", sub.Name, err)
			continue
		}
		hash, err := libHash(s.cfg.harnessDir, sub.Adapter)
		if err != nil {
			log.Printf("library %s: %v", sub.Name, err)
			continue
		}
		bySHA := map[string][]targetState{}
		var order []string
		for _, st := range states {
			if err := s.db.setTarget(st); err != nil {
				log.Printf("library %s: %v", sub.Name, err)
			}
			if _, ok := bySHA[st.SHA]; !ok {
				order = append(order, st.SHA)
			}
			bySHA[st.SHA] = append(bySHA[st.SHA], st)
		}
		for _, sha := range order {
			if ok, err := s.db.libJobExists(sub.Name, sha, hash); err != nil || ok {
				continue
			}
			var names, labels []string
			for _, st := range bySHA[sha] {
				names, labels = append(names, st.Name), append(labels, st.Label)
			}
			j := libJob(sub, sha, names, labels, "")
			j.Priority = 1
			if err := s.db.insertJob(j); err != nil {
				log.Printf("library %s: %v", sub.Name, err)
				continue
			}
			log.Printf("queue %s", j.HeadDesc)
		}
	}
}

// idleLibJob is a rerun of the least measured library commit among the
// current targets; nil when none can be measured.
func (s *scheduler) idleLibJob() (*job, error) {
	targets, err := s.db.targets()
	if err != nil {
		return nil, err
	}
	type cand struct {
		sub    *subject
		sha    string
		names  []string
		labels []string
		runs   int
	}
	var cands []*cand
	seen := map[string]*cand{}
	for _, t := range targets {
		sub := subjectByName(t.Subject)
		if sub == nil {
			continue
		}
		key := t.Subject + "|" + t.SHA
		c := seen[key]
		if c == nil {
			hash, err := libHash(s.cfg.harnessDir, sub.Adapter)
			if err != nil {
				return nil, err
			}
			var done, failed int
			if err := s.db.db.QueryRow(`SELECT coalesce(sum(state = ?), 0), coalesce(sum(state = ?), 0) FROM jobs WHERE kind = ? AND subject = ? AND head_sha = ? AND harness = ?`,
				stateDone, stateFailed, kindLib, t.Subject, t.SHA, hash).Scan(&done, &failed); err != nil {
				return nil, err
			}
			if failed > 0 || done == 0 {
				// Does not build, or its first job is still to come.
				continue
			}
			c = &cand{sub: sub, sha: t.SHA, runs: done}
			seen[key] = c
			cands = append(cands, c)
		}
		c.names, c.labels = append(c.names, t.Name), append(c.labels, t.Label)
	}
	if len(cands) == 0 {
		return nil, nil
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].runs < cands[b].runs })
	c := cands[0]
	return libJob(c.sub, c.sha, c.names, c.labels, "refinement run"), nil
}
