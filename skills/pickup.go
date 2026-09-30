package skills

import (
	"fmt"
	"regexp"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

var liftingRe = regexp.MustCompile(`You have [a-z ]+ lifting `)

var pickedLetterRe = regexp.MustCompile(`([a-zA-Z$]) - `)

// removeFirstStr mirrors list.remove(x) (ValueError if missing).
func removeFirstStr(xs []string, x string) []string {
	i := Index(xs, x)
	return append(xs[:i:i], xs[i+1:]...)
}

// Moving items between the floor, containers and the inventory (inventory.py) as frames.

// checkCounts mirrors the asserts shared by pickup and drop; nil counts become the item counts.
func checkCounts(items []*world.Item, counts []int) []int {
	if counts == nil {
		counts = make([]int, len(items))
		for i, it := range items {
			counts[i] = it.Count
		}
	}
	return counts
}

func assertCounts(items []*world.Item, counts []int) {
	Assert(len(counts) == len(items))
	sum := 0
	for _, c := range counts {
		sum += c
	}
	okCounts := true
	for i := 0; i < min(len(counts), len(items)); i++ {
		if !(0 <= counts[i] && counts[i] <= items[i].Count) {
			okCounts = false
			break
		}
	}
	Assert(sum > 0 && okCounts)
}

// ---- pickup

// pickupF is pickup(items, counts); counts nil means None.
type pickupF struct {
	Items     []*world.Item
	Counts    []int
	Letters   []string
	Screens   []int
	OneItem   bool
	DropCount int
	Letter    string
	PC        int
}

func Pickup(items []*world.Item, counts []int) Frame { return &pickupF{Items: items, Counts: counts} }

func (s *pickupF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		// TODO: if polyphormed, sometimes 'You are physically incapable of picking anything up.'
		items := s.Items
		s.Counts = checkCounts(items, s.Counts)
		Assert(len(items) > 0)
		for _, item := range items {
			Assert(inv.InBelowMe(item))
		}
		assertCounts(items, s.Counts)

		if inv.LettersBelowMeNone {
			Raise("TypeError", "'NoneType' object is not subscriptable")
		}
		s.Letters = make([]string, len(items))
		for i, item := range items {
			s.Letters[i] = inv.LettersBelowMe[Index(inv.ItemsBelowMe, item)]
		}
		s.Screens = make([]int, len(items))
		for i, item := range items {
			idx := Index(inv.ItemsBelowMe, item)
			n := 0
			for _, l := range inv.LettersBelowMe[:min(idx+1, len(inv.LettersBelowMe))] {
				if l == "a" {
					n++
				}
			}
			s.Screens[i] = max(n-1, 0)
		}
		s.PC = 1
		return call(sense.ItemsGuard(sense.GetItemsBelowMe(false)))
	case 1:
		s.OneItem = len(inv.ItemsBelowMe) == 1
		s.PC = 2
		return call(sense.Atom(&pickupBodyF{P: s}))
	case 2:
		if s.OneItem && s.DropCount != 0 {
			idx := Index(inv.Items.AllLetters, s.Letter[0])
			s.PC = 3
			return call(Drop([]*world.Item{inv.Items.AllItems[idx]}, []int{s.DropCount}, false))
		}
		fallthrough
	case 3:
		s.PC = 4
		return call(sense.GetItemsBelowMe(false))
	}
	return ret(nil)
}

// pickupBodyF is the atom operation of pickup; it fills P.DropCount and P.Letter.
type pickupBodyF struct {
	P  *pickupF
	PC int
}

func (s *pickupBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	p := s.P
	switch s.PC {
	case 0:
		s.PC = 1
		if p.OneItem {
			for _, sc := range p.Screens {
				Assert(sc == 0)
			}
			s.PC = 2
			return call(a.Term.Key(A_Command_PICKUP))
		}
		maxScreen := p.Screens[0]
		for _, sc := range p.Screens[1:] {
			if sc > maxScreen {
				maxScreen = sc
			}
		}
		var parts []string
		for currentScreen := 0; currentScreen < maxScreen+1; currentScreen++ {
			var sb strings.Builder
			for j := 0; j < min(len(p.Letters), len(p.Items), len(p.Counts), len(p.Screens)); j++ {
				item, count, screen := p.Items[j], p.Counts[j], p.Screens[j]
				if count != 0 && screen == currentScreen {
					if item.Count != count {
						sb.WriteString(fmt.Sprint(count))
					}
					if p.Letters[j] == "" {
						Raise("TypeError", "can only concatenate str (not \"NoneType\") to str")
					}
					sb.WriteString(p.Letters[j])
				}
			}
			parts = append(parts, sb.String())
		}
		d := a.Term.Dialog(A_Command_PICKUP, &textThenMoreGen{Text: strings.Join(parts, " ")})
		return call(d)
	case 2:
		p.DropCount = p.Items[0].Count - p.Counts[0]
		s.PC = 1
	}
	// PC 1: after the pickup command or a "Continue?" answer
	if Search(liftingRe, a.Message) != nil && strings.Contains(a.Message, "Continue?") {
		return call(sense.TypeText("y"))
	}
	if p.OneItem && p.DropCount != 0 {
		m := Search(pickedLetterRe, a.Message)
		Assert(m != nil, a.Message)
		p.Letter = m[1]
	}
	return ret(nil)
}

// textThenMoreGen types Text in the dialog and confirms with MORE.
type textThenMoreGen struct {
	Text string
	PC   int
}

func (g *textThenMoreGen) Resume(c *Ctx) Cmd {
	switch g.PC {
	case 0:
		g.PC = 1
		return call(c.W.Term.RawText(g.Text))
	case 1:
		g.PC = 2
		return call(c.W.Term.RawKey(A_MiscAction_MORE))
	}
	return ret(nil)
}
