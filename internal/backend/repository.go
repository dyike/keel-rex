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

type GitFile struct{ Status, Path string }
type GitSnapshot struct {
	Root, Branch, Diff, Selected string
	Files                        []GitFile
}

// Repository owns Git commands and file previews; presentation state stays in the view.
type Repository struct{ directory string }

func NewRepository(directory string) *Repository { return &Repository{directory: directory} }
func gitCommand(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	return string(b), err
}
func (r *Repository) Run(args ...string) (string, error) { return gitCommand(r.directory, args...) }
func (r *Repository) HasHEAD() bool {
	_, err := r.Run("rev-parse", "--verify", "HEAD")
	return err == nil
}
func (r *Repository) Snapshot(selected string) (GitSnapshot, error) {
	dir := r.directory
	root, e := gitCommand(dir, "rev-parse", "--show-toplevel")
	if e != nil {
		return GitSnapshot{}, fmt.Errorf("This directory is not a Git repository.")
	}
	root = strings.TrimSpace(root)
	branch, _ := gitCommand(root, "branch", "--show-current")
	status, _ := gitCommand(root, "status", "--porcelain=v1", "--untracked-files=all", "-z")
	items := strings.Split(status, "\x00")
	var files []GitFile
	for i := 0; i < len(items); i++ {
		v := items[i]
		if len(v) < 4 {
			continue
		}
		files = append(files, GitFile{v[:2], v[3:]})
		if v[0] == 'R' || v[0] == 'C' {
			i++
		}
	}
	found := false
	for _, f := range files {
		if f.Path == selected {
			found = true
		}
	}
	if !found {
		selected = ""
		if len(files) > 0 {
			selected = files[0].Path
		}
	}
	diff := "Working tree clean"
	if selected != "" {
		var err error
		diff, err = gitCommand(root, "diff", "--no-ext-diff", "--no-color", "HEAD", "--", selected)
		if err != nil {
			diff = ""
		}
		if diff == "" {
			diff, _ = gitCommand(root, "diff", "--no-ext-diff", "--no-color", "--", selected)
		}
		if diff == "" {
			diff, _ = gitCommand(root, "diff", "--cached", "--no-ext-diff", "--no-color", "--", selected)
		}
		if diff == "" {
			if b, e := readPreview(filepath.Join(root, selected)); e == nil {
				if len(b) > 256<<10 {
					b = b[:256<<10]
				}
				if bytes.IndexByte(b, 0) >= 0 {
					diff = "Binary file: " + selected
				} else {
					diff = "Untracked: " + selected + "\n" + string(b)
				}
			}
		}
	}

	return GitSnapshot{Root: root, Branch: strings.TrimSpace(branch), Diff: diff, Selected: selected, Files: files}, nil
}
func readPreview(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 256<<10))
}
