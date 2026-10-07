package main

import (
	"fmt"
	"github.com/charmbracelet/x/term"
	"math/rand/v2"
	"os"
	"strings"
	"time"
)

type gridPoint struct{ x, y int }

func runSnake() {
	state, e := term.MakeRaw(os.Stdin.Fd())
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return
	}
	defer term.Restore(os.Stdin.Fd(), state)
	fmt.Print("\x1b[?1049h\x1b[?25l")
	defer fmt.Print("\x1b[?25h\x1b[?1049l")
	snake := []gridPoint{{13, 15}, {13, 14}, {13, 13}, {13, 12}, {13, 11}, {12, 11}, {11, 11}, {10, 11}, {9, 11}, {8, 11}, {8, 12}, {8, 13}, {7, 13}}
	food := []gridPoint{{1, 2}, {19, 5}, {9, 7}, {20, 8}, {23, 15}, {6, 21}, {27, 21}, {2, 22}, {15, 22}, {4, 23}, {24, 25}, {10, 26}}
	direction := gridPoint{0, 1}
	score := 43
	paused, over := true, false
	input := make(chan byte, 64)
	go func() {
		b := make([]byte, 1)
		for {
			if _, e := os.Stdin.Read(b); e != nil {
				return
			}
			input <- b[0]
		}
	}()
	tick := time.NewTicker(125 * time.Millisecond)
	defer tick.Stop()
	escape := 0
	draw := func() {
		var b strings.Builder
		b.WriteString("\x1b[H\x1b[2J\x1b[36m")
		fmt.Fprintf(&b, "S N A K E   Score: %d\r\n+%s+\r\n", score, strings.Repeat("-", 56))
		for y := 0; y < 28; y++ {
			b.WriteString("\x1b[36m|")
			for x := 0; x < 28; x++ {
				v := "  "
				for i, p := range snake {
					if p == (gridPoint{x, y}) {
						v = "\x1b[32m■ \x1b[36m"
						if i == 0 {
							v = "\x1b[32m◆ \x1b[36m"
						}
						break
					}
				}
				for _, p := range food {
					if p == (gridPoint{x, y}) {
						v = "\x1b[31m● \x1b[36m"
					}
				}
				b.WriteString(v)
			}
			b.WriteString("|\r\n")
		}
		fmt.Fprintf(&b, "+%s+\r\n\x1b[0m", strings.Repeat("-", 56))
		status := "Paused · Space resume · R restart · Ctrl+C quit"
		if !paused {
			status = "Playing · Space pause · R restart · Ctrl+C quit"
		}
		if over {
			status = "Game over · R restart · Ctrl+C quit"
		}
		b.WriteString(status)
		fmt.Print(b.String())
	}
	draw()
	for {
		select {
		case ch := <-input:
			if escape == 1 && ch == '[' {
				escape = 2
				continue
			}
			if escape == 2 {
				d := direction
				switch ch {
				case 'A':
					d = gridPoint{0, -1}
				case 'B':
					d = gridPoint{0, 1}
				case 'C':
					d = gridPoint{1, 0}
				case 'D':
					d = gridPoint{-1, 0}
				}
				if d.x != -direction.x || d.y != -direction.y {
					direction = d
				}
				escape = 0
				continue
			}
			switch ch {
			case 3, 4, 'q':
				return
			case 27:
				escape = 1
			case ' ':
				if !over {
					paused = !paused
				}
			case 'r', 'R':
				snake = []gridPoint{{13, 15}, {13, 14}, {13, 13}}
				score = 0
				paused = true
				over = false
				direction = gridPoint{0, 1}
			}
			draw()
		case <-tick.C:
			if paused {
				continue
			}
			head := snake[0]
			head.x += direction.x
			head.y += direction.y
			if head.x < 0 || head.x >= 28 || head.y < 0 || head.y >= 28 {
				over = true
				paused = true
				draw()
				continue
			}
			for _, p := range snake[:len(snake)-1] {
				if p == head {
					over = true
					paused = true
				}
			}
			if over {
				draw()
				continue
			}
			snake = append([]gridPoint{head}, snake...)
			eat := -1
			for i, p := range food {
				if p == head {
					eat = i
				}
			}
			if eat < 0 {
				snake = snake[:len(snake)-1]
			} else {
				score++
				food[eat] = gridPoint{rand.IntN(28), rand.IntN(28)}
			}
			draw()
		}
	}
}
