package strategy

import (
	"testing"

	. "aago/internal/pyrt"
	. "aago/nle"
)

// Every solution of the solver's table plays on its map: each push has a boulder to push, a free and reachable
// tile for the agent behind it and a free tile or the last hole in front of it, and in the end every boulder
// has gone down the last hole.
func TestSokoSolutionsPlay(t *testing.T) {
	if len(SokoMapsData) != 8 {
		t.Fatalf("%d Sokoban maps, want 8 (4 levels, 2 variants each)", len(SokoMapsData))
	}
	for i := range SokoMapsData {
		md := &SokoMapsData[i]
		sm := sokoConvertMap(md)
		if sm.at(sm.pos.Y, sm.pos.X) != Soko_EMPTY {
			t.Errorf("map %d: the start %v is not an empty tile", i, sm.pos)
		}
		for k, m := range md.Answer {
			if exc := Try(func() { sm.move(m[0], m[1], m[2], m[3]) }); exc != nil {
				t.Fatalf("map %d, move %d of %d %v: %v", i, k, len(md.Answer), m, exc)
			}
		}
		if n := len(sm.eq(Soko_BOULDER).nonzero()); n != 0 {
			t.Errorf("map %d: %d boulders left after the solution", i, n)
		}
	}
}

// The texts in maps.py are indented by 4 spaces the solutions do not count; the conversion drops them.
func TestSokoConvertDropsIndent(t *testing.T) {
	for i := range SokoMapsData {
		md := &SokoMapsData[i]
		sm := sokoConvertMap(md)
		if sm.w != len(md.Grid[0])-sokoIndent || sm.h != len(md.Grid) {
			t.Fatalf("map %d: %dx%d from %dx%d", i, sm.h, sm.w, len(md.Grid), len(md.Grid[0]))
		}
		for y := range sm.h {
			for x := range sm.w {
				if sm.at(y, x) != md.Grid[y][x+sokoIndent] {
					t.Fatalf("map %d: tile (%d, %d) moved", i, y, x)
				}
			}
		}
		// the solver's first push, on the raw table, starts on a tile without a boulder
		m := md.Answer[0]
		if md.Grid[m[0]][m[1]] == Soko_BOULDER {
			t.Errorf("map %d: the first move %v already fits the indented map", i, m)
		}
		if sm.at(m[0], m[1]) != Soko_BOULDER {
			t.Errorf("map %d: no boulder at the first move %v", i, m)
		}
	}
}
