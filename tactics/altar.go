package tactics

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// ---- check_altar: learn the alignment of the altar below me

type checkAltarN struct{}

func (checkAltarN) Check(a *world.Agent) (bool, Frame) {
	level := a.CurrentLevel()
	pos := Pos{a.Blstats.Y, a.Blstats.X}
	if level.Altars.Has(pos) && level.Altars.MustGet(pos) == world.Character_UNKNOWN {
		return true, sense.Atom(&checkAltarF{Level: level, Pos: pos})
	}
	return false, nil
}

type checkAltarF struct {
	Level *world.Level
	Pos   Pos
	PC    int
}

func (s *checkAltarF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		s.PC = 1
		return call(a.Term.Key(A_Command_LOOK))
	}
	text := a.Message
	if text == "" {
		text = a.Popup[PyIdx(0, len(a.Popup))]
	}
	r := Search(world.ExplAltarRe, text)
	Assert(r != nil, a.Message, a.Popup)
	alignment := r[1]
	al, ok := world.ExplNameToAlignment[alignment]
	Assert(ok, alignment, a.Message)
	s.Level.Altars.Set(s.Pos, al)
	return ret(nil)
}

// ---- identify_items_on_altar: drop unidentified items on the nearest reachable altar

func unknownDroppable(item *world.Item) bool {
	return item.CanBeDroppedFromInventory() && item.Status == perception.Item_UNKNOWN
}

type IdentifyOnAltar struct{}

func (IdentifyOnAltar) Check(a *world.Agent) (bool, Frame) {
	mask := Isin(&a.CurrentLevel().Objects, G_ALTAR)
	if !mask.Any() {
		return false, nil
	}
	dis := a.Bfs()
	mask = mask.And(dis.Ne(-1))
	if !mask.Any() {
		return false, nil
	}
	if !Any(world.FlattenItems(a.Inventory.Items.AllItems), unknownDroppable) {
		return false, nil
	}
	return true, &identifyOnAltarF{Mask: mask, Dis: dis}
}

type identifyOnAltarF struct {
	Mask BoolGrid
	Dis  I32Grid
	PC   int
}

func (s *identifyOnAltarF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		nearest := s.Mask.And(s.Dis.Eq(s.Dis.MinWhere(s.Mask)))
		altar := nearest.First()
		s.PC = 1
		return call(skills.GoTo(altar.Y, altar.X, skills.GoToOpts{}))
	case 1:
		itemsToDrop := Filter(world.FlattenItems(a.Inventory.Items.AllItems), unknownDroppable)
		if len(itemsToDrop) == 0 {
			Panic("items to drop on altar vanished")
		}
		// TODO: move chunking to inventory.drop
		itemsToDrop = itemsToDrop[:world.PySliceEnd(a.Inventory.Items.FreeSlots(), len(itemsToDrop))]
		s.PC = 2
		return call(skills.Drop(itemsToDrop, nil, true))
	}
	return ret(nil)
}
