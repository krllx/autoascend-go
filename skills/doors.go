package skills

import (
	"aago/flow"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// openDoorF is the body of open_door (inside panic_if_position_changes): returns whether the door opened.
type openDoorF struct {
	Y, X, PC int
}

func (s *openDoorF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		Assert(G_DOOR_CLOSED.Has(int(a.Glyphs()[s.Y][s.X])))
		s.PC = 1
		return call(directionTo(a, s.Y, s.X))
	}
	a.CurrentLevel().DoorOpenCount[s.Y][s.X]++
	return ret(!G_DOOR_CLOSED.Has(int(a.Glyphs()[s.Y][s.X])))
}

func OpenDoor(y, x int) Frame { return flow.PosGuard(&openDoorF{Y: y, X: x}) }

// kickF is the atom body of kick: the kick key, then the direction from the position at that moment.
type kickF struct {
	Y, X, PC int
}

func (s *kickF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_KICK))
	case 1:
		s.PC = 2
		return call(Direction(world.CalcDirection(a.Blstats.Y, a.Blstats.X, s.Y, s.X, false)))
	}
	return ret(nil)
}

func Kick(y, x int) Frame { return flow.PosGuard(sense.Atom(&kickF{Y: y, X: x})) }
