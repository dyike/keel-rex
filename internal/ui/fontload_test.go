package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gioui.org/font"
	"github.com/dyike/keel/ui/theme"
)

// Loading only PingFang SC's two faces keeps Chinese shaping and the heap small.
func TestPingFangFacesOnly(t *testing.T) {
	cjk, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc")
	if len(cjk) == 0 {
		t.Skip("no PingFang")
	}
	// Other UI tests may already have populated global shaping caches. Check
	// this font load's retained allocation rather than the whole test process.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	previousShaper := theme.Material.Shaper
	b, _ := os.ReadFile(cjk[0])
	var kept []font.Font
	err := theme.LoadFontsWhere(func(f font.Font) bool {
		ok := f.Typeface == "PingFang SC" && (f.Weight == font.Normal || f.Weight == font.SemiBold)
		if ok {
			kept = append(kept, f)
		}
		return ok
	}, b)
	if err != nil || len(kept) != 2 {
		t.Fatalf("kept %v, err %v", kept, err)
	}
	b = nil
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	runtime.KeepAlive(previousShaper)
	retained := int64(m.HeapAlloc) - int64(before.HeapAlloc)
	if retained > 120<<20 {
		t.Fatalf("retained %d MB after loading two faces", retained>>20)
	}
	t.Logf("kept %v, retained %d MB", kept, retained>>20)
}
