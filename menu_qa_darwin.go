//go:build darwin && keelnativeqa

package main

import (
	"encoding/json"
	"fmt"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/window"
	"golang.org/x/image/draw"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func init() {
	for _, arg := range os.Args[1:] {
		if arg == "-server" {
			return
		}
	}
	if os.Getenv("REX_MENU_QA") != "1" {
		return
	}
	if os.Getenv("KEEL_REX_DIR") == "" {
		panic("native menu QA requires an isolated KEEL_REX_DIR")
	}
	dir := os.Getenv("REX_MENU_QA_DIR")
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "keel-rex-menu-qa-")
		if err != nil {
			panic(err)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		panic(err)
	}
	fmt.Println("Native menu QA artifacts:", dir)
	go func() {
		time.Sleep(12 * time.Second)
		stack := make([]byte, 1<<20)
		n := runtime.Stack(stack, true)
		os.WriteFile(filepath.Join(dir, "timeout.txt"), stack[:n], 0600)
		os.Exit(2)
	}()
	go func() {
		time.Sleep(2 * time.Second)
		if os.Getenv("REX_ICON_QA") == "1" {
			output := filepath.Join(dir, "dock-icon.png")
			referencePath := filepath.Join(dir, "configured-icon-appkit.png")
			if err := window.NativeQACaptureIcon(output, os.Getenv("KEEL_RUN_ICON"), referencePath); err != nil {
				panic(err)
			}
			expectedFile, err := os.Open(referencePath)
			if err != nil {
				panic(err)
			}
			expected, err := png.Decode(expectedFile)
			expectedFile.Close()
			if err != nil {
				panic(err)
			}
			actualFile, err := os.Open(output)
			if err != nil {
				panic(err)
			}
			actual, err := png.Decode(actualFile)
			actualFile.Close()
			if err != nil {
				panic(err)
			}
			if actual.Bounds() != expected.Bounds() {
				if actual.Bounds().Dx() != expected.Bounds().Dx()*2 || actual.Bounds().Dy() != expected.Bounds().Dy()*2 {
					panic("AppKit has the wrong icon size")
				}
				// AppKit may return a Retina TIFF representation.
				resized := image.NewNRGBA(expected.Bounds())
				draw.CatmullRom.Scale(resized, resized.Bounds(), actual, actual.Bounds(), draw.Src, nil)
				actual = resized
			}
			var difference uint64
			for y := 0; y < actual.Bounds().Dy(); y++ {
				for x := 0; x < actual.Bounds().Dx(); x++ {
					r, g, b, a := actual.At(x, y).RGBA()
					er, eg, eb, ea := expected.At(x, y).RGBA()
					for _, pair := range [][2]uint32{{r, er}, {g, eg}, {b, eb}, {a, ea}} {
						if pair[0] > pair[1] {
							difference += uint64(pair[0] - pair[1])
						} else {
							difference += uint64(pair[1] - pair[0])
						}
					}
				}
			}
			average := float64(difference) / float64(actual.Bounds().Dx()*actual.Bounds().Dy()*4*65535)
			if average > 0.002 {
				panic(fmt.Sprintf("AppKit icon differs from configured artwork: %.5f", average))
			}
			fmt.Printf("Native Dock icon matches keel.json: %dx%d, difference %.6f\n", actual.Bounds().Dx(), actual.Bounds().Dy(), average)
		}
		for _, command := range []string{"new-tab", "split-right", "split-down", "palette"} {
			if command == "palette" && os.Getenv("REX_IME_QA") == "1" {
				window.NativeQACompose(false)
				time.Sleep(500 * time.Millisecond)
				check := make(chan bool, 1)
				core.Update(func() {
					check <- !strings.Contains(applicationApp.focused.term.session.plain(), "imepreedit") && applicationApp.focused.term.ime.composing
				})
				if !<-check {
					panic("native preedit was sent to PTY or not retained")
				}
				window.NativeQACompose(true)
				time.Sleep(500 * time.Millisecond)
				core.Update(func() {
					check <- strings.Contains(applicationApp.focused.term.session.plain(), "你好中文") && !applicationApp.focused.term.ime.composing
				})
				if !<-check {
					panic("native Chinese commit missing from PTY")
				}
				fmt.Println("Native IME preedit and Chinese commit passed")
			}
			window.NativeQAActivateMenu(command)
			time.Sleep(500 * time.Millisecond)
		}
		core.Update(func() {
			a := applicationApp
			count := 0
			a.tabs[a.active].root.each(func(*pane) { count++ })
			b, _ := json.Marshal(map[string]any{"tabs": len(a.tabs), "panes": count, "palette": a.palette})
			os.WriteFile(filepath.Join(dir, "result.json"), b, 0600)
			if len(a.tabs) != 2 || count != 3 || !a.palette {
				panic("native menu actions did not complete")
			}
			a.endAll()
		})
	}()
}
