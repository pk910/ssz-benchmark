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
// it brought in. The repository pages list it, measured or not.

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

// historyHead identifies the stored history of a library: the newest
// commit and the tags, so that a new tag on an old commit renews it too.
func (s *store) historyHead(subject string) string {
	var head string
	_ = s.db.QueryRow(`SELECT sha || ' ' || (SELECT coalesce(group_concat(tags, ','), '') FROM (SELECT tags FROM branch_commits WHERE subject = ?1 ORDER BY pos)) FROM branch_commits WHERE subject = ?1 AND pos = 0`, subject).Scan(&head)
	return head
}

func historyID(commits []branchCommit) string {
	var tags []string
	for _, c := range commits {
		tags = append(tags, strings.Join(c.Tags, " "))
	}
	return commits[0].SHA + " " + strings.Join(tags, ",")
}

func (s *store) setBranchCommits(subject string, commits []branchCommit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM branch_commits WHERE subject = ?`, subject); err != nil {
		return err
	}
	for i, c := range commits {
		if _, err := tx.Exec(`INSERT INTO branch_commits(subject, sha, pos, committed, title, tags) VALUES(?, ?, ?, ?, ?, ?)`,
			subject, c.SHA, i, c.Committed, c.Title, strings.Join(c.Tags, " ")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// branchCommits returns the stored history of a library's main branch,
// newest first.
func (s *store) branchCommits(subject string) ([]branchCommit, error) {
	rows, err := s.db.Query(`SELECT sha, committed, title, tags FROM branch_commits WHERE subject = ? ORDER BY pos`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []branchCommit
	for rows.Next() {
		var c branchCommit
		var tags string
		if err := rows.Scan(&c.SHA, &c.Committed, &c.Title, &tags); err != nil {
			return nil, err
		}
		c.Tags = strings.Fields(tags)
		out = append(out, c)
	}
	return out, rows.Err()
}

// updateHistory renews the stored history of a library's main branch: from
// the mirror for the mirrored library, from a fetch of the branch for the
// others (only when remote is set).
func (s *scheduler) updateHistory(ctx context.Context, sub *subject, branch string, remote bool) {
	gitDir := s.git.dir
	if !sub.mirrored() {
		if !remote {
			return
		}
		gitDir = filepath.Join(s.cfg.dataDir, "work", "history", sub.Name+".git")
		fctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := fetchHistory(fctx, gitDir, sub.Repo, branch)
		cancel()
		if err != nil {
			log.Printf("history of %s: %v", sub.Name, err)
			return
		}
	}
	commits, err := branchLog(ctx, gitDir, "refs/heads/"+branch)
	if err != nil || len(commits) == 0 {
		log.Printf("history of %s: %v (%d commits)", sub.Name, err, len(commits))
		return
	}
	if historyID(commits) == s.db.historyHead(sub.Name) {
		return
	}
	if err := s.db.setBranchCommits(sub.Name, commits); err != nil {
		log.Printf("history of %s: %v", sub.Name, err)
	}
}
