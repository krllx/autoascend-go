package sense

import (
	"fmt"
	"regexp"
	"strings"

	"aago/flow"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/world"
)

var (
	severalObjectsRe = regexp.MustCompile(`There are (several|many) objects here\.`)
	liftingRe        = regexp.MustCompile(`You have [a-z ]+ lifting (.+)\.\s+Continue\?`)
	pickedUpRe       = regexp.MustCompile(`^[a-zA-Z$] - `)
)

// ---- inventory: what lies below me (':' and the pick up menu)

type lookBelowF struct {
	Assume  bool
	Lifting string // the item ',' tried to lift without a menu
	PC      int
}

func GetItemsBelowMe(assume bool) Frame { return flow.PosGuard(Atom(&lookBelowF{Assume: assume})) }

func (s *lookBelowF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		s.PC = 1
		if !s.Assume {
			return call(a.Term.Key(A_Command_LOOK))
		} else if Contains(a.Popup, "Things that are here:") || Search(severalObjectsRe, a.Message) != nil {
			// LOOK is necessary even when 'Things that are here' popup is present for some very rare cases
			return call(a.Term.Key(A_Command_LOOK))
		}
		fallthrough
	case 1:
		if strings.Contains(a.Message, "Something is ") && strings.Contains(a.Message, `You read: "`) {
			index := strings.Index(a.Message, `You read: "`) + len(`You read: "`)
			Assert(strings.Contains(a.Message[index:], `"`))
			engraving := a.Message[index : index+strings.Index(a.Message[index:], `"`)]
			inv.EngravingBelowMe, inv.EngravingBelowMeNone = engraving, false
		} else {
			inv.EngravingBelowMe, inv.EngravingBelowMeNone = "", false
		}

		if !Contains(a.Popup, "Things that are here:") && !strings.Contains(strings.Join(a.Popup, "\n"), "There is ") {
			var items []*world.Item
			var letters []string
			if strings.Contains(a.Message, "You see no objects here.") {
				items = []*world.Item{}
				letters = []string{}
			} else if strings.Contains(a.Message, "You see here ") {
				itemStr := a.Message[strings.Index(a.Message, "You see here ")+len("You see here "):]
				dot := strings.Index(itemStr, ".")
				if dot < 0 {
					Raise("ValueError", "substring not found")
				}
				itemStr = itemStr[:dot]
				items = []*world.Item{inv.ItemManager.GetItemFromText(itemStr, -1, -1, inv.CurrentPositionID())}
				letters = []string{""} // None
			} else {
				items = []*world.Item{}
				letters = []string{}
			}
			return s.done(inv, items, letters)
		}
		s.PC = 2
		return call(a.Term.Key(A_Command_PICKUP)) // FIXME: parse LOOK output, add this fragment to pickup method
	case 2:
		// Not in the Python bot. When punished, the attached iron chain lies here too: LOOK lists it, so ','
		// is pressed, but ',' leaves the chain out and takes the one other object without a menu. A heavy one
		// (the ball) asks first: decline, it is the only item here. A light one is picked up: nothing is left.
		if !Contains(a.Popup, "Pick up what?") {
			if m := Search(liftingRe, a.Message); m != nil {
				s.Lifting = m[1]
				s.PC = 3
				return call(TypeText("n"))
			}
			if pickedUpRe.MatchString(a.Message) {
				return s.done(inv, []*world.Item{}, []string{})
			}
		}
	case 3:
		item := inv.ItemManager.GetItemFromText(s.Lifting, -1, -1, inv.CurrentPositionID())
		return s.done(inv, []*world.Item{item}, []string{""}) // letter None: ',' takes it without a menu
	}
	var items []*world.Item
	var letters []string
	if !Contains(a.Popup, "Pick up what?") {
		m := a.Message
		if strings.Contains(m, "You cannot reach the bottom of the pit.") ||
			strings.Contains(m, "You cannot reach the bottom of the abyss.") ||
			strings.Contains(m, "You cannot reach the floor.") ||
			strings.Contains(m, "There is nothing here to pick up.") ||
			strings.Contains(m, " solidly fixed to the floor.") ||
			strings.Contains(m, "You read:") ||
			strings.Contains(m, "You don't see anything in here to pick up.") ||
			strings.Contains(m, "You cannot reach the ground.") ||
			strings.Contains(m, "You don't feel anything in here to pick up.") {
			items = []*world.Item{}
			letters = []string{}
		} else {
			Assert(false, a.Message, a.Popup)
		}
	} else {
		popup := a.Popup
		lines := popup[Index(popup, "Pick up what?")+1:]
		category := -1
		items = []*world.Item{}
		letters = []string{}
		for _, line := range lines {
			if cat, ok := perception.InventoryNameToCategory[line]; ok {
				category = cat
				continue
			}
			Assert(world.PySlice(line, 1, 4) == " - ", line)
			letter, rest := line[:1], line[4:]
			letters = append(letters, letter)
			items = append(items, inv.ItemManager.GetItemFromText(rest, category, -1, inv.CurrentPositionID()))
		}
	}
	return s.done(inv, items, letters)
}

