package ui

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/dyike/keel/gio/font"
	"github.com/dyike/keel/gio/text"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"golang.org/x/image/math/fixed"
)

func TestDarwinChineseFontLocations(t *testing.T) {
	for _, location := range []string{
		"Fonts/PingFang.ttc",
		"Fonts/Supplemental/PingFang.ttc",
		"AssetsV2/com_apple_MobileAsset_Font7/a.asset/AssetData/PingFang.ttc",
		"Assets/com_apple_MobileAsset_Font3/b.asset/AssetData/PingFang.ttc",
		"Fonts/Hiragino Sans GB.ttc",
		"Fonts/STHeiti Medium.ttc",
	} {
		t.Run(location, func(t *testing.T) {
			library := t.TempDir()
			path := filepath.Join(library, location)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if files := darwinFontFiles(library); !slices.Contains(files, path) {
				t.Fatalf("system Chinese font at %s was not found: %v", location, files)
			}
		})
	}
}

func TestDarwinSymbolFontsAlongsideCJK(t *testing.T) {
	library := t.TempDir()
	for _, name := range []string{"PingFang.ttc", "Apple Symbols.ttf", "Apple Braille.ttf"} {
		path := filepath.Join(library, "Fonts", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	files := darwinFontFiles(library)
	for _, name := range []string{"Apple Symbols.ttf", "Apple Braille.ttf"} {
		if !slices.Contains(files, filepath.Join(library, "Fonts", name)) {
			t.Fatalf("symbol font %s missing from %v", name, files)
		}
	}
}

func TestSystemFontFallback(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("uses installed macOS fonts")
	}
	// The installer changes process-wide font state; isolate it from the other
	// rendering tests, just as the unsupported Windows bitmap fixture is.
	if os.Getenv("REX_SYSTEM_FONT_HELPER") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSystemFontFallback$", "-test.v")
		cmd.Env = append(os.Environ(), "REX_SYSTEM_FONT_HELPER=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("system font regression: %v\n%s", err, output)
		} else {
			t.Log(string(output))
		}
		return
	}
	files := platformFontFiles()
	install, err := prepareAppFonts("darwin", files)
	if err != nil {
		t.Fatal(err)
	}
	if err := install(); err != nil {
		t.Fatal(err)
	}
	t.Logf("system fonts: %v", files)
	// Explicitly loaded faces must keep their file identity. Otherwise the
	// platform renderer silently uses vector coverage even with CoreText on.
	for _, weight := range []font.Weight{font.Normal, font.Bold} {
		for _, sample := range []string{"Menlo", "中文终端", "⠋⠙⠹"} {
			theme.Material.Shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: terminalFontFace, Weight: weight}, PxPerEm: fixed.I(28), MaxWidth: 10000}, sample)
			for g, ok := theme.Material.Shaper.NextGlyph(); ok; g, ok = theme.Material.Shaper.NextGlyph() {
				path, index, _, _, _, known := theme.Material.Shaper.GlyphFileInstance(g.ID)
				if !known || path == "" || index < 0 {
					t.Fatalf("font source lost: text=%q weight=%v path=%q index=%d", sample, weight, path, index)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("font source cannot be opened: %v", err)
				}
				if sample == "Menlo" && !slices.Contains(files, path) {
					t.Fatalf("explicit terminal font was not used: %q", path)
				}
				if sample == "中文终端" && !slices.Contains(files, path) {
					t.Fatalf("loaded CJK family was bypassed: %q", path)
				}
			}
		}
	}
	// Cover CJK text and npm/TUI spinners. Check both individual characters
	// and mixed runs: a .notdef box has ink, so pixel checks alone miss it.
	samples := []string{
		"⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⠁⠂⠄⡀⢀⠠⠐⠈◐◓◑◒◴◷◶◵▁▂▃▄▅▆▇█░▒▓",
		"证据明显优势窗口尺寸峰值完成时间为啥中文",
		"如果指 CPU 和内存，目前没有证据说明 keel-rex 比 Alacritty 有优势。",
		"后台会话持续运行，关闭窗口后可以接回。",
		"要判断性能优势，需要同机、同窗口尺寸、同字体、同历史行数。",
	}
	for _, sample := range samples {
		for _, family := range []font.Typeface{uiFontFace, terminalFontFace} {
			for _, weight := range []font.Weight{font.Normal, font.Bold} {
				check := func(s string) {
					t.Helper()
					theme.Material.Shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: family, Weight: weight}, PxPerEm: fixed.I(28), MaxWidth: 10000}, s)
					for g, ok := theme.Material.Shaper.NextGlyph(); ok; g, ok = theme.Material.Shaper.NextGlyph() {
						// Gio's packed ID keeps the OpenType glyph ID in its low
						// 32 bits. Glyph 0 is .notdef, the missing-character box.
						if uint32(g.ID) == 0 {
							t.Fatalf("missing glyph: text=%q, family=%s, weight=%v", s, family, weight)
						}
					}
				}
				for _, r := range sample {
					if r != ' ' {
						check(string(r))
					}
				}
				check(sample)
			}
		}
	}
	// Exercise the actual terminal row renderer with normal/bold text and both
	// appearances, so a successful font lookup cannot hide a drawing failure.
	for _, sample := range []struct {
		name, text string
		width      int
	}{
		{"cjk", "证据明显优势窗口尺寸峰值完成时间为啥中文", 2},
		{"spinner", "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", 1},
	} {
		a := &app{prefs: defaultPreferences()}
		a.prefs.FontSize = 20
		cw, ch := a.prefs.FontSize*.60208, a.prefs.FontSize*1.28
		term := &terminal{owner: a, cols: len([]rune(sample.text)) * sample.width, rows: 2, rowVersions: []uint64{1, 1}}
		term.cells = make([]uv.Cell, term.cols*term.rows)
		for row := range term.rows {
			for i, r := range []rune(sample.text) {
				cell := uv.Cell{Content: string(r), Width: sample.width}
				if row == 1 {
					cell.Style.Attrs = uv.AttrBold
				}
				term.cells[row*term.cols+sample.width*i] = cell
			}
		}
		for _, appearance := range []string{"dark", "light"} {
			a.prefs.Appearance = appearance
			for _, scale := range []float32{1, 2} {
				path := filepath.Join(t.TempDir(), fmt.Sprintf("system-%s-%s-%gx.png", sample.name, appearance, scale))
				w, h := int(math.Ceil(float64(float32(term.cols)*cw)))+2, int(math.Ceil(float64(2*ch)))
				if err := window.ScreenshotAtScale(codexRows{fullRowPainter{rowPainter{term}}}, w, h, scale, path); err != nil {
					t.Fatal(err)
				}
				img := readPNG(t, path)
				for row := range term.rows {
					for i, r := range []rune(sample.text) {
						pixels := 0
						for y := int(float32(row) * ch * scale); y < int(float32(row+1)*ch*scale); y++ {
							for x := int(float32(i*sample.width) * cw * scale); x < int(float32((i+1)*sample.width)*cw*scale); x++ {
								if img.At(x, y) != img.At(img.Bounds().Max.X-1, y) {
									pixels++
								}
							}
						}
						if pixels < 10 {
							t.Fatalf("%s %gx row %d: %c has only %d visible pixels", appearance, scale, row, r, pixels)
						}
					}
				}
				keep(t, path)
			}
		}
		term.glyphRenderer.Release()
	}
	if !strings.Contains(string(terminalFontFace), "Menlo") {
		t.Fatal("Latin terminal text lost its monospace system font")
	}
}
