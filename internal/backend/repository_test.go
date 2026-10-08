package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRepository(t *testing.T) *Repository {
	t.Helper()
	repo := NewRepository(t.TempDir())
	repoCommand(t, repo, "init", "-q", "-b", "main")
	for _, config := range [][2]string{{"user.name", "Rex Test"}, {"user.email", "rex@example.invalid"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}, {"core.hooksPath", filepath.Join(repo.directory, "hooks")}} {
		repoCommand(t, repo, "config", config[0], config[1])
	}
	return repo
}
func repoCommand(t *testing.T, repo *Repository, args ...string) string {
	t.Helper()
	out, err := repo.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}
func repoWrite(t *testing.T, repo *Repository, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo.directory, name), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func repoSnapshot(t *testing.T, repo *Repository, selected string, staged bool) GitSnapshot {
	t.Helper()
	snapshot, err := repo.Snapshot(selected, staged)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func repoFile(t *testing.T, snapshot GitSnapshot, path string) GitFile {
	t.Helper()
	for _, file := range snapshot.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("file %q not in %+v", path, snapshot.Files)
	return GitFile{}
}

func TestRepositorySeparatesIndexAndWorktreeAndCommitsOnlyIndex(t *testing.T) {
	repo := testRepository(t)
	repoWrite(t, repo, "change.txt", "base\n")
	repoCommand(t, repo, "add", "--all")
	repoCommand(t, repo, "commit", "-qm", "Initial")
	repoWrite(t, repo, "change.txt", "staged\n")
	repoCommand(t, repo, "add", "--", "change.txt")
	repoWrite(t, repo, "change.txt", "working\n")
	repoWrite(t, repo, "untracked.txt", "keep\n")
	index := repoSnapshot(t, repo, "change.txt", true)
	work := repoSnapshot(t, repo, "change.txt", false)
	file := repoFile(t, work, "change.txt")
	if file.Status != "MM" || !file.Staged() || !file.Unstaged() {
		t.Fatalf("mixed status: %+v", file)
	}
	if !index.SelectedStaged || !strings.Contains(index.Diff, "+staged") || strings.Contains(index.Diff, "+working") {
		t.Fatalf("index diff: %s", index.Diff)
	}
	if work.SelectedStaged || !strings.Contains(work.Diff, "-staged") || !strings.Contains(work.Diff, "+working") {
		t.Fatalf("worktree diff: %s", work.Diff)
	}
	if out, err := repo.Commit("Only staged changes"); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	if got := repoCommand(t, repo, "show", "HEAD:change.txt"); got != "staged\n" {
		t.Fatalf("committed unstaged content: %q", got)
	}
	if _, err := repo.Commit("Empty index"); err == nil {
		t.Fatal("committed without staged changes")
	}
	if _, err := repo.Commit("   "); err == nil {
		t.Fatal("accepted empty commit message")
	}
	snapshot := repoSnapshot(t, repo, "change.txt", false)
	file = repoFile(t, snapshot, "change.txt")
	if _, err := repo.Stage(file); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UnstageAll(); err != nil {
		t.Fatal(err)
	}
	if got := repoCommand(t, repo, "diff", "--cached"); got != "" {
		t.Fatalf("index still populated: %s", got)
	}
	data, err := os.ReadFile(filepath.Join(repo.directory, "change.txt"))
	if err != nil || string(data) != "working\n" {
		t.Fatalf("unstage changed working content: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(repo.directory, "untracked.txt")); err != nil {
		t.Fatal("lost untracked file:", err)
	}
}

func TestRepositoryUnstageBeforeFirstCommitKeepsEditedFiles(t *testing.T) {
	repo := testRepository(t)
	path := ":(glob)draft [1].txt"
	repoWrite(t, repo, path, "staged\n")
	snapshot := repoSnapshot(t, repo, path, false)
	if snapshot.HasHEAD || snapshot.SelectedStaged {
		t.Fatalf("unborn repository: %+v", snapshot)
	}
	if _, err := repo.Stage(repoFile(t, snapshot, path)); err != nil {
		t.Fatal(err)
	}
	repoWrite(t, repo, path, "edited after staging\n")
	snapshot = repoSnapshot(t, repo, path, true)
	if !snapshot.SelectedStaged || !strings.Contains(snapshot.Diff, "+staged") {
		t.Fatalf("initial staged diff: %+v", snapshot)
	}
	if _, err := repo.Unstage(repoFile(t, snapshot, path)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(repo.directory, path))
	if string(data) != "edited after staging\n" {
		t.Fatalf("lost edited file: %q", data)
	}
	repoCommand(t, repo, "add", "--all")
	if _, err := repo.UnstageAll(); err != nil {
		t.Fatal(err)
	}
	if got := repoCommand(t, repo, "ls-files"); got != "" {
		t.Fatalf("unborn index still populated: %q", got)
	}
	repoCommand(t, repo, "add", "--all")
	if out, err := repo.Commit("First commit"); err != nil {
		t.Fatalf("first commit: %v %s", err, out)
	}
	if snapshot = repoSnapshot(t, repo, "", false); !snapshot.HasHEAD || len(snapshot.Files) != 0 {
		t.Fatalf("first commit state: %+v", snapshot)
	}
}

func TestRepositoryRenameStageAndUnstageIncludeCorrectPaths(t *testing.T) {
	repo := testRepository(t)
	old, renamed := "old file.txt", "new\nfile.txt"
	repoWrite(t, repo, old, "base\n")
	repoCommand(t, repo, "add", "--all")
	repoCommand(t, repo, "commit", "-qm", "Initial")
	if err := os.Rename(filepath.Join(repo.directory, old), filepath.Join(repo.directory, renamed)); err != nil {
		t.Fatal(err)
	}
	repoCommand(t, repo, "add", "--all")
	snapshot := repoSnapshot(t, repo, renamed, true)
	file := repoFile(t, snapshot, renamed)
	if file.Status != "R " || file.OriginalPath != old {
		t.Fatalf("rename parse: %+v", file)
	}
	repoWrite(t, repo, renamed, "modified rename\n")
	snapshot = repoSnapshot(t, repo, renamed, false)
	file = repoFile(t, snapshot, renamed)
	if _, err := repo.Stage(file); err != nil {
		t.Fatal("stage edit of indexed rename:", err)
	}
	// Large edits may change Git's rename detection to an add/delete pair.
	// Unstage the original rename entry to restore both paths in the index.
	if _, err := repo.Unstage(GitFile{Status: "R ", Path: renamed, OriginalPath: old}); err != nil {
		t.Fatal(err)
	}
	if got := repoCommand(t, repo, "diff", "--cached"); got != "" {
		t.Fatalf("left old path staged: %s", got)
	}
	data, _ := os.ReadFile(filepath.Join(repo.directory, renamed))
	if string(data) != "modified rename\n" {
		t.Fatalf("unstage changed renamed file: %q", data)
	}
	if _, err := os.Stat(filepath.Join(repo.directory, old)); !os.IsNotExist(err) {
		t.Fatal("unstage recreated working old path")
	}
	repoCommand(t, repo, "checkout", "--detach", "-q")
	if branch := repoSnapshot(t, repo, "", false).Branch; !strings.HasPrefix(branch, "HEAD · ") {
		t.Fatalf("detached label: %q", branch)
	}
}

func TestRepositoryConflictMustBeResolvedAndStaged(t *testing.T) {
	repo := testRepository(t)
	repoWrite(t, repo, "conflict.txt", "base\n")
	repoCommand(t, repo, "add", "--all")
	repoCommand(t, repo, "commit", "-qm", "Initial")
	repoCommand(t, repo, "checkout", "-qb", "side")
	repoWrite(t, repo, "conflict.txt", "side\n")
	repoCommand(t, repo, "commit", "-qam", "Side")
	repoCommand(t, repo, "checkout", "-q", "main")
	repoWrite(t, repo, "conflict.txt", "main\n")
	repoCommand(t, repo, "commit", "-qam", "Main")
	if _, err := repo.Run("merge", "--no-edit", "side"); err == nil {
		t.Fatal("expected merge conflict")
	}
	snapshot := repoSnapshot(t, repo, "conflict.txt", false)
	file := repoFile(t, snapshot, "conflict.txt")
	if !file.Conflicted() || file.Staged() || !file.Unstaged() {
		t.Fatalf("conflict state: %+v", file)
	}
	repoWrite(t, repo, "other.txt", "keep staged work\n")
	repoCommand(t, repo, "add", "--", "other.txt")
	if _, err := repo.UnstageAll(); err != nil {
		t.Fatal(err)
	}
	if !repoFile(t, repoSnapshot(t, repo, "conflict.txt", false), "conflict.txt").Conflicted() {
		t.Fatal("bulk unstage cleared an unresolved conflict")
	}
	if _, err := repo.Commit("Unresolved"); err == nil || !strings.Contains(err.Error(), "conflicted") {
		t.Fatalf("commit with conflict: %v", err)
	}
	repoWrite(t, repo, "conflict.txt", "resolved\n")
	if _, err := repo.Stage(file); err != nil {
		t.Fatal(err)
	}
	if out, err := repo.Commit("Resolve conflict"); err != nil {
		t.Fatalf("resolved commit: %v %s", err, out)
	}
}

func TestRepositoryPreviewsBinaryAndSymlinkAndPreservesHookFailure(t *testing.T) {
	repo := testRepository(t)
	repoWrite(t, repo, "binary.bin", "a\x00b")
	if diff := repoSnapshot(t, repo, "binary.bin", false).Diff; diff != "Binary file: binary.bin" {
		t.Fatalf("binary preview: %q", diff)
	}
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("OUTSIDE_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(repo.directory, "link")); err != nil {
		t.Fatal(err)
	}
	if diff := repoSnapshot(t, repo, "link", false).Diff; strings.Contains(diff, "OUTSIDE_CONTENT") || !strings.Contains(diff, "Symlink → ") {
		t.Fatalf("symlink preview: %q", diff)
	}
	repoCommand(t, repo, "add", "--all")
	hooks := filepath.Join(repo.directory, "hooks")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho hook-blocked >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := repo.Commit("Hook failure"); err == nil || !strings.Contains(out, "hook-blocked") {
		t.Fatalf("hook bypassed or hidden: %v %s", err, out)
	}
	if repo.HasHEAD() {
		t.Fatal("hook failure created a commit")
	}
	if got := repoCommand(t, repo, "ls-files"); got == "" {
		t.Fatal("hook failure discarded index")
	}
}
