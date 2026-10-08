package backend

import (
	"os"
	"path/filepath"
)

func atomicJSON(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	temp, e := os.CreateTemp(filepath.Dir(path), ".state-*")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	if e = temp.Chmod(0600); e == nil {
		_, e = temp.Write(b)
	}
	if e == nil {
		e = temp.Sync()
	}
	closeErr := temp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}

// SettingsStore owns the preferences file; callers own its JSON schema.
type SettingsStore struct{ path string }

func NewSettingsStore(directory string) *SettingsStore {
	return &SettingsStore{path: filepath.Join(directory, "settings.json")}
}
func (s *SettingsStore) Load() ([]byte, error)  { return os.ReadFile(s.path) }
func (s *SettingsStore) Save(data []byte) error { return atomicJSON(s.path, data) }
