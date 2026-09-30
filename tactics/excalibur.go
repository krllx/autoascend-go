package tactics

import (
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// ---- dip_for_excalibur: a lawful character of level 5+ dips a long sword into a fountain

func excaliburCandidate(a *world.Agent) *world.Item {
	var candidate *world.Item
	for item := range world.EachItem(a.Inventory.Items.AllItems) {
		if item.IsUnambiguous() && item.Object() == O_FromName1("long sword") {
			if item.DmgBonus != nil { // TODO: better condition for excalibur existance
				return nil
			}
			candidate = item
		}
	}
	return candidate
}

type DipForExcalibur struct{}

func (DipForExcalibur) Check(a *world.Agent) (bool, Frame) {
	g := a.GlobalLogic
	if a.Character.Alignment != world.Character_LAWFUL || a.Blstats.ExperienceLevel < 5 {
		return false, nil
	}
	if a.CurrentLevel().DungeonNumber == world.Level_GNOMISH_MINES &&
		(g.St.MinetownLevel == nil || a.CurrentLevel().Key() == *g.St.MinetownLevel) {
		return false, nil
	}
	dis := a.Bfs()
	mask := Isin(&a.CurrentLevel().Objects, G_FOUNTAIN).And(dis.Ne(-1))
	if !mask.Any() {
		return false, nil
	}
	if excaliburCandidate(a) == nil {
		return false, nil
	}
	return true, &dipForExcaliburF{Mask: mask}
}

type dipForExcaliburF struct {
	Mask BoolGrid
	PC   int
}

func (s *dipForExcaliburF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		fountain := s.Mask.First()
		s.PC = 1
		return call(skills.GoTo(fountain.Y, fountain.X, skills.GoToOpts{}))
	case 1:
		candidate := excaliburCandidate(a)
		if candidate == nil {
			return ret(nil)
		}
		// TODO: refactor
		s.PC = 2
		return call(sense.Atom(&dipF{Item: candidate}))
	}
	return ret(nil)
}

type dipF struct {
	Item *world.Item
	PC   int
}

func (s *dipF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(skills.MoveToInventory(s.Item))
	case 1:
		s.Item = c.Ret.(*world.Item)
		s.PC = 2
		return call(a.Term.Key(A_Command_DIP))
	case 2:
		s.PC = 3
		return call(sense.TypeText(a.Inventory.Items.GetLetter(s.Item)))
	}
	if strings.Contains(a.Message, "What do you want to dip ") && strings.Contains(a.Message, "into?") {
		Panic("no fountain here")
	}
	return ret(nil)
}
