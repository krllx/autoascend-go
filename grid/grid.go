package grid

import (
	"unsafe"

	. "aago/internal/pyrt"
)

const (
	SY = 21 // C.SIZE_Y
	SX = 79 // C.SIZE_X
)

type Pos struct{ Y, X int }

type BoolGrid [SY][SX]bool
type I16Grid [SY][SX]int16
type I32Grid [SY][SX]int32
type F32Grid [SY][SX]float32
type F64Grid [SY][SX]float64

func InBounds(y, x int) bool { return y >= 0 && y < SY && x >= 0 && x < SX }

// ---- I32Grid ops (distance maps from BFS: -1 = unreachable)

// Eq, Ne, Gt, Le, Lt, Ge (numpy `g == v` etc.) are in vec.go; `dis.Ne(-1)` is the mask of reachable cells.

// MinWhere is numpy `g[mask].min()`. Raises ValueError when the mask is empty.
func (g *I32Grid) MinWhere(mask BoolGrid) int32 {
	var m int32
	found := false
	for y := range g {
		for x := range g[y] {
			if mask[y][x] && (!found || g[y][x] < m) {
				m, found = g[y][x], true
			}
		}
	}
	if !found {
		Raise("ValueError", "zero-size array to reduction operation minimum which has no identity")
	}
	return m
}

// ---- translate (utils.translate): out[y+dy][x+dx] = a[y][x], zeros elsewhere

func TranslateB(a *BoolGrid, dy, dx int) (out BoolGrid) {
	for y := 0; y < SY; y++ {
		ny := y + dy
		if ny < 0 || ny >= SY {
			continue
		}
		for x := 0; x < SX; x++ {
			nx := x + dx
			if nx >= 0 && nx < SX {
				out[ny][nx] = a[y][x]
			}
		}
	}
	return
}

func TranslateI16(a *I16Grid, dy, dx int) (out I16Grid) {
	for y := 0; y < SY; y++ {
		ny := y + dy
		if ny < 0 || ny >= SY {
			continue
		}
		for x := 0; x < SX; x++ {
			nx := x + dx
			if nx >= 0 && nx < SX {
				out[ny][nx] = a[y][x]
			}
		}
	}
	return
}

// ---- bfs (utils.bfs, numba kernel) on an arbitrary h x w shape, flat row-major slices

func BFSFlat(h, w, y0, x0 int, walkable, walkDiag []bool, canSqueeze bool) []int32 {
	dis := make([]int32, h*w)
	for i := range dis {
		dis[i] = -1
	}
	dis[y0*w+x0] = 0
	buf := make([]int32, 0, h*w*2)
	buf = append(buf, int32(y0), int32(x0))
	for idx := 0; idx < len(buf); idx += 2 {
		y, x := int(buf[idx]), int(buf[idx+1])
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				py, px := y+dy, x+dx
				if py >= 0 && py < h && px >= 0 && px < w && (dy != 0 || dx != 0) {
					if walkable[py*w+px] &&
						(Abs(dy)+Abs(dx) <= 1 ||
							(walkDiag[py*w+px] && walkDiag[y*w+x] &&
								(canSqueeze || walkable[py*w+x] || walkable[y*w+px]))) {
						if dis[py*w+px] == -1 {
							dis[py*w+px] = dis[y*w+x] + 1
							buf = append(buf, int32(py), int32(px))
						}
					}
				}
			}
		}
	}
	return dis
}

func flatB(g *BoolGrid) []bool { return unsafe.Slice(&g[0][0], SY*SX) }

const bfsW = SX + 2 // padded row width: a false border removes all bounds checks from the inner loop

// unreached is a padded BFS distance map with every cell -1 (read only).
var unreached = func() (d [(SY + 2) * bfsW]int32) {
	for i := range d {
		d[i] = -1
	}
	return
}()

// BFS mirrors utils.bfs on a 21x79 grid. Distances of a BFS do not depend on the neighbour order,
// so this padded, flat-indexed version returns exactly the numba kernel's result. The neighbours are
// written out (the helpers inline) and the queue holds int16 cell indices: 40% faster than looping over
// offset tables.
func BFS(y0, x0 int, walkable, walkDiag *BoolGrid, canSqueeze bool) (dis I32Grid) {
	var w, d [(SY + 2) * bfsW]bool
	var dist [(SY + 2) * bfsW]int32
	for y := 0; y < SY; y++ {
		copy(w[(y+1)*bfsW+1:(y+1)*bfsW+1+SX], walkable[y][:])
		copy(d[(y+1)*bfsW+1:(y+1)*bfsW+1+SX], walkDiag[y][:])
	}
	dist = unreached // a block copy; filling the cells one by one took a tenth of the BFS
	var queue [SY * SX]int16 // padded indices are < (SY+2)*bfsW
	start := (y0+1)*bfsW + x0 + 1
	dist[start] = 0
	queue[0] = int16(start)
	size := 1
	orth := func(j int, nd int32) {
		if w[j] && dist[j] == -1 {
			dist[j] = nd
			queue[size] = int16(j)
			size++
		}
	}
	for idx := 0; idx < size; idx++ {
		i := int(queue[idx])
		nd := dist[i] + 1
		orth(i-bfsW, nd)
		orth(i+bfsW, nd)
		orth(i-1, nd)
		orth(i+1, nd)
		if !d[i] {
			continue
		}
		// a diagonal step: both ends diagonal-walkable, and a free orthogonal corner unless squeezing is allowed
		diag := func(j, vert, horiz int) {
			if w[j] && dist[j] == -1 && d[j] && (canSqueeze || w[vert] || w[horiz]) {
				dist[j] = nd
				queue[size] = int16(j)
				size++
			}
		}
		diag(i-bfsW-1, i-bfsW, i-1)
		diag(i-bfsW+1, i-bfsW, i+1)
		diag(i+bfsW-1, i+bfsW, i-1)
		diag(i+bfsW+1, i+bfsW, i+1)
	}
	for y := 0; y < SY; y++ {
		copy(dis[y][:], dist[(y+1)*bfsW+1:(y+1)*bfsW+1+SX])
	}
	return
}

// Dilate mirrors utils.dilate (cv2.dilate, out-of-image pixels ignored).
func Dilate(mask *BoolGrid, radius int, withDiagonal bool) (out BoolGrid) {
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			if !mask[y][x] {
				continue
			}
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					if !withDiagonal && dy != 0 && dx != 0 {
						continue
					}
					ny, nx := y+dy, x+dx
					if InBounds(ny, nx) {
						out[ny][nx] = true
					}
				}
			}
		}
	}
	return
}

func Adjacent(p1, p2 Pos) bool {
	return max(Abs(p1.Y-p2.Y), Abs(p1.X-p2.X)) == 1
}

// CalcDPS mirrors utils.calc_dps.
func CalcDPS(toHit, damage float64) float64 {
	return damage * min(20, max(0, toHit-1)) / 20
}
