package tactics

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	"aago/skills"
	"aago/world"
)

// ---- patrol

type patrolStepper struct{}

func (patrolStepper) Step(a *world.Agent) Frame {
	reachable := a.Bfs()
	allZero := true
	total := int64(0)
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			if reachable[y][x] < 0 {
				reachable[y][x] = 0
			}
			if reachable[y][x] != 0 {
				allZero = false
			}
			total += int64(reachable[y][x])
		}
	}
	if allZero {
		return nil
	}
	return &patrolGoF{Reachable: reachable, Total: total}
}

type patrolGoF struct {
	Reachable I32Grid
	Total     int64
	PC        int
}

func (s *patrolGoF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	a := c.W
	p := make([]float64, SY*SX)
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			p[y*SX+x] = float64(s.Reachable[y][x]) / float64(s.Total)
		}
	}
	i := a.Rng.ChoiceP(p)
	y, x := FloorDiv(i, SX), FloorMod(i, SX)
	s.PC = 1
	return call(skills.GoTo(y, x, skills.GoToOpts{Fast: true}))
}
