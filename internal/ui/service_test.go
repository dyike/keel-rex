package ui

import (
	"encoding/json"
	"github.com/dyike/keel-rex/internal/backend"
	"testing"
	"time"
)

type workspaceServiceStub struct {
	backend.WorkspaceService
	model                     backend.Session
	openedID, openedDirectory string
	layout                    json.RawMessage
}

func (s *workspaceServiceStub) Open(id, dir string) (backend.Session, error) {
	s.openedID, s.openedDirectory = id, dir
	return s.model, nil
}
func (s *workspaceServiceStub) LoadLayout() (json.RawMessage, error) { return s.layout, nil }
func (s *workspaceServiceStub) SaveLayout(data json.RawMessage) error {
	s.layout = append(json.RawMessage(nil), data...)
	return nil
}

type observedModel struct {
	modelSession
	stopped      chan struct{}
	changed      chan struct{}
	unsubscribed chan struct{}
}

func (s *observedModel) Subscribe() chan struct{}  { return s.changed }
func (s *observedModel) Unsubscribe(chan struct{}) { close(s.unsubscribed) }
func (s *observedModel) Done() <-chan struct{}     { return s.stopped }

func TestPaneCreationAndPersistenceUseInjectedService(t *testing.T) {
	model := &observedModel{stopped: make(chan struct{}), changed: make(chan struct{}, 1), unsubscribed: make(chan struct{})}
	service := &workspaceServiceStub{model: model}
	a := &app{service: service, prefs: defaultPreferences()}
	p := newPane(a, "/project")
	if p.term == nil || p.term.session != model || service.openedID != "" || service.openedDirectory != "/project" {
		t.Fatal("pane bypassed the injected service")
	}
	// Layout writes pass through the service object, even without a socket.
	a.dataDir = t.TempDir()
	a.tabs = []*workspace{{root: p, focus: p, title: "Injected"}}
	a.persistQueue = make(chan persistJob, 1)
	go a.persistenceWriter()
	a.persist(true)
	var restored savedWorkspace
	if json.Unmarshal(service.layout, &restored) != nil || len(restored.Tabs) != 1 || restored.Tabs[0].Root.Session != "model" {
		t.Fatalf("layout not saved by service: %s", service.layout)
	}
	close(a.persistQueue)
	close(model.stopped)
	select {
	case <-model.unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("view retained its model observer after shutdown")
	}
}
