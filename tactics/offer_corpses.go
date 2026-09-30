package tactics

import (
	"slices"
	"strings"

	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// ---- offer_corpses: sacrifice fresh corpses on a co-aligned altar until the god grants an artifact

type OfferCorpses struct{}

func (OfferCorpses) Check(a *world.Agent) (bool, Frame) {
	g := a.GlobalLogic
	g.ItemPriority.TakeSacrificialCorpses = false
	if g.St.GotArtifact {
		return false, nil
	}
	lvl := a.CurrentLevel()
	altars := Filter(lvl.Altars.Keys(), func(p Pos) bool { return lvl.Altars.MustGet(p) == a.Character.Alignment })
	if len(altars) == 0 {
		return false, nil
	}
	dis := a.Bfs()
	altars = Filter(altars, func(p Pos) bool { return dis[p.Y][p.X] != -1 })
	if len(altars) == 0 {
		return false, nil
	}
	g.ItemPriority.TakeSacrificialCorpses = true
	if !Any(world.FlattenItems(a.Inventory.Items.AllItems), g.CanSacrify) {
		return false, nil
	}
	altar := MinBy(altars, func(p Pos) int32 { return dis[p.Y][p.X] })
	return true, &offerCorpsesF{Altar: altar}
}

type offerCorpsesF struct {
	Altar Pos
	PC    int
}

func (s *offerCorpsesF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return call(skills.GoTo(s.Altar.Y, s.Altar.X, skills.GoToOpts{}))
	}
	if s.PC == 1 {
		s.PC = 2
		return call(flow.PosGuard(&offerLoopF{}))
	}
	return ret(nil)
}

// offerLoopF offers corpses one by one until there is no point.
type offerLoopF struct{ PC int }

func (s *offerLoopF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 1 && c.Ret.(bool) { // stop
		return ret(nil)
	}
	items := world.FlattenItems(a.Inventory.Items.AllItems)
	i := slices.IndexFunc(items, a.GlobalLogic.CanSacrify)
	if i < 0 {
		return ret(nil)
	}
	s.PC = 1
	return call(sense.Atom(&offerCorpseF{Item: items[i]}))
}

// offerCorpseF sacrifices one corpse on the altar under the agent; returns true when there is no point in
// offering more.
type offerCorpseF struct {
	Item *world.Item
	PC   int
}

func (s *offerCorpseF) Resume(c *Ctx) Cmd {
	a := c.W
	g := a.GlobalLogic
	switch s.PC {
	case 0:
		s.PC = 1
		return call(skills.MoveToInventory(s.Item))
	case 1:
		s.Item = c.Ret.(*world.Item)
		Assert(g.CanSacrify(s.Item))
		s.PC = 2
		return call(a.Term.Key(A_Command_OFFER))
	case 2:
		if (strings.Contains(a.Message, "There is ") || strings.Contains(a.Message, "There are ")) &&
			(strings.Contains(a.Message, "sacrifice it?") || strings.Contains(a.Message, "sacrifice one?")) {
			return call(sense.TypeText("n"))
		}
		Assert(strings.Contains(a.Message, "What do you want to sacrifice?"), a.Message)
		s.PC = 3
		return call(sense.TypeText(a.Inventory.Items.GetLetter(s.Item)))
	case 3:
		switch {
		case strings.Contains(a.Message, "Nothing happens."):
			s.PC = 4
			return call(sense.CallItem(s.Item, "old"))
		case strings.Contains(a.Message, "Use my gift wisely"):
			g.St.GotArtifact = true
			s.PC = 4
			return call(sense.GetItemsBelowMe(false))
		case strings.Contains(a.Message, "So this is how you repay loyalty?"):
			Panic("pet sacrified")
		default:
			Assert(strings.Contains(a.Message, "Your sacrifice is consumed in a flash of light") ||
				strings.Contains(a.Message, "Your sacrifice is consumed in a burst of flame") ||
				(strings.Contains(a.Message, "The blood covers the altar!") &&
					strings.Contains(a.Message, "You have summoned ")),
				a.Message)
		}
		return ret(false)
	}
	return ret(true) // PC 4
}
