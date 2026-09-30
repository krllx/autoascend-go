package tactics

import (
	"sort"

	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/skills"
	"aago/world"
)

// ---- eat_from_inventory

type EatFromInventory struct{}

func (EatFromInventory) Check(a *world.Agent) (bool, Frame) {
	if a.Blstats.HungerState < Hunger_HUNGRY {
		return false, nil
	}
	ok := []int{MON_FromName("lizard") - NH_GLYPH_MON_OFF, MON_FromName("lichen") - NH_GLYPH_MON_OFF}
	for it := range world.EachItem(a.Inventory.Items.AllItems) {
		if it.Category == NH_FOOD_CLASS && it.Objs[0].Name != "sprig of wolfsbane" &&
			(!it.IsCorpse() || Contains(ok, it.MonsterID)) {
			return true, flow.ThenNil(skills.Eat(it, false, true))
		}
	}
	return false, nil
}

// ---- eat_corpses_from_ground

type EatCorpses struct{ OnlyBelowMe bool }

func (n EatCorpses) Check(a *world.Agent) (bool, Frame) {
	level := a.CurrentLevel()
	var toEat []world.CorpseTarget
	collect := func(p Pos, m *OMap[int, int]) {
		for _, mid := range m.Keys() {
			age := m.MustGet(mid)
			if level.Shop[p.Y][p.X] {
				continue
			}
			if a.IsCorpseEditable(mid, age) {
				toEat = append(toEat, world.CorpseTarget{p.Y, p.X, mid})
			}
		}
	}
	if n.OnlyBelowMe {
		p := Pos{a.Blstats.Y, a.Blstats.X}
		if !level.CorpsesToEat.Has(p) {
			return false, nil
		}
		collect(p, level.CorpsesToEat.MustGet(p))
	} else {
		for _, p := range level.CorpsesToEat.Keys() {
			collect(p, level.CorpsesToEat.MustGet(p))
		}
	}
	if len(toEat) == 0 {
		return false, nil
	}
	dis := a.Bfs()
	var f []world.CorpseTarget
	for _, e := range toEat {
		if dis[e.Y][e.X] != -1 {
			f = append(f, e)
		}
	}
	sort.SliceStable(f, func(i, j int) bool { return dis[f[i].Y][f[i].X] < dis[f[j].Y][f[j].X] })
	if len(f) == 0 {
		return false, nil
	}
	t := f[0]
	tp := Pos{t.Y, t.X}
	if t.Y != a.Blstats.Y || t.X != a.Blstats.X {
		return true, &eatCorpsesF{Level: level, Y: t.Y, X: t.X, MonsterID: t.MonsterID}
	}

	// already there: the rest of the generator runs in the check up to the first corpse to eat
	if level.CorpsesToEat.Has(tp) && level.CorpsesToEat.MustGet(tp).Has(t.MonsterID) {
		age := level.CorpsesToEat.MustGet(tp).MustGet(t.MonsterID)
		if level.Shop[t.Y][t.X] {
			level.CorpsesToEat.Del(tp)
			Raise("StopIteration", "")
		}
		if a.Inventory.ItemsBelowMeNone {
			Raise("TypeError", "'NoneType' object is not iterable")
		}
		items := a.Inventory.ItemsBelowMe
		for i, item := range items {
			if eatCorpseMatches(item, t.MonsterID) && a.IsCorpseEditable(t.MonsterID, age) {
				return true, &eatCorpsesRestF{Items: append([]*world.Item(nil), items[i:]...), MonsterID: t.MonsterID, Age: age}
			}
		}
		level.CorpsesToEat.MustGet(tp).Del(t.MonsterID)
	}
	return false, nil
}

func eatCorpseMatches(item *world.Item, monsterID int) bool {
	return item.IsCorpse() && item.MonsterID == monsterID
}

// stillBelow: the item is in the current list of items below me. Not in the Python bot, which eats every
// matching corpse of the list it saw on arrival; after the first meal that list is replaced with new items, the
// next old corpse is neither below nor in the inventory, and eat() failed its "TODO: nested containers" assert
// (seed 1123). Now the rest of the old list is skipped and the next check of the tactic eats the next corpse.
func stillBelow(a *world.Agent, item *world.Item) bool {
	return !a.Inventory.ItemsBelowMeNone && Contains(a.Inventory.ItemsBelowMe, item)
}

// eatCorpsesRestF eats the first item (already checked) and every other matching corpse in Items.
type eatCorpsesRestF struct {
	Items          []*world.Item
	MonsterID, Age int
	K              int
}

func (s *eatCorpsesRestF) Resume(c *Ctx) Cmd {
	a := c.W
	for s.K < len(s.Items) {
		k, it := s.K, s.Items[s.K]
		s.K++
		if k == 0 || (eatCorpseMatches(it, s.MonsterID) && a.IsCorpseEditable(s.MonsterID, s.Age) && stillBelow(a, it)) {
			return call(skills.Eat(it, false, true))
		}
	}
	return ret(nil)
}

// eatCorpsesF walks to the corpse and eats every matching corpse there.
type eatCorpsesF struct {
	Level     *world.Level
	Y, X      int
	MonsterID int
	Age       int
	Items     []*world.Item
	I         int
	PC        int
}

func (s *eatCorpsesF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(skills.GoTo(s.Y, s.X, skills.GoToOpts{}))
	case 1:
		level, tp := s.Level, Pos{s.Y, s.X}
		if !(level.CorpsesToEat.Has(tp) && level.CorpsesToEat.MustGet(tp).Has(s.MonsterID)) {
			return ret(nil)
		}
		s.Age = level.CorpsesToEat.MustGet(tp).MustGet(s.MonsterID)
		if level.Shop[s.Y][s.X] {
			level.CorpsesToEat.Del(tp)
			return ret(nil)
		}
		if a.Inventory.ItemsBelowMeNone {
			Raise("TypeError", "'NoneType' object is not iterable")
		}
		s.Items = a.Inventory.ItemsBelowMe
		s.PC = 2
	}
	for s.I < len(s.Items) {
		item := s.Items[s.I]
		s.I++
		if eatCorpseMatches(item, s.MonsterID) && a.IsCorpseEditable(s.MonsterID, s.Age) && stillBelow(a, item) {
			return call(skills.Eat(item, false, true))
		}
	}
	return ret(nil)
}
