package mterm

// Cell is a single cell in the terminal
type Cell struct {
	Char rune
	nl   bool // new: 2023-12-13 is new line
	SGRState
}

type Grid struct {
	cells       []Cell
	backlogSize int
	size        [2]int
	cursor      [2]int
}

func (g *Grid) Size() (rows, cols int) {
	return g.size[0], g.size[1]
}

// Resize regular resize without reflow, it will chomp any extra lines/columns
func (g *Grid) Resize(rows, cols int) {
	maxRows, maxCols := g.size[0], g.size[1]
	newCells := make([]Cell, rows*cols)
	for i := 0; i < maxRows && i < rows; i++ {
		tOff := i * cols
		sOff := i * g.size[1]
		if tOff < 0 || sOff < 0 {
			break
		}
		targetLine := newCells[tOff : tOff+cols]
		sourceLine := g.cells[sOff : sOff+(maxCols-1)]
		copy(targetLine, sourceLine)
	}
	g.size = [2]int{rows, cols}
	g.cursor = [2]int{
		clamp(g.cursor[0], 0, rows-1),
		clamp(g.cursor[1], 0, cols-1),
	}
	g.cells = newCells
}

// ResizeAndReflow resize the grid and reflow based on newline markers
func (g *Grid) ResizeAndReflow(rows, cols int) {
	maxRows, maxCols := g.size[0], g.size[1]

	curRowsSize := len(g.cells) / maxCols

	backRows := max(len(g.cells)/maxCols-maxRows, 0)
	newCells := make([]Cell, max(curRowsSize, rows)*cols)

	ni := 0
	shrink := 0
	addLine := func(line []Cell) {
		orows := 1 + (len(line)-1)/maxCols
		nrows := 1 + (len(line)-1)/cols
		rowsDiff := nrows - orows

		switch {
		case rowsDiff > 0:
			newCells = grow(newCells, rowsDiff*cols)
		case rowsDiff < 0:
			shrink += -rowsDiff
		}

		if ni > len(newCells) {
			return
		}
		n := copy(newCells[ni:], line)
		ni += n
		if len(line)%cols != 0 {
			ni += (cols - (ni % cols))
		}
	}

	start := 0
	// up to Cursor perhaps instead of full screen
	for i := start; i < len(g.cells); {
		c := g.cells[i]
		if !c.nl {
			i++
			continue
		}
		// add logical text line
		addLine(g.cells[start : i+1])
		// next line
		start = i + maxCols - (i % maxCols)
		i = start
	}
	copy(newCells[ni:], g.cells[start:])

	cursorLine := g.cursor[0]
	// 'scroll' up and move cursor up if needed
	if shrink > 0 {
		end := max(len(newCells)-shrink*cols, rows*cols)
		newCells = newCells[:end]

		mv := max(shrink-backRows, 0)
		cursorLine -= mv
	}
	switch {
	// if new screen is smaller, we shrink if the cursor is at bottom
	// it will scroll up
	case rows < maxRows:
		shrunk := maxRows - rows
		c := max(cursorLine-(rows-1), 0)
		shrunk = max(shrunk-c, 0)
		end := max(len(newCells)-shrunk*cols, rows*cols)
		newCells = newCells[:end]
	// if new screen is bigger and we have a backlog, move the cursor until
	// we don't have any history left
	case rows > maxRows:
		grown := rows - maxRows
		c := min(backRows, grown)
		cursorLine += c
	}
	g.cells = newCells
	g.size = [2]int{rows, cols}
	g.cursor = [2]int{
		cursorLine,
		clamp(g.cursor[1], 0, cols-1),
	}
}
