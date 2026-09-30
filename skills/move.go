package skills

import (
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// dirF is direction(dir): one direction key; returns true.
type dirF struct {
	Dir string
	PC  int
}

func Direction(dir string) Frame { return &dirF{Dir: dir} }

func (s *dirF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		act, ok := world.DirActions[s.Dir]
		if !ok {
			Raise("KeyError", "%q", s.Dir)
		}
		s.PC = 1
		return call(c.W.Term.Key(act))
	}
	return ret(true)
}

// directionTo is direction(calc_direction(me, (y, x))), the direction computed when the call is made.
func directionTo(a *world.Agent, y, x int) Frame {
	return Direction(world.CalcDirection(a.Blstats.Y, a.Blstats.X, y, x, false))
}

// moveF is move(dir): a step in a direction or on stairs, with the position checks of agent.move.
type moveF struct {
	Dir    string
	EY, EX int
	Level  *world.Level // level before taking the stairs
	PC     int          // 0: start, 1: stairs done, 2: plain move done
}

func Move(dir string) Frame { return &moveF{Dir: dir} }

func (s *moveF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		dir := s.Dir
		s.EY = a.Blstats.Y + B2i(strings.Contains(dir, "s")) - B2i(strings.Contains(dir, "n"))
		s.EX = a.Blstats.X + B2i(strings.Contains(dir, "e")) - B2i(strings.Contains(dir, "w"))
		if (s.EY != a.Blstats.Y || s.EX != a.Blstats.X) && a.MonsterTracker.MonsterMask[PyIdx(s.EY, SY)][PyIdx(s.EX, SX)] {
			Panic("Monster on a next tile when moving: (%d,%d)", s.EY, s.EX)
		}
		if dir == "<" || dir == ">" {
			s.Level = a.CurrentLevel()
			s.PC = 1
			return call(sense.Atom(&stairsF{Dir: dir, EY: s.EY, EX: s.EX, Level: s.Level}))
		}
		s.PC = 2
		return call(Direction(dir))
	case 1:
		return ret(nil)
	}
	if a.Blstats.Y != s.EY || a.Blstats.X != s.EX {
		Panic("agent position do not match after \"move\": expected (%d, %d), got (%d, %d)", s.EY, s.EX, a.Blstats.Y, a.Blstats.X)
	}
	return ret(nil)
}

// stairsF is the atom operation of move on stairs: take them and remember where they lead.
type stairsF struct {
	Dir    string
	EY, EX int
	Level  *world.Level
	PC     int
}

func (s *stairsF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		s.PC = 1
		return call(Direction(s.Dir))
	}
	level := s.Level
	Assert(a.CurrentLevel().Key() != level.Key(), a.Message)
	level.StairDestination.Set(Pos{s.EY, s.EX}, world.StairDest{Level: a.CurrentLevel().Key(), Pos: Pos{a.Blstats.Y, a.Blstats.X}})
	a.CurrentLevel().StairDestination.Set(Pos{a.Blstats.Y, a.Blstats.X}, world.StairDest{Level: level.Key(), Pos: Pos{s.EY, s.EX}})
	return ret(nil)
}

func MoveTo(a *world.Agent, y, x int) Frame {
	return Move(world.CalcDirection(a.Blstats.Y, a.Blstats.X, y, x, false))
}

// fastGoToF is the atom body of _fast_go_to: the travel command, cursor moves, then '.'.
type fastGoToF struct {
	Y, X   int
	PY, PX int
	PC     int
}

func (s *fastGoToF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_TRAVEL))
	case 1:
		s.PY, s.PX = a.CursorPos.Y, a.CursorPos.X
	case 2: // a cursor move is done
		Assert(Pos{s.PY, s.PX} == a.CursorPos)
	case 3:
		return ret(nil)
	}
	if s.PY != s.Y || s.PX != s.X {
		dy, dx := Sign(s.Y-s.PY), Sign(s.X-s.PX)
		d := world.CalcDirection(s.PY, s.PX, s.PY+dy, s.PX+dx, false)
		s.PY += dy
		s.PX += dx
		s.PC = 2
		return call(Direction(d))
	}
	s.PC = 3
	return call(Direction("."))
}

func fastGoTo(y, x int) Frame { return sense.Atom(&fastGoToF{Y: y, X: x}) }
