package skills

import (
	"strings"

	"aago/flow"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// meleeF is the body of melee_attack.
type meleeF struct {
	Y, X, PC int
}

func (s *meleeF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		g := int(a.Glyphs()[s.Y][s.X])
		Assert(G_MONS.Has(g) || G_INVISIBLE_MON.Has(g) || G_SWALLOW.Has(g))
		s.PC = 1
		return call(directionTo(a, s.Y, s.X))
	}
	return ret(true)
}

func MeleeAttack(y, x int) Frame { return flow.PosGuard(&meleeF{Y: y, X: x}) }

// useItemDirF is the atom body shared by zap and fire: command key, item letter, direction; returns true.
type useItemDirF struct {
	Cmd  Action
	Item *world.Item
	Dir  string
	PC   int
}

func (s *useItemDirF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(s.Cmd))
	case 1:
		if strings.Contains(a.Message, "You can't do that while carrying so much stuff.") {
			// not in the Python bot: the letter would run as a command (see getAvailableActions)
			Panic("can't throw or zap while carrying so much stuff")
		}
		s.PC = 2
		return call(sense.TypeText(a.Inventory.Items.GetLetter(s.Item)))
	case 2:
		s.PC = 3
		return call(Direction(s.Dir))
	}
	return ret(true)
}

func Zap(item *world.Item, dir string) Frame {
	return sense.Atom(&useItemDirF{Cmd: A_Command_ZAP, Item: item, Dir: dir})
}

// fireF is fire(item, direction): false when polymorphed, else throw it.
type fireF struct {
	Item *world.Item
	Dir  string
	PC   int
}

func Fire(item *world.Item, dir string) Frame { return &fireF{Item: item, Dir: dir} }

func (s *fireF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		if c.W.Character.Prop.Polymorph() {
			return ret(false)
		}
		s.PC = 1
		return call(sense.Atom(&useItemDirF{Cmd: A_Command_THROW, Item: s.Item, Dir: s.Dir}))
	}
	return ret(true)
}
