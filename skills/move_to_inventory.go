package skills

import (
	. "aago/internal/pyrt"
	"aago/sense"
	"aago/world"
)

// ---- move to inventory

// moveToInventoryF is move_to_inventory(items): pick the items up or take them out of containers, then
// return the inventory items that correspond to them.
type moveToInventoryF struct {
	Items  []*world.Item
	Single bool // move_to_inventory(item): return the one item
	PC     int
}

func MoveToInventory(item *world.Item) Frame {
	return &moveToInventoryF{Items: []*world.Item{item}, Single: true}
}

func moveToInventoryList(items []*world.Item) Frame { return &moveToInventoryF{Items: items} }

func (s *moveToInventoryF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	items := s.Items
	switch s.PC {
	case 0:
		// all items in self.items will be updated!
		var moved []*world.Item // the set moved_items
		for _, item := range items {
			if inv.InAllItems(item) && !Contains(moved, item) {
				moved = append(moved, item)
			}
		}
		if len(moved) == len(items) {
			return s.result(items)
		}
		s.PC = 1
		return call(sense.Atom(&moveToInventoryBodyF{Items: items, Moved: moved}))
	case 1:
		// TODO: HACK
		lo := *a.LastObservation
		lo.InvStrs = a.Observation.InvStrs
		lo.InvOclasses = a.Observation.InvOclasses
		lo.InvGlyphs = a.Observation.InvGlyphs
		lo.InvLetters = a.Observation.InvLetters
		a.LastObservation = &lo
		s.PC = 2
		return call(sense.UpdateItems(true))
	}
	r := []*world.Item{}
	for _, item := range items {
		var cands []*world.Item
		for _, i := range inv.Items.AllItems {
			if !Contains(r, i) {
				cands = append(cands, i)
			}
		}
		r = append(r, world.FindEquivalentItem(item, cands))
	}
	return s.result(r)
}

func (s *moveToInventoryF) result(r []*world.Item) Cmd {
	if s.Single {
		Assert(len(r) == 1)
		return ret(r[0])
	}
	return ret(r)
}

// moveToInventoryBodyF is the atom operation of move_to_inventory.
type moveToInventoryBodyF struct {
	Items      []*world.Item
	Moved      []*world.Item   // the set moved_items, in insertion order
	Containers [][]*world.Item // chain(items_below_me, items): the lists, read when the loop reaches them
	List, I    int
	PC         int // 0: start, 1: picked up, 2: container loop
}

func (s *moveToInventoryBodyF) mark(items []*world.Item) {
	for _, i := range items {
		if !Contains(s.Moved, i) {
			s.Moved = append(s.Moved, i)
		}
	}
}

func (s *moveToInventoryBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		var its []*world.Item
		for _, i := range s.Items {
			if inv.InBelowMe(i) {
				its = append(its, i)
			}
		}
		s.PC = 1
		if len(its) > 0 {
			s.mark(its)
			return call(Pickup(its, nil))
		}
		fallthrough
	case 1:
		// for container in chain(self.items_below_me, self.items)
		if inv.ItemsBelowMeNone {
			Raise("TypeError", "'NoneType' object is not iterable")
		}
		s.Containers = [][]*world.Item{inv.ItemsBelowMe, nil}
		s.List, s.I = 0, 0
		s.PC = 2
	}
	for s.List < 2 {
		if s.List == 1 && s.I == 0 {
			s.Containers[1] = inv.Items.AllItems
		}
		list := s.Containers[s.List]
		for s.I < len(list) {
			container := list[s.I]
			s.I++
			if container.IsContainer() {
				var its []*world.Item
				for _, i := range s.Items {
					if Contains(container.Content.Items, i) {
						its = append(its, i)
					}
				}
				if len(its) > 0 {
					s.mark(its)
					return call(UseContainer(container, []*world.Item{}, its, nil, nil))
				}
			}
		}
		s.List++
		s.I = 0
	}
	var unique []*world.Item
	for _, x := range s.Items {
		if !Contains(unique, x) {
			unique = append(unique, x)
		}
	}
	eq := len(s.Moved) == len(unique)
	if eq {
		for _, x := range s.Items {
			if !Contains(s.Moved, x) {
				eq = false
				break
			}
		}
	}
	Assert(eq, "TODO: nested containers", s.Moved, s.Items)
	return ret(nil)
}
