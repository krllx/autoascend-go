package tactics

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// ---- search_neighbors_for_traps(offset)

type searchNeighborsForTrapsN struct{ Offset int }

func (n searchNeighborsForTrapsN) Check(a *world.Agent) (bool, Frame) {
	offset := n.Offset
	searchCount := 0
	level := a.CurrentLevel()
	for _, p := range a.Neighbors(a.Blstats.Y, a.Blstats.X, false, true) {
		y, x := p.Y, p.X
		if level.WasOn[y][x] || G_TRAPS.Has(int(level.Objects[y][x])) {
			continue
		}
		c := 0
		for yy := max(y-1, 0); yy < min(y+2, SY); yy++ {
			for xx := max(x-1, 0); xx < min(x+2, SX); xx++ {
				c += int(level.SearchCount[yy][xx])
			}
		}
		if int(a.LastObservation.Specials[y][x])&NH_MG_OBJPILE == 0 {
			searchCount = max(searchCount, offset-c)
			continue
		}
		searchCount = max(searchCount, offset+4-c)
	}
	if searchCount == 0 {
		return false, nil
	}
	return true, &searchTimesF{N: searchCount}
}

// searchTimesF searches N times, one turn each.
type searchTimesF struct{ N, I int }

func (s *searchTimesF) Resume(c *Ctx) Cmd {
	if s.I < s.N {
		s.I++
		return call(skills.SearchTimes(1))
	}
	return ret(nil)
}

// ---- untrap_traps

type untrapTrapsN struct{}

var untrappableTraps = NewGSet(SS_S_web, SS_S_bear_trap, SS_S_land_mine, SS_S_dart_trap, SS_S_arrow_trap)

func (untrapTrapsN) Check(a *world.Agent) (bool, Frame) {
	e := a.Exploration
	if a.Blstats.Hitpoints < 10 || Div(float64(a.Blstats.Hitpoints), float64(a.Blstats.MaxHitpoints)) < 0.5 {
		// not enough HP to risk untrapping at all
		return false, nil
	}
	// TODO: consider strategy for SS.S_pit, SS.S_spiked_pit
	level := a.CurrentLevel()
	trapMask := Isin(&level.Objects, untrappableTraps)
	if !trapMask.Any() {
		return false, nil
	}
	var trapLocations []Pos
	for _, loc := range trapMask.Nonzero() {
		if e.WorthUntrapping(loc.Y, loc.X) {
			trapLocations = append(trapLocations, loc)
		}
	}
	if len(trapLocations) == 0 {
		return false, nil
	}
	trapMask = Dilate(&trapMask, 1, true)
	bfs := a.Bfs()
	trapMask[a.Blstats.Y][a.Blstats.X] = false // don't try to untrap when standing on it
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			trapMask[y][x] = trapMask[y][x] && bfs[y][x] >= 0
		}
	}
	if !trapMask.Any() {
		return false, nil
	}
	first := true
	var minB int32
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			if trapMask[y][x] && (first || bfs[y][x] < minB) {
				minB = bfs[y][x]
				first = false
			}
		}
	}
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			trapMask[y][x] = trapMask[y][x] && bfs[y][x] == minB
		}
	}
	Assert(trapMask.Any())
	if !trapMask.Any() {
		return false, nil
	}
	return true, &untrapTrapsF{Level: level, Mask: trapMask, Traps: trapLocations}
}

type untrapTrapsF struct {
	Level *world.Level
	Mask  BoolGrid
	Traps []Pos
	I     int
	PC    int // 0: go, 1: next trap, 2: untrapped, 3: terrain checked
}

func (s *untrapTrapsF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		t := s.Mask.Nonzero()[0]
		Assert(t.Y != a.Blstats.Y || t.X != a.Blstats.X)
		s.PC = 1
		return call(skills.GoTo(t.Y, t.X, skills.GoToOpts{}))
	case 2:
		s.PC = 3
		return call(sense.CheckTerrain(true))
	case 3:
		s.I++
	}
	for ; s.I < len(s.Traps); s.I++ {
		trapY, trapX := s.Traps[s.I].Y, s.Traps[s.I].X
		if !Adjacent(Pos{a.Blstats.Y, a.Blstats.X}, Pos{trapY, trapX}) {
			continue
		}
		trapGlyph := int(s.Level.Objects[trapY][trapX])
		if !untrappableTraps.Has(trapGlyph) {
			Panic("a trap is no longer there")
		}
		// assert level.objects[trap_y, trap_x] == trap_glyph
		if trapY == a.Blstats.Y && trapX == a.Blstats.X {
			continue
		}
		s.PC = 2
		return call(skills.Untrap(trapY, trapX))
	}
	return ret(nil)
}
