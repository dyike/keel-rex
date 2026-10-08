package main

import (
	"os/exec"
	"strings"
	"testing"
)

// Keep the headless service independently buildable as the UI evolves.
func TestSessionServerHasNoUIDependencies(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "./cmd/rex-server").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect server dependencies: %v\n%s", err, output)
	}
	for dependency := range strings.FieldsSeq(string(output)) {
		if strings.HasPrefix(dependency, "gioui.org") || strings.HasPrefix(dependency, "github.com/dyike/keel/ui") || dependency == "github.com/dyike/keel-rex/internal/ui" || dependency == "github.com/dyike/keel-rex/assets" {
			t.Fatalf("session server links UI dependency %s", dependency)
		}
	}
}
