package skills

import (
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

func popup0(p []string) string {
	if len(p) == 0 {
		Raise("IndexError", "list index out of range")
	}
	return p[0]
}

// useContainerF is use_container(container, items_to_put, items_to_take, put_counts, take_counts); nil
// counts mean None.
type useContainerF struct {
	Container           *world.Item
	Put, Take           []*world.Item
	PutCounts, TakeCnts []int
	// containers to re-check afterwards: chain(all_items, items_below_me), read before the loops
	All, Below []*world.Item
	BelowNone  bool
	List, I    int
	PC         int // 0: start, 1: atom done, 2: re-check loops
}

func UseContainer(container *world.Item, put, take []*world.Item, putCounts, takeCounts []int) Frame {
	return &useContainerF{Container: container, Put: put, Take: take, PutCounts: putCounts, TakeCnts: takeCounts}
}

func (s *useContainerF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	container := s.Container
	switch s.PC {
	case 0:
		Assert(inv.InAllItems(container) || inv.InBelowMe(container))
		for _, item := range s.Put {
			Assert(inv.InAllItems(item))
		}
		for _, item := range s.Take {
			if container.Content == nil {
				Raise("AttributeError", "'NoneType' object has no attribute 'items'")
			}
			Assert(Contains(container.Content.Items, item))
		}
		Assert(container.IsContainer())
		Assert(len(s.Take)-len(s.Put) <= inv.Items.FreeSlots()) // TODO: take counts into consideration
		Assert(!container.Content.Locked, container)
		s.PC = 1
		return call(sense.Atom(&useContainerBodyF{U: s}))
	case 1:
		// for item in chain(self.items.all_items, self.items_below_me)
		s.All = inv.Items.AllItems
		s.Below, s.BelowNone = inv.ItemsBelowMe, inv.ItemsBelowMeNone
		s.PC = 2
	}
	for {
		list := s.All
		if s.List == 1 {
			list = s.Below
		}
		for s.I < len(list) {
			item := list[s.I]
			s.I++
			if item.IsContainer() && world.StrPtrEq(item.ContainerID, container.ContainerID) {
				return call(sense.CheckContainerContent(item))
			}
		}
		if s.List == 1 {
			return ret(nil)
		}
		if s.BelowNone {
			Raise("TypeError", "'NoneType' object is not iterable")
		}
		s.List, s.I = 1, 0
	}
}

// useContainerBodyF is the atom operation of use_container: open the container and run the dialog.
type useContainerBodyF struct {
	U  *useContainerF
	PC int // 0: start, 1: applied, 2: loot prompt, 3: dialog done
}

func (s *useContainerBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	container := s.U.Container
	switch s.PC {
	case 0:
		// TODO: refactor: the same fragment is in check_container_content
		if inv.InAllItems(container) {
			s.PC = 1
			return call(a.Term.Key(A_Command_APPLY))
		} else if inv.InBelowMe(container) {
			s.PC = 2
			return call(a.Term.Key(A_Command_LOOT))
		}
		Assert(false)
	case 1:
		Assert(!strings.Contains(a.Message, "You can't do that while carrying so much stuff."), a.Message)
		d := a.Term.Dialog(world.Ch(inv.Items.GetLetter(container)[0]), &useContainerGen{U: s.U})
		s.PC = 3
		return call(d)
	case 2:
		Assert(!Contains(a.Popup, "Loot which containers?"), a.Popup)
		Assert(!strings.Contains(a.Message, "Loot in what direction?"))
		if strings.Contains(a.Message, "You don't find anything here to loot.") {
			Panic("no container to loot")
		}
		r := FindAll(sense.LootRe, a.Message)
		Assert(len(r) == 1, a.Message)
		text := r[0][0]
		it := inv.ItemManager.GetItemFromText(text, -1, -1, inv.CurrentPositionID())
		if world.StrPtrEq(it.ContainerID, container.ContainerID) {
			d := a.Term.Dialog(world.Ch('y'), &useContainerGen{U: s.U})
			s.PC = 3
			return call(d)
		}
		return call(a.Term.Key(world.Ch('n'))) // and read the next prompt at PC 2
	}
	return ret(nil)
}

// useContainerGen answers the container menus: put in / take out, then select the items.
type useContainerGen struct {
	U  *useContainerF
	PC int
}

func (g *useContainerGen) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	u := g.U
	put, take := len(u.Put) > 0, len(u.Take) > 0
	for {
		switch g.PC {
		case 0:
			if strings.Contains(a.Message, " vanished!") {
				if u.Container.ContainerID == nil {
					Raise("KeyError", "None")
				}
				if _, ok := inv.ItemManager.ContainerContents[*u.Container.ContainerID]; !ok {
					Raise("KeyError", "%s", *u.Container.ContainerID)
				}
				delete(inv.ItemManager.ContainerContents, *u.Container.ContainerID)
				Panic("some items from the container vanished")
			}
			g.PC = 1
			if strings.Contains(a.SingleMessage, "You carefully open ") || strings.Contains(a.SingleMessage, "You open ") {
				return call(a.Term.RawKey(world.Ch(' ')))
			}
		case 1:
			Assert(!strings.Contains(a.SingleMessage, "You have no free hand."), "TODO: handle it")
			Assert(strings.Contains(popup0(a.SinglePopup), "Do what with "))
			g.PC = 2
			if put && take {
				return call(a.Term.RawKey(world.Ch('r')))
			} else if put && !take {
				return call(a.Term.RawKey(world.Ch('i')))
			} else if !put && take {
				return call(a.Term.RawKey(world.Ch('o')))
			}
			Assert(false)
		case 2:
			if !put {
				g.PC = 5
				continue
			}
			g.PC = 3
			if strings.Contains(popup0(a.SinglePopup), "Put in what type of objects?") {
				return call(a.Term.RawText("a\r"))
			}
		case 3:
			Assert(strings.Contains(popup0(a.SinglePopup), "Put in what?"), a.SingleMessage, a.SinglePopup)
			g.PC = 5
			return call(selectItems(u.Put, u.PutCounts))
		case 5:
			if !take {
				return ret(nil)
			}
			if len(a.SinglePopup) == 0 || (a.SinglePopup[0] != "Take out what type of objects?" &&
				a.SinglePopup[0] != "Take out what?") {
				Assert(!strings.Contains(a.Message, " inside, you are blasted by a "), a.Message)
				Assert(a.SingleMessage != "" || len(a.SinglePopup) > 0, a.Message, a.Popup)
				return call(a.Term.RawKey(world.Ch(' ')))
			}
			g.PC = 6
			if popup0(a.SinglePopup) == "Take out what type of objects?" {
				return call(a.Term.RawText("a\r"))
			}
		case 6:
			Assert(strings.Contains(popup0(a.SinglePopup), "Take out what?"))
			g.PC = 7
			return call(selectItems(u.Take, u.TakeCnts))
		case 7:
			g.PC = 8
			if a.Observation.Misc[2] != 0 {
				return call(a.Term.RawKey(world.Ch(' ')))
			}
		case 8:
			if strings.Contains(a.SingleMessage, "You have ") && strings.Contains(a.SingleMessage, " removing ") &&
				strings.Contains(a.SingleMessage, "Continue? [ynq] (q)") {
				return call(a.Term.RawKey(world.Ch('y')))
			}
			return ret(nil)
		}
	}
}
