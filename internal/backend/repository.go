package backend

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitFile struct{ Status, Path, OriginalPath string }

func (f GitFile) Conflicted() bool {
	switch f.Status {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}
func (f GitFile) Staged() bool {
	return len(f.Status) == 2 && !f.Conflicted() && f.Status[0] != ' ' && f.Status[0] != '?'
}
func (f GitFile) Unstaged() bool {
	return len(f.Status) == 2 && (f.Conflicted() || f.Status[1] != ' ')
}
func (f GitFile) paths() []string {
	if f.OriginalPath != "" && strings.Contains(f.Status, "R") {
		return []string{f.OriginalPath, f.Path}
	}
	return []string{f.Path}
}

type GitSnapshot struct {
	Root, Branch, Diff, Selected string
	SelectedStaged, HasHEAD      bool
	Files                        []GitFile
}

// Repository owns Git commands and file previews; presentation state stays in the view.
type Repository struct{ directory string }

func NewRepository(directory string) *Repository { return &Repository{directory: directory} }
func gitCommand(dir string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"--literal-pathspecs"}, args...)...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	return string(b), err
}
func (r *Repository) Run(args ...string) (string, error) { return gitCommand(r.directory, args...) }
func (r *Repository) HasHEAD() bool {
	_, err := r.Run("rev-parse", "--verify", "HEAD")
	return err == nil
}
func (r *Repository) Snapshot(selected string, staged bool) (GitSnapshot, error) {
	dir := r.directory
	root, e := gitCommand(dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return GitSnapshot{}, fmt.Errorf("This directory is not a Git repository.")
	}
	root = strings.TrimSuffix(root, "\n")
	repo := NewRepository(root)
	hasHEAD := repo.HasHEAD()
	branch, e := repo.Run("branch", "--show-current")
	if e != nil {
		return GitSnapshot{}, fmt.Errorf("read branch: %s: %w", strings.TrimSpace(branch), e)
	}
	if strings.TrimSpace(branch) == "" && hasHEAD {
		id, err := repo.Run("rev-parse", "--short", "HEAD")
		if err != nil {
			return GitSnapshot{}, err
		}
		branch = "HEAD · " + strings.TrimSpace(id)
	}
	status, e := repo.Run("status", "--porcelain=v1", "--untracked-files=all", "-z")
	if e != nil {
		return GitSnapshot{}, fmt.Errorf("read status: %s: %w", strings.TrimSpace(status), e)
	}
	items := strings.Split(status, "\x00")
	var files []GitFile
	for i := 0; i < len(items); i++ {
		v := items[i]
		if len(v) < 4 {
			continue
		}
		file := GitFile{Status: v[:2], Path: v[3:]}
		if strings.ContainsAny(file.Status, "RC") && !file.Conflicted() && i+1 < len(items) {
			i++
			file.OriginalPath = items[i]
		}
		files = append(files, file)
	}
	choice := func(samePath bool, index bool) *GitFile {
		for i := range files {
			f := &files[i]
			if (!samePath || f.Path == selected) && (index && f.Staged() || !index && f.Unstaged()) {
				return f
			}
		}
		return nil
	}
	file := choice(true, staged)
	if file == nil {
		staged = !staged
		file = choice(true, staged)
	}
	if file == nil {
		staged = false
		file = choice(false, staged)
	}
	if file == nil {
		staged = true
		file = choice(false, staged)
	}
	selected = ""
	diff := "Working tree clean"
	if file != nil {
		selected = file.Path
		if file.Status == "??" {
			b, err := readPreview(filepath.Join(root, selected))
			if err != nil {
				return GitSnapshot{}, fmt.Errorf("read untracked file: %w", err)
			}
			diff = "Untracked: " + selected + "\n" + string(b)
			if bytes.IndexByte(b, 0) >= 0 {
				diff = "Binary file: " + selected
			}
		} else {
			args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color"}
			if staged {
				args = append(args, "--cached")
			}
			args = append(append(args, "--"), file.paths()...)
			var err error
			diff, err = repo.Run(args...)
			if err != nil {
				return GitSnapshot{}, fmt.Errorf("read diff: %s: %w", strings.TrimSpace(diff), err)
			}
		}
	}

	return GitSnapshot{Root: root, Branch: strings.TrimSpace(branch), Diff: diff, Selected: selected, SelectedStaged: staged, HasHEAD: hasHEAD, Files: files}, nil
}

func (r *Repository) Stage(file GitFile) (string, error) {
	paths := []string{file.Path}
	if len(file.Status) == 2 && file.Status[1] == 'R' {
		paths = file.paths()
	}
	return r.Run(append([]string{"add", "--"}, paths...)...)
}
func (r *Repository) Unstage(file GitFile) (string, error) {
	args := []string{"reset", "--"}
	if !r.HasHEAD() {
		// Only remove the index entry; retain working files even if edited
		// again after staging, before the repository's first commit.
		args = []string{"rm", "--cached", "-f", "--"}
	}
	return r.Run(append(args, file.paths()...)...)
}
func (r *Repository) UnstageAll() (string, error) {
	// Reset only ordinary index entries. Resetting '.' would also clear
	// unmerged entries and hide unresolved conflicts from the next snapshot.
	names, err := r.Run("diff", "--cached", "--name-only", "--no-renames", "--diff-filter=ACDMRT", "-z")
	if err != nil {
		return names, err
	}
	if names == "" {
		return "", fmt.Errorf("No staged changes")
	}
	paths := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")
	args := []string{"reset", "--"}
	if !r.HasHEAD() {
		args = []string{"rm", "--cached", "-f", "--"}
	}
	return r.Run(append(args, paths...)...)
}
func (r *Repository) Commit(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("Enter a commit message")
	}
	conflicts, err := r.Run("diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return conflicts, err
	}
	if conflicts != "" {
		return "", fmt.Errorf("Resolve and stage conflicted files before committing")
	}
	out, err := r.Run("diff", "--cached", "--quiet")
	if err == nil {
		return "", fmt.Errorf("Stage changes before committing")
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		return out, err
	}
	return r.Run("commit", "-m", message)
}
func readPreview(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		return []byte("Symlink → " + link), err
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 256<<10))
}
