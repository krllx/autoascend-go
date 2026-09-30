package skills

import (
	"fmt"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// ---- drop

// dropF is drop(items, counts, smart); counts nil means None. Returns false if nothing can be dropped.
type dropF struct {
	Items  []*world.Item
	Counts []int
	Smart  bool
	PC     int
}

func Drop(items []*world.Item, counts []int, smart bool) Frame {
	return &dropF{Items: items, Counts: counts, Smart: smart}
}

func (s *dropF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		s.PC = 1
		if s.Smart {
			return call(moveToInventoryList(s.Items))
		}
	case 1:
		s.Items = c.Ret.([]*world.Item)
	case 2:
		s.PC = 3
		return call(sense.GetItemsBelowMe(false))
	case 3:
		return ret(true)
	}
	items := s.Items
	counts := checkCounts(items, s.Counts)
	Assert(len(items) > 0)
	for _, item := range items {
		Assert(inv.InAllItems(item))
	}
	assertCounts(items, counts)

	letters := make([]byte, len(items))
	for i, item := range items {
		letters[i] = inv.Items.AllLetters[Index(inv.Items.AllItems, item)]
	}
	var textsToType []string
	for i := 0; i < len(items); i++ {
		if counts[i] != 0 {
			t := ""
			if items[i].Count != counts[i] {
				t = fmt.Sprint(counts[i])
			}
			textsToType = append(textsToType, t+string(letters[i]))
		}
	}
	if !Any(items, func(i *world.Item) bool { return i.CanBeDroppedFromInventory() }) {
		return ret(false)
	}
	d := a.Term.Dialog(A_Command_DROPTYPE, &dropGen{Texts: textsToType})
	s.PC = 2
	return call(sense.Atom(d))
}

// dropGen answers the multi-drop menus: category 'a', then the letters page by page, then MORE.
type dropGen struct {
	Texts []string // still to type
	Round []string // copy of Texts for the scan of the current page
	J     int      // index in Round
	I     int      // pages turned
	PC    int
}

func (g *dropGen) Resume(c *Ctx) Cmd {
	a := c.W
	for {
		switch g.PC {
		case 0:
			if strings.Contains(strings.Join(a.SinglePopup, "\n"), "Drop what type of items?") {
				g.PC = 1
				return call(a.Term.RawKey(world.Ch('a')))
			}
			g.PC = 3
		case 1:
			g.PC = 3
			return call(a.Term.RawKey(A_MiscAction_MORE))
		case 3:
			Assert(strings.Contains(strings.Join(a.SinglePopup, "\n"), "What would you like to drop?"),
				a.SingleMessage, a.SinglePopup)
			g.PC = 4
		case 4: // while texts_to_type
			if len(g.Texts) == 0 {
				g.PC = 9
				return call(a.Term.RawKey(A_MiscAction_MORE))
			}
			g.Round = append([]string{}, g.Texts...)
			g.J = 0
			g.PC = 7
		case 5: // Round[J] is typed
			g.Texts = removeFirstStr(g.Texts, g.Round[g.J])
			g.J++
			g.PC = 7
		case 6: // the page is turned
			g.I++
			Assert(g.I < 100, "infinite loop", g.Texts, a.Message)
			g.PC = 4
		case 7: // for text in texts_to_type.copy()
			for g.J < len(g.Round) {
				text := g.Round[g.J]
				letter := text[len(text)-1]
				if strings.Contains(strings.Join(a.SinglePopup, "\n"), string(letter)+" - ") {
					g.PC = 5
					return call(a.Term.RawText(text))
				}
				g.J++
			}
			if len(g.Texts) > 0 {
				g.PC = 6
				return call(a.Term.RawKey(A_TextCharacters_SPACE))
			}
			Assert(g.I < 100, "infinite loop", g.Texts, a.Message)
			g.PC = 4
		case 9:
			return ret(nil)
		}
	}
}
