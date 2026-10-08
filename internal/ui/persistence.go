package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type savedPane struct {
	ID                 int        `json:"id"`
	Session, Dir, Kind string     `json:",omitempty"`
	Vertical           bool       `json:",omitempty"`
	Ratio              float32    `json:",omitempty"`
	First, Second      *savedPane `json:",omitempty"`
}
type savedTab struct {
	Title        string
	Custom, Zoom bool
	Focus        int
	Root         *savedPane
}
type savedWorkspace struct {
	Version, Active int
	Tabs            []savedTab
}

func snapshotPane(p *pane) *savedPane {
	if p == nil {
		return nil
	}
	out := &savedPane{ID: p.id, Vertical: p.vertical, Ratio: p.ratio, First: snapshotPane(p.first), Second: snapshotPane(p.second)}
	if p.term != nil {
		out.Kind = "terminal"
		out.Dir = p.term.session.Directory()
		out.Session = p.term.session.ID()
	}
	if p.git != nil {
		out.Kind = "git"
		out.Dir = p.git.dir
	}
	return out
}
func (a *app) workspaceState() savedWorkspace {
	state := savedWorkspace{Version: 1, Active: a.active}
	for _, t := range a.tabs {
		focus := 0
		if t.focus != nil {
			focus = t.focus.id
		}
		state.Tabs = append(state.Tabs, savedTab{t.title, t.customTitle, t.zoom, focus, snapshotPane(t.root)})
	}
	return state
}

type persistJob struct {
	Data []byte
	Done chan error
}

func (a *app) persist(wait bool) {
	if a.dataDir == "" || a.closed {
		return
	}
	b, e := json.Marshal(a.workspaceState())
	if e != nil {
		return
	}
	if !wait && string(b) == a.lastLayout {
		return
	}
	a.lastLayout = string(b)
	if wait {
		done := make(chan error, 1)
		a.persistQueue <- persistJob{b, done}
		if e := <-done; e != nil {
			a.notice = e.Error()
		}
		return
	}
	job := persistJob{Data: b}
	select {
	case a.persistQueue <- job:
	default:
		select {
		case <-a.persistQueue:
		default:
		}
		a.persistQueue <- job
	}
}
func (a *app) persistenceWriter() {
	for job := range a.persistQueue {
		e := a.backendService().SaveLayout(job.Data)
		if job.Done != nil {
			job.Done <- e
		}
	}
}

func (a *app) restoreWorkspace() bool {
	data, e := a.backendService().LoadLayout()
	if e != nil {
		return false
	}
	var saved savedWorkspace
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 || len(saved.Tabs) == 0 || len(saved.Tabs) > 100 {
		return false
	}
	for _, t := range saved.Tabs {
		tab := &workspace{title: t.Title, customTitle: t.Custom, zoom: t.Zoom}
		tab.root = a.restorePane(t.Root, 0)
		if tab.root == nil {
			continue
		}
		tab.root.each(func(p *pane) {
			if p.id == t.Focus {
				tab.focus = p
			}
		})
		a.tabs = append(a.tabs, tab)
	}
	if len(a.tabs) == 0 {
		return false
	}
	a.activate(max(0, min(saved.Active, len(a.tabs)-1)))
	return true
}
func (a *app) restorePane(saved *savedPane, depth int) *pane {
	if saved == nil || depth > 20 {
		return nil
	}
	dir := saved.Dir
	if st, e := os.Stat(dir); e != nil || !st.IsDir() {
		dir = a.cwd
	}
	p := &pane{id: saved.ID, vertical: saved.Vertical, ratio: max(.15, min(.85, saved.Ratio))}
	nextID = max(nextID, p.id)
	if saved.First != nil {
		p.first = a.restorePane(saved.First, depth+1)
		p.second = a.restorePane(saved.Second, depth+1)
		if p.first == nil {
			return p.second
		}
		if p.second == nil {
			return p.first
		}
		return p
	}
	if saved.Kind == "git" {
		p.git = &gitState{dir: dir}
		p.git.refresh()
		return p
	}
	s, e := a.backendService().Open(saved.Session, dir)
	if e != nil {
		s, e = a.backendService().Open("", dir)
	}
	if e != nil {
		p.err = e.Error()
	} else {
		p.term = a.terminalView(s, p, false)
	}
	return p
}
func (a *app) validateDirectory(path string) (string, error) {
	if path == "~" || len(path) > 1 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	path, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	stat, e := os.Stat(path)
	if e != nil {
		return "", e
	}
	if !stat.IsDir() {
		return "", &os.PathError{Op: "open directory", Path: path, Err: os.ErrInvalid}
	}
	return path, nil
}
