package skills

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	"aago/world"
)

type GoToOpts struct {
	StopOneBefore bool
	MaxSteps      int // 0 means None
	Callback      GoToCallback
	Fast          bool
}

// GoToCallback is go_to's callback: checked before the walk and after every move; true stops the walk.
// It must not act.
type GoToCallback interface{ Check(a *world.Agent) bool }

// goToF is go_to(y, x, stop_one_before, max_steps, callback, fast): walk along the shortest path,
// recomputing it when a peaceful monster or an unwalkable tile blocks the way.
type goToF struct {
	Y, X          int
	O             GoToOpts
	StopOneBefore bool
	StepsTaken    int
	Cont          bool
	Path          []Pos
	PI            int
	MyY, MyX      int
	PC            int // 0: start, 1: loop head, 2: fast travel done, 3: next path tile, 4: moved
}

func GoTo(y, x int, o GoToOpts) Frame { return &goToF{Y: y, X: x, O: o} }

func (s *goToF) callback(a *world.Agent) bool { return s.O.Callback != nil && s.O.Callback.Check(a) }

func (s *goToF) Resume(c *Ctx) Cmd {
	a := c.W
	for {
		switch s.PC {
		case 0:
			s.StopOneBefore = s.O.StopOneBefore
			Assert(!s.StopOneBefore || (a.Blstats.Y != s.Y || a.Blstats.X != s.X))
			Assert(s.O.MaxSteps == 0 || !s.O.Fast)
			if s.StopOneBefore {
				if d := a.Bfs(); d[s.Y][s.X] == -1 {
					dis := a.Bfs()
					var best *Pos
					for _, p := range a.Neighbors(s.Y, s.X, true, true) {
						if dis[p.Y][p.X] != -1 && (best == nil || dis[best.Y][best.X] > dis[p.Y][p.X]) {
							pp := p
							best = &pp
						}
					}
					Assert(best != nil, "no reachable neighbor")
					s.Y, s.X = best.Y, best.X
					s.StopOneBefore = false
				}
			}
			if d := a.Bfs(); true {
				Assert(d[s.Y][s.X] != -1)
			}
			if s.callback(a) {
				return ret(nil)
			}
			s.StepsTaken = 0
			s.Cont = true
			s.PC = 1
		case 1: // while cont and me != (y, x)
			if !(s.Cont && (a.Blstats.Y != s.Y || a.Blstats.X != s.X)) {
				return ret(nil)
			}
			dis := a.Bfs()
			if dis[s.Y][s.X] == -1 {
				Panic("end point is no longer accessible")
			}
			path := a.Path(a.Blstats.Y, a.Blstats.X, s.Y, s.X, nil)
			path = path[1:]
			if s.StopOneBefore {
				if len(path) > 0 {
					path = path[:len(path)-1]
				}
			}
			s.Path, s.PI = path, 0
			s.PC = 3
			if s.O.Fast && len(path) > 2 {
				s.MyY, s.MyX = a.Blstats.Y, a.Blstats.X
				s.PC = 2
				last := path[len(path)-1]
				return call(fastGoTo(last.Y, last.X))
			}
		case 2:
			s.PC = 3
			if s.MyY != a.Blstats.Y || s.MyX != a.Blstats.X {
				s.PC = 1
			}
		case 3: // for y, x in path
			if s.PI == len(s.Path) {
				s.Cont = false // the path is walked without a break
				s.PC = 1
				continue
			}
			p := s.Path[s.PI]
			s.Y, s.X = p.Y, p.X // Python rebinds the target variables here
			if a.MonsterTracker.PeacefulMonsterMask[s.Y][s.X] || !a.CurrentLevel().Walkable[s.Y][s.X] {
				s.Cont = true
				s.PC = 1
				continue
			}
			s.PC = 4
			return call(MoveTo(a, s.Y, s.X))
		case 4:
			if s.callback(a) {
				return ret(nil)
			}
			s.StepsTaken++
			if s.O.MaxSteps != 0 && s.StepsTaken >= s.O.MaxSteps {
				s.Cont = false
				s.PC = 1
				continue
			}
			s.PI++
			s.PC = 3
		}
	}
}
