package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The history of a library's main branch, as its repository has it: the
// commits on the branch itself, a merge as one commit without the commits
// it brought in. The repository pages read it from a local repository
// when they are built, and list it, measured or not.

type branchCommit struct {
	SHA       string
	Committed int64
	Title     string
	Tags      []string
}

// branchLog reads the commits on a branch of a local repository, newest
// first, following first parents only.
func branchLog(ctx context.Context, gitDir, ref string) ([]branchCommit, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", gitDir, "log", "--first-parent",
		"--decorate-refs=refs/tags", "--format=%H%x09%ct%x09%D%x09%s", ref, "--").Output()
	if err != nil {
		return nil, fmt.Errorf("log of %s: %w", ref, err)
	}
	var commits []branchCommit
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 || len(f[0]) != 40 {
			continue
		}
		c := branchCommit{SHA: f[0], Title: f[3]}
		c.Committed, _ = strconv.ParseInt(f[1], 10, 64)
		for _, d := range strings.Split(f[2], ", ") {
			if tag, ok := strings.CutPrefix(d, "tag: "); ok {
				c.Tags = append(c.Tags, tag)
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// fetchHistory brings the commits (without their files) and the tags of a
// branch of another repository into a local repository kept for that.
func fetchHistory(ctx context.Context, gitDir, repo, branch string) error {
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		if err := os.MkdirAll(filepath.Dir(gitDir), 0o755); err != nil {
			return err
		}
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", "--bare", gitDir).CombinedOutput(); err != nil {
			return fmt.Errorf("init %s: %v: %s", gitDir, err, out)
		}
	}
	out, err := exec.CommandContext(ctx, "git", "-C", gitDir, "fetch", "-q", "--filter=tree:0", "--force", repo,
		"+refs/heads/"+branch+":refs/heads/"+branch, "+refs/tags/*:refs/tags/*").CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch %s %s: %v: %s", repo, branch, err, out)
	}
	return nil
}

// historyDir is the local repository that has the history of a library's
// main branch: the mirror, or the repository kept for the commits of
// another library's branch.
func historyDir(dataDir string, sub *subject) string {
	if sub.mirrored() {
		return filepath.Join(dataDir, "repo.git")
	}
	return filepath.Join(dataDir, "work", "history", sub.Name+".git")
}

// updateHistory fetches the commits of another library's main branch. The
// mirrored library's are in the mirror.
func (s *scheduler) updateHistory(ctx context.Context, sub *subject, branch string) {
	if sub.mirrored() {
		return
	}
	fctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := fetchHistory(fctx, historyDir(s.cfg.dataDir, sub), sub.Repo, branch); err != nil {
		log.Printf("history of %s: %v", sub.Name, err)
	}
}
