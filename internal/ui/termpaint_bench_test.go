package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	uv "github.com/charmbracelet/ultraviolet"
)

// Keep one renderer alive while every row changes, as it does during scrolling.
// Recreating a screenshot window for each frame hides retained GPU resources.
func BenchmarkTerminalScrolling(b *testing.B) {
	install, err := prepareAppFonts(runtime.GOOS, platformFontFiles())
	if err != nil {
		b.Fatal(err)
	}
	if err := install(); err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"ASCII", "ANSI", "CJK"} {
		b.Run(name, func(b *testing.B) {
			const cols, rows = 127, 31
			const scale float32 = 2
			a := &app{prefs: defaultPreferences()}
			term := &terminal{owner: a, cols: cols, rows: rows, cells: make([]uv.Cell, cols*rows), rowVersions: make([]uint64, rows)}
			defer term.glyphRenderer.Release()
			size := image.Pt(1912, 992)
			win, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				b.Fatal(err)
			}
			defer win.Release()
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}}
			b.ReportAllocs()
			b.ResetTimer()
			for frame := 0; frame < b.N; frame++ {
				for y := range rows {
					// Reproducible changing text without allocating fixture strings.
					n := uint32(frame*rows+y+1) * 2654435761
					for x := 0; x < cols; x++ {
						n ^= n << 13
						n ^= n >> 17
						n ^= n << 5
						cell := uv.Cell{Content: string(rune('a' + n%26)), Width: 1}
						if name == "ANSI" {
							cell.Style.Fg = color.NRGBA{R: uint8(x/8) * 17, G: 100, B: 180, A: 255}
							if x/16%2 == 1 {
								cell.Style.Attrs = uv.AttrBold
							}
						}
						if name == "CJK" && x+1 < cols && x%4 == 0 {
							cell.Content = string([]rune("中文终端测试输出")[n%8])
							cell.Width = 2
							term.cells[y*cols+x] = cell
							x++
							cell = uv.Cell{}
						}
						term.cells[y*cols+x] = cell
					}
					term.rowVersions[y]++
				}
				ops.Reset()
				gtx.Now = time.Now()
				rowPainter{term}.Layout(gtx)
				if err := win.Frame(&ops); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			runtime.GC()
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			b.ReportMetric(float64(memory.HeapAlloc)/(1<<20), "live-heap-MiB")
			// Optional artifacts for before/after memory and visual checks.
			if dir := os.Getenv("REX_RENDER_PROFILE_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					b.Fatal(err)
				}
				img := image.NewRGBA(image.Rectangle{Max: size})
				if err := win.Screenshot(img); err != nil {
					b.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, name+".png"))
				if err != nil {
					b.Fatal(err)
				}
				encodeErr := png.Encode(f, img)
				closeErr := f.Close()
				if encodeErr != nil || closeErr != nil {
					b.Fatalf("save screenshot: %v, %v", encodeErr, closeErr)
				}
				if runtime.GOOS == "darwin" {
					data, err := exec.Command("vmmap", "-summary", fmt.Sprint(os.Getpid())).CombinedOutput()
					if err != nil {
						b.Fatalf("vmmap: %v: %s", err, data)
					}
					if err := os.WriteFile(filepath.Join(dir, strings.ToLower(name)+"-memory.txt"), data, 0644); err != nil {
						b.Fatal(err)
					}
				}
			}
			runtime.KeepAlive(term)
		})
	}
}
