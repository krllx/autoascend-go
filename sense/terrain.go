package sense

import (
	. "aago/nle"
	"aago/world"
)

// checkTerrainF is check_terrain(force): every 50 turns (or when forced) read the #terrain view.
type checkTerrainF struct {
	Force bool
	PC    int
}

func CheckTerrain(force bool) Frame { return &checkTerrainF{Force: force} }

func (s *checkTerrainF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		if s.Force || a.LastTerrainCheck < 0 || a.Blstats.Time-a.LastTerrainCheck > 50 {
			a.LastTerrainCheck = a.Blstats.Time
			s.PC = 1
			return call(Atom(&terrainViewF{}))
		}
	}
	return ret(nil)
}

type terrainViewF struct{ PC int }

func (s *terrainViewF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(TypeText("#te"))
	case 1:
		s.PC = 2
		return call(a.Term.Keys(A_MiscAction_MORE, world.IterStr("b")))
	case 2:
		a.UpdateLevel()
		s.PC = 3
		return call(a.Term.Key(A_Command_ESC))
	}
	return ret(nil)
}
