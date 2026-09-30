package sense

import (
	"regexp"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/world"
)

var LootRe = regexp.MustCompile(`There is ([a-zA-z0-9# ]+) here, loot it\? \[ynq\] \(q\)`)

// checkContainerF is Inventory.check_container_content(item): open the container (apply it, or #loot it
// from the floor) and read what is inside.
type checkContainerF struct {
	Item *world.Item
	PC   int
}

func CheckContainerContent(item *world.Item) Frame { return &checkContainerF{Item: item} }

func (s *checkContainerF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	if s.PC == 1 {
		return ret(nil)
	}
	item := s.Item
	Assert(item.IsPossibleContainer() || item.IsContainer())
	Assert(inv.InAllItems(item) || inv.InBelowMe(item))

	var content *world.ContainerContent
	if item.Content != nil {
		content = item.Content
		content.Reset()
	} else {
		content = world.NewContainerContent()
	}
	s.PC = 1
	return call(Atom(&containerLookF{Item: item, Content: content}))
}

// containerLookF is the atom operation of check_container_content.
type containerLookF struct {
	Item        *world.Item
	Content     *world.ContainerContent
	BagOfTricks bool
	PC          int // 0: start, 1: applied, 2: looked (apply), 3: loot prompt, 4: looked, 5: called
}

func (s *containerLookF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	item := s.Item
	switch s.PC {
	case 0:
		// TODO: refactor: the same fragment is in use_container
		if inv.InAllItems(item) {
			s.PC = 1
			return call(a.Term.Key(A_Command_APPLY))
		}
		s.PC = 3
		return call(a.Term.Key(A_Command_LOOT))
	case 1:
		if strings.Contains(a.Message, "You can't do that while carrying so much stuff.") {
			return ret(nil) // TODO: is not changing the content in this case a good way to handle this?
		}
		d := a.Term.Dialog(world.Ch(inv.Items.GetLetter(item)[0]), &containerLookGen{Look: s})
		s.PC = 2
		return call(d)
	case 2:
		if strings.Contains(a.Message, "You have no hands!") {
			return ret(nil)
		}
	case 3:
		if strings.Contains(a.Message, "You don't find anything here to loot.") {
			Panic("no container below me")
		}
		Assert(!Contains(a.Popup, "Loot which containers?"), a.Popup)
		Assert(strings.Contains(a.Message, "There is ") && strings.Contains(a.Message, ", loot it?"), a.Message)
		r := FindAll(LootRe, a.Message)
		Assert(len(r) == 1, a.Message)
		text := r[0][0]
		it := inv.ItemManager.GetItemFromText(text, -1, -1, inv.CurrentPositionID())
		if (item.ContainerID != nil && world.StrPtrEq(it.ContainerID, item.ContainerID)) ||
			(item.ContainerID == nil && item.TextNone == it.TextNone && item.Text == it.Text) {
			d := a.Term.Dialog(world.Ch('y'), &containerLookGen{Look: s})
			s.PC = 4
			return call(d)
		}
		return call(a.Term.Key(world.Ch('n'))) // and read the next prompt at PC 3
	case 5:
		return s.finish(inv)
	}

	// after the look (PC 2 or 4)
	if s.BagOfTricks {
		Assert(item.Content == nil)
		Panic("bag of tricks bites")
	}
	if inv.InAllItems(item) && (item.ContainerID == nil || item.Comment != *item.ContainerID) {
		s.PC = 5
		return call(CallItem(item, world.PyStrOfID(item.ContainerID)))
	}
	return s.finish(inv)
}

func (s *containerLookF) finish(inv *world.Inventory) Cmd {
	item := s.Item
	if item.Content == nil {
		Assert(item.ContainerID != nil)
		_, ok := inv.ItemManager.ContainerContents[*item.ContainerID]
		Assert(!ok)
		inv.ItemManager.ContainerContents[*item.ContainerID] = s.Content
		item.Content = s.Content
	}

	// TODO: make it more elegant
	if len(item.Glyphs) == 1 && !inv.ItemManager.IsNotBagOfTricks[item.Glyphs[0]] {
		inv.ItemManager.IsNotBagOfTricks[item.Glyphs[0]] = true
		inv.ItemManager.UpdatePossibleObjects(item)
	}
	return ret(nil)
}

