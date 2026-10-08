package backend

import "testing"

func TestDevelopmentBundleKeepsSessionDirectory(t *testing.T) {
	for _, tc := range []struct {
		executable  string
		development bool
		want        string
	}{
		{"/Applications/Rex Keel.app/Contents/MacOS/rex-keel", false, "Rex Keel"},
		{"/tmp/keel-run-123/rex-keel-1.app/Contents/MacOS/rex-keel", true, "Rex Keel Dev"},
		{"/tmp/keel-run-123/rex-keel-1", false, "Rex Keel Dev"},
	} {
		if got := stateDirectoryName(tc.executable, tc.development); got != tc.want {
			t.Fatalf("%s (development=%t): got %s want %s", tc.executable, tc.development, got, tc.want)
		}
	}
}