func (s *lookBelowF) done(inv *world.Inventory, items []*world.Item, letters []string) Cmd {
	inv.ItemsBelowMe, inv.ItemsBelowMeNone = items, false
	inv.LettersBelowMe, inv.LettersBelowMeNone = letters, false
	return ret(nil)
}

// itemsBelowWatch is panic_if_items_below_me_change's callback: AgentPanic when the (letter, text) pairs of
// the items below me differ from the ones saved at entry.
type itemsBelowWatch struct {
	OldItems       []*world.Item
	OldItemsNone   bool
	OldLetters     []string
	OldLettersNone bool
}

func (w *itemsBelowWatch) Fire(a *world.Agent) {
	inv := a.Inventory
	pairs := func(items []*world.Item, itemsNone bool, letters []string, lettersNone bool) []string {
		if itemsNone || lettersNone {
			Raise("TypeError", "'NoneType' object is not iterable")
		}
		n := min(len(items), len(letters))
		r := make([]string, n)
		for i := 0; i < n; i++ {
			r[i] = fmt.Sprintf("%q\x00%v\x00%q", letters[i], items[i].TextNone, items[i].Text)
		}
		return r
	}
	o := pairs(w.OldItems, w.OldItemsNone, w.OldLetters, w.OldLettersNone)
	cur := pairs(inv.ItemsBelowMe, inv.ItemsBelowMeNone, inv.LettersBelowMe, inv.LettersBelowMeNone)
	if strings.Join(o, "\x01") != strings.Join(cur, "\x01") || len(o) != len(cur) {
		Panic("items below me changed")
	}
}

// itemsGuardF is `with inventory.panic_if_items_below_me_change(): body`.
type itemsGuardF struct {
	Body  Frame
	Watch world.Watch
	PC    int
}

func ItemsGuard(body Frame) Frame { return &itemsGuardF{Body: body} }

func (s *itemsGuardF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		inv := a.Inventory
		s.Watch = &itemsBelowWatch{inv.ItemsBelowMe, inv.ItemsBelowMeNone, inv.LettersBelowMe, inv.LettersBelowMeNone}
		a.OnUpdate = append(a.OnUpdate, s.Watch)
		s.PC = 1
		return call(s.Body)
	}
	s.exit(a)
	return ret(c.Ret)
}

func (s *itemsGuardF) Unwind(c *Ctx) {
	if s.PC == 1 {
		s.exit(c.W)
	}
}

func (s *itemsGuardF) exit(a *world.Agent) {
	Assert(Contains(a.OnUpdate, s.Watch))
	i := Index(a.OnUpdate, s.Watch)
	a.OnUpdate = append(a.OnUpdate[:i:i], a.OnUpdate[i+1:]...)
}