// containerLookGen answers the screens after the container is opened and fills Look.Content.
type containerLookGen struct {
	Look *containerLookF
	PC   int
}

func (g *containerLookGen) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	content := g.Look.Content
	switch g.PC {
	case 0:
		g.PC = 1
		if strings.Contains(a.SingleMessage, "You carefully open ") || strings.Contains(a.SingleMessage, "You open ") {
			return call(a.Term.RawKey(world.Ch(' ')))
		}
		fallthrough
	case 1:
		if strings.Contains(a.SingleMessage, "It develops a huge set of teeth and bites you!") {
			g.Look.BagOfTricks = true
			return ret(nil)
		}
		if strings.Contains(a.SingleMessage, "Hmmm, it turns out to be locked.") ||
			strings.Contains(a.SingleMessage, "It is locked.") {
			content.Locked = true
			g.PC = 9
			return call(a.Term.RawKey(A_Command_ESC))
		}
		if world.CheckIfTriggeredContainerTrap(a.SingleMessage) {
			a.StatsLogger.LogEvent("triggered_undetected_trap")
			Panic("triggered trap while looting")
		}
		if strings.Contains(a.SingleMessage, "You have no hands!") ||
			strings.Contains(a.SingleMessage, "You have no free hand.") {
			return ret(nil)
		}
		if strings.Contains(a.Message, " vanished!") {
			Panic("some items from the container vanished")
		}
		if strings.Contains(a.Message, "cat") && strings.Contains(a.Message, " inside the box is ") {
			Panic("encountered a cat in a box")
		}
		Assert(len(a.SinglePopup) > 0, a.SingleMessage)
		if !strings.Contains(strings.Join(a.SinglePopup, "\n"), "\no - ") {
			// ':' sometimes doesn't display items correctly if there's >= 22 items (the first page isn't shown)
			g.PC = 2
			return call(a.Term.RawKey(world.Ch(':')))
		}
		g.PC = 3
		return call(a.Term.RawText("o"))
	case 2:
		if strings.Contains(a.SingleMessage, " is empty") {
			return ret(nil)
		}
		Assert(false, a.SingleMessage, a.SinglePopup)
	case 3:
		if strings.Contains(a.SingleMessage, " is empty") && len(a.SinglePopup) == 0 {
			return ret(nil)
		}
		g.PC = 4
		if len(a.SinglePopup) > 0 && a.SinglePopup[0] == "Take out what type of objects?" {
			return call(a.Term.RawText("a\r"))
		}
		fallthrough
	case 4:
		if len(a.SinglePopup) > 0 && strings.Contains(a.SinglePopup[0], "Take out what?") {
			g.PC = 5
		} else {
			Assert(false, a.SingleMessage, a.SinglePopup)
		}
		fallthrough
	case 5:
		if a.Observation.Misc[2] != 0 {
			return call(a.Term.RawKey(world.Ch(' ')))
		}
		category := -1
		cnt := 0
		for _, l := range a.Popup {
			if l == "Take out what?" {
				cnt++
			}
		}
		Assert(cnt == 1, a.Popup)
		popup := a.Popup
		for _, text := range popup[Index(popup, "Take out what?")+1:] {
			if text == "" {
				continue
			}
			if cat, ok := perception.InventoryNameToCategory[text]; ok {
				category = cat
				continue
			}
			Assert(category != -1)
			Assert(world.PySlice(text, 1, 4) == " - ")
			text = text[4:]
			content.Items = append(content.Items, inv.ItemManager.GetItemFromText(text, category, -1, nil))
		}
	}
	return ret(nil) // case 9: the locked container is closed
}
