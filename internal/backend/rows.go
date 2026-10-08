package backend

import uv "github.com/charmbracelet/ultraviolet"

// blankRow is the version of a row the screen does not reach.
const blankRow = ^uint64(0)

// syncRows brings cells, the window's cols×rows grid, up to the screen at
// scroll, copying only the rows whose version changed, and returns the
// screen's state and whether a row changed. versions holds the version of
// each row in cells: a remote session's from its updates, a local one's
// counted here.
func (s *session) syncRows(scroll, cols, rows int, cells *[]uv.Cell, versions *[]uint64) (Frame, bool) {
	if len(*cells) != cols*rows || len(*versions) != rows {
		*cells = make([]uv.Cell, cols*rows)
		*versions = make([]uint64, rows)
	}
	grid, vers := *cells, *versions
	changed := false
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		f := s.frame
		for y := 0; y < rows; y++ {
			v, src := blankRow, []uv.Cell(nil)
			if y < f.Rows && y < len(s.rowVersions) && f.Cols > 0 && len(s.grid) >= (y+1)*f.Cols {
				v, src = s.rowVersions[y], s.grid[y*f.Cols:(y+1)*f.Cols]
			}
			if vers[y] == v {
				continue
			}
			dst := grid[y*cols : (y+1)*cols]
			clear(dst[copy(dst, src):])
			vers[y] = v
			changed = true
		}
		return f, changed
	}
	f := s.frameState(scroll)
	for y := 0; y < rows; y++ {
		dst := grid[y*cols : (y+1)*cols]
		rowChanged := vers[y] == 0
		for x := range dst {
			var c uv.Cell
			if x < f.Cols && y < f.Rows {
				c = encodeCell(s.cellAt(x, y, f)).cell()
			}
			if !dst[x].Equal(&c) {
				dst[x] = c
				rowChanged = true
			}
		}
		if rowChanged {
			s.localVersion++
			vers[y] = s.localVersion
			changed = true
		}
	}
	return f, changed
}
