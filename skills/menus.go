package skills

import (
	"strconv"

	. "aago/internal/pyrt"
	"aago/world"
)

// ---- menus

// selectItemsF is _select_items_in_popup(items, counts): pick the items in a menu, page by page, and confirm.
type selectItemsF struct {
	Items  []*world.Item
	Counts []int // nil: None
	LineI  int
	N      int
	Idx    int  // the item being selected
	Letter byte // its letter
	PC     int  // 0: new page, 1: scanning lines, 2: count typed, 3: letter typed, 4: '\r' sent, 5: page turned
}

func selectItems(items []*world.Item, counts []int) Frame {
	Assert(counts == nil || len(counts) == len(items))
	s := &selectItemsF{Items: append([]*world.Item{}, items...)}
	if counts != nil {
		s.Counts = append([]int{}, counts...)
	}
	return s
}

func (s *selectItemsF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 2:
		s.PC = 3
		return call(a.Term.RawKey(world.Ch(s.Letter)))
	case 3:
		i := s.Idx
		s.Items = append(s.Items[:i:i], s.Items[i+1:]...)
		if s.Counts != nil {
			s.Counts = append(s.Counts[:i:i], s.Counts[i+1:]...)
		}
		if len(s.Items) == 0 {
			s.PC = 4
			return call(a.Term.RawKey(world.Ch('\r')))
		}
		s.LineI++
		s.PC = 1
	case 4:
		return ret(true)
	case 0, 5:
		if len(a.SinglePopup) == 0 {
			Panic("no popup, but some items were not selected yet")
		}
		s.N = len(a.SinglePopup)
		s.LineI = 0
		s.PC = 1
	}
	for s.LineI < s.N {
		line := a.SinglePopup[PyIdx(s.LineI, len(a.SinglePopup))]
		if world.PySlice(line, 1, 4) == " - " {
			for _, item := range s.Items {
				if item.TextNone || item.Text != line[4:] {
					continue
				}
				s.Idx = Index(s.Items, item)
				s.Letter = line[0]
				if s.Counts != nil && s.Counts[s.Idx] != item.Count {
					s.PC = 2
					return call(a.Term.RawText(strconv.Itoa(s.Counts[s.Idx])))
				}
				s.PC = 3
				return call(a.Term.RawKey(world.Ch(s.Letter)))
			}
		}
		if len(s.Items) == 0 {
			s.PC = 4
			return call(a.Term.RawKey(world.Ch('\r')))
		}
		s.LineI++
	}
	s.PC = 5
	return call(a.Term.RawKey(world.Ch(' ')))
}
