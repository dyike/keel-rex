// Memprobe measures where keel-rex's memory goes: the fonts internal/ui/run.go loads,
// the alternatives, and one session's VT scrollback. It prints JSON for
// tools/bench/report.py.
//
//	go run ./tools/bench/memprobe > tools/bench/memprobe.json
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	giofont "gioui.org/font"
	gioot "gioui.org/font/opentype"
	"gioui.org/text"
	"github.com/charmbracelet/x/vt"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/math/fixed"
)

type probe struct {
	Name   string  `json:"name"`
	Faces  int     `json:"faces,omitempty"`
	HeapMB float64 `json:"heap_mb"`
}

type face struct{ f *font.Font }

func (x face) Face() *font.Face { return font.NewFace(x.f) }

func liveHeap() float64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / 1e6
}

func collection(path string) []giofont.FontFace {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fs, _ := gioot.ParseCollection(b)
	return fs
}

func pingFang() string {
	m, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*/*.asset/AssetData/PingFang.ttc")
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// fonts builds a shaper the way mode says and returns the live heap after
// shaping a Chinese line, so lazily loaded system fonts are counted.
func fonts(mode string) probe {
	base := liveHeap()
	faces := append(collection("/System/Library/Fonts/Menlo.ttc"), collection("/System/Library/Fonts/HelveticaNeue.ttc")...)
	switch mode {
	case "all":
		faces = append(faces, collection(pingFang())...)
	case "one":
		if b, err := os.ReadFile(pingFang()); err == nil {
			lds, _ := opentype.NewLoaders(bytes.NewReader(b))
			for _, ld := range lds {
				f, err := font.NewFont(ld)
				if err != nil {
					continue
				}
				if d := f.Describe(); d.Family == "PingFang SC" && d.Aspect.Weight == font.WeightNormal {
					faces = append(faces, giofont.FontFace{Font: gioot.DescriptionToFont(d), Face: face{f}})
					break
				}
			}
		}
	}
	opts := []text.ShaperOption{text.WithCollection(faces)}
	if mode != "system" {
		opts = append(opts, text.NoSystemFonts())
	}
	sh := text.NewShaper(opts...)
	sh.LayoutString(text.Parameters{Font: giofont.Font{Typeface: "Menlo"}, PxPerEm: fixed.I(16), MaxWidth: 1e6}, "终端渲染性能测试中文字符宽度对齐 hello 世界")
	for _, ok := sh.NextGlyph(); ok; _, ok = sh.NextGlyph() {
	}
	heap := liveHeap() - base
	runtime.KeepAlive(sh)
	runtime.KeepAlive(faces)
	return probe{Name: mode, Faces: len(faces), HeapMB: round(heap)}
}

// scrollback fills one emulator, as a keel-rex session holds it, past its
// history limit.
func scrollback(cols, lines int) probe {
	base := liveHeap()
	e := vt.NewEmulator(cols, 31)
	e.SetScrollbackSize(lines)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := e.Read(buf); err != nil {
				return
			}
		}
	}()
	line := []byte(strings.Repeat("x", cols-1) + "\r\n")
	for i := 0; i < lines+100; i++ {
		e.Write(line)
	}
	heap := liveHeap() - base
	runtime.KeepAlive(e)
	return probe{Name: "scrollback", HeapMB: round(heap)}
}

func round(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func main() {
	out := map[string]any{}
	var fs []probe
	for _, m := range []string{"all", "one", "system", "none"} {
		fs = append(fs, fonts(m))
	}
	out["fonts"] = fs
	var sb []map[string]any
	for _, n := range []int{1000, 3000, 10000} {
		p := scrollback(127, n)
		sb = append(sb, map[string]any{"lines": n, "cols": 127, "heap_mb": p.HeapMB, "bytes_per_cell": round(p.HeapMB * 1e6 / float64(n*127))})
	}
	out["scrollback"] = sb
	json.NewEncoder(os.Stdout).Encode(out)
}
