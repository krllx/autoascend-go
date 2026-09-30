package strategy

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of soko_solver/__init__.py and GlobalLogic.solve_sokoban_strategy.

const (
	Soko_IGNORE  = 0
	Soko_EMPTY   = 1
	Soko_WALL    = 2
	Soko_BOULDER = 3
	Soko_TARGET  = 4
)

// ---- minimal 2-D numpy bool arrays with slicing and broadcasting

type npB2 struct {
	h, w int
	d    []bool
}

func newNpB2(h, w int) npB2 { return npB2{h: h, w: w, d: make([]bool, h*w)} }

// get reads element (i, j) of the broadcast view with result shape (>= own shape).
func (m npB2) get(i, j int) bool {
	if m.h == 1 {
		i = 0
	}
	if m.w == 1 {
		j = 0
	}
	return m.d[i*m.w+j]
}

func (m npB2) all() bool {
	for _, v := range m.d {
		if !v {
			return false
		}
	}
	return true
}

func (m npB2) nonzero() []Pos {
	var r []Pos
	for i := 0; i < m.h; i++ {
		for j := 0; j < m.w; j++ {
			if m.d[i*m.w+j] {
				r = append(r, Pos{i, j})
			}
		}
	}
	return r
}

// npSliceBounds mirrors the normalization of a numpy basic slice start:stop (step 1) on a dim of size n.
func npSliceBounds(start, stop, n int) (int, int) {
	norm := func(i int) int {
		if i < 0 {
			i += n
			if i < 0 {
				i = 0
			}
		} else if i > n {
			i = n
		}
		return i
	}
	lo, hi := norm(start), norm(stop)
	if hi < lo {
		hi = lo
	}
	return lo, hi
}

// npSliceGrid mirrors g[y0:y1, x0:x1] (a copy; views are only read here).
func npSliceGrid(g *BoolGrid, y0, y1, x0, x1 int) npB2 {
	ylo, yhi := npSliceBounds(y0, y1, SY)
	xlo, xhi := npSliceBounds(x0, x1, SX)
	r := newNpB2(yhi-ylo, xhi-xlo)
	for i := 0; i < r.h; i++ {
		for j := 0; j < r.w; j++ {
			r.d[i*r.w+j] = g[ylo+i][xlo+j]
		}
	}
	return r
}

func npBroadcastDim(a, b int) int {
	if a == b {
		return a
	}
	if a == 1 {
		return b
	}
	if b == 1 {
		return a
	}
	Raise("ValueError", "operands could not be broadcast together")
	return 0
}

// npBin mirrors a numpy elementwise binary op on two bool arrays with broadcasting.
func npBin(a, b npB2, op func(x, y bool) bool) npB2 {
	h := npBroadcastDim(a.h, b.h)
	w := npBroadcastDim(a.w, b.w)
	r := newNpB2(h, w)
	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			r.d[i*w+j] = op(a.get(i, j), b.get(i, j))
		}
	}
	return r
}

func npAnd(x, y bool) bool { return x && y }

func npOr(x, y bool) bool { return x || y }

func npEq(x, y bool) bool { return x == y }

func (m npB2) not() npB2 {
	r := newNpB2(m.h, m.w)
	for i, v := range m.d {
		r.d[i] = !v
	}
	return r
}

// ---- SokoMap

type sokoMap struct {
	h, w int
	g    []int8
	pos  Pos
}

// sokoIndent is the source indentation of the map texts in soko_solver/maps.py: every line starts with 4
// spaces, but the solutions count columns from the character after them. The Python bot converts the texts
// with the indentation, so the first move of every solution fails an assert in SokoMap.move and the bot gives
// up the game. Not in the Python bot: the port drops these columns.
const sokoIndent = 4

// sokoConvertMap mirrors convert_map on the precomputed map data, without the indentation columns (returns
// a fresh copy).
func sokoConvertMap(md *SokoMapData) *sokoMap {
	if md.Err != "" {
		Raise(md.Err, "convert_map")
	}
	h := len(md.Grid)
	w := 0
	if h > 0 {
		w = len(md.Grid[0]) - sokoIndent
	}
	s := &sokoMap{h: h, w: w, g: make([]int8, h*w), pos: Pos{md.PosY, md.PosX - sokoIndent}}
	for i, row := range md.Grid {
		if len(row) != w+sokoIndent {
			Raise("ValueError", "inhomogeneous shape")
		}
		for _, v := range row[:sokoIndent] {
			Assert(v == Soko_IGNORE, "a Sokoban map line without the indentation")
		}
		copy(s.g[i*w:], row[sokoIndent:])
	}
	return s
}

func (s *sokoMap) at(y, x int) int8 { return s.g[PyIdx(y, s.h)*s.w+PyIdx(x, s.w)] }

func (s *sokoMap) set(y, x int, v int8) {
	s.g[PyIdx(y, s.h)*s.w+PyIdx(x, s.w)] = v
}

func (s *sokoMap) eq(v int8) npB2 {
	r := newNpB2(s.h, s.w)
	for i, c := range s.g {
		r.d[i] = c == v
	}
	return r
}

// bfs mirrors SokoMap.bfs (returns flat dis, row-major h x w).
func (s *sokoMap) bfs() []int32 {
	walkable := s.eq(Soko_EMPTY).d
	diag := make([]bool, s.h*s.w)
	return BFSFlat(s.h, s.w, s.pos.Y, s.pos.X, walkable, diag, false)
}

func (s *sokoMap) disAt(dis []int32, y, x int) int32 { return dis[PyIdx(y, s.h)*s.w+PyIdx(x, s.w)] }

// move mirrors SokoMap.move.
func (s *sokoMap) move(by, bx, dy, dx int) {
	Assert(s.at(by, bx) == Soko_BOULDER)
	Assert(s.at(by-dy, bx-dx) == Soko_EMPTY)
	Assert(s.disAt(s.bfs(), by-dy, bx-dx) != -1)
	v := s.at(by+dy, bx+dx)
	Assert(v == Soko_EMPTY || v == Soko_TARGET)

	s.pos = Pos{by, bx}
	s.set(by, bx, Soko_EMPTY)
	if s.at(by+dy, bx+dx) == Soko_EMPTY {
		s.set(by+dy, bx+dx, Soko_BOULDER)
	} else if s.at(by+dy, bx+dx) == Soko_TARGET {
	} else {
		Assert(false)
	}
}

// ---- solve_sokoban_strategy

func pyAt32(g *I32Grid, y, x int) int32 { return g[PyIdx(y, SY)][PyIdx(x, SX)] }

func pyAt16(g *I16Grid, y, x int) int { return int(g[PyIdx(y, SY)][PyIdx(x, SX)]) }
