package sense

import (
	"bytes"
	"sort"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// updateStateF is agent.update_state(): run the six state updaters in order (three of them may press keys),
// restoring the message and popup after each, then the on_update callbacks.
type updateStateF struct {
	AllowUpdate, AllowCallbacks bool
	Message                     string
	Popup                       []string
	I                           int  // next updater
	Restore                     bool // an updater child has just returned
	PC                          int  // 0: enter, 1: running (isUpdatingState set)
}

func UpdateState(allowUpdate, allowCallbacks bool) Frame {
	return &updateStateF{AllowUpdate: allowUpdate, AllowCallbacks: allowCallbacks}
}

func (s *updateStateF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		Assert(!a.NoStepCalls)
		if a.IsUpdatingState {
			return ret(nil)
		}
		a.IsUpdatingState = true
		s.Message, s.Popup = a.Message, a.Popup
		s.PC = 1
	}
	if s.AllowUpdate {
		for {
			if s.Restore {
				a.Message, a.Popup = s.Message, s.Popup
				s.Restore = false
			}
			if s.I == 6 {
				break
			}
			s.I++
			var child Frame
			switch s.I {
			case 1:
				a.Character.Update()
			case 2:
				child = &inventoryUpdateF{}
			case 3:
				child = &monsterUpdateF{}
			case 4:
				child = CheckTerrain(false)
			case 5:
				a.UpdateLevel()
			case 6:
				a.GlobalLogic.Update()
			}
			s.Restore = true
			if child != nil {
				return call(child)
			}
		}
	}
	if s.AllowCallbacks {
		a.CallAllUpdateFunctions()
	}
	a.IsUpdatingState = false
	return ret(nil)
}

func (s *updateStateF) Unwind(c *Ctx) {
	if s.PC == 1 {
		c.W.IsUpdatingState = false
	}
}

// inventoryUpdateF is Inventory.update: discoveries every 200 steps, the inventory list, and what lies
// below me after moving (or while standing on Elbereth).
type inventoryUpdateF struct{ PC int }

func (s *inventoryUpdateF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		s.PC = 1
		m := inv.ItemManager
		if m.LastObjectGlyphMappingUpdateNone || m.LastObjectGlyphMappingUpdateStep+200 < a.StepCount {
			return call(UpdateObjectGlyphMapping())
		}
		fallthrough
	case 1:
		s.PC = 2
		return call(UpdateItems(false))
	case 2:
		b := a.Blstats
		p := inv.PreviousBlstats
		if inv.PreviousBlstatsNone ||
			p.Y != b.Y || p.X != b.X || p.LevelNumber != b.LevelNumber || p.DungeonNumber != b.DungeonNumber ||
			(inv.EngravingBelowMeNone || strings.ToLower(inv.EngravingBelowMe) == "elbereth") {
			assumeAppropriateMessage := !inv.PreviousBlstatsNone && (inv.EngravingBelowMeNone || inv.EngravingBelowMe == "")

			inv.PreviousBlstats = a.Blstats
			inv.PreviousBlstatsNone = false
			inv.ItemsBelowMe, inv.ItemsBelowMeNone = nil, true
			inv.LettersBelowMe, inv.LettersBelowMeNone = nil, true
			inv.EngravingBelowMe, inv.EngravingBelowMeNone = "", true

			s.PC = 3
			return call(GetItemsBelowMe(assumeAppropriateMessage))
		}
	}
	Assert(!inv.ItemsBelowMeNone && !inv.LettersBelowMeNone && !inv.EngravingBelowMeNone)
	return ret(nil)
}

// itemsUpdateF is InventoryItems.update(force): rebuild the inventory from the observation when it changed,
// looking into containers on the way; if the inventory changes meanwhile, start over (a recursive call).
type itemsUpdateF struct {
	Force    bool
	Prev     [55][80]uint8 // inv_strs the list is built from
	Iterable []world.InvEntry
	I        int
	Item     *world.Item // the item being added
	PC       int         // 0: start, 1: next entry, 2: container checked, 3: restarted
}

func UpdateItems(force bool) Frame { return &itemsUpdateF{Force: force} }

func (s *itemsUpdateF) Resume(c *Ctx) Cmd {
	a := c.W
	ii := a.Inventory.Items
	switch s.PC {
	case 0:
		if s.Force {
			ii.RecheckContainers = true
		}
		lo := a.LastObservation
		if !(s.Force || ii.PreviousInvStrsNone || lo.InvStrs != ii.PreviousInvStrs) {
			return ret(nil)
		}
		ii.Clear()
		ii.PreviousInvStrs = lo.InvStrs
		ii.PreviousInvStrsNone = false
		s.Prev = ii.PreviousInvStrs

		// For some reasons sometime the inventory entries in last_observation may be duplicated
		seen := map[world.InvEntry]bool{}
		var iterable []world.InvEntry
		for i := 0; i < len(lo.InvStrs); i++ {
			itemName := string(bytes.Trim(lo.InvStrs[i][:], "\x00"))
			e := world.InvEntry{itemName, int(lo.InvOclasses[i]), int(lo.InvGlyphs[i]), lo.InvLetters[i]}
			if itemName == "" {
				continue
			}
			if !seen[e] {
				seen[e] = true
				iterable = append(iterable, e)
			}
		}
		sort.SliceStable(iterable, func(i, j int) bool { return iterable[i].Letter < iterable[j].Letter })

		letters := map[byte]bool{}
		for _, e := range iterable {
			letters[e.Letter] = true
		}
		Assert(len(iterable) == len(letters), "letters in inventory are not unique")
		s.Iterable = iterable
		s.PC = 1
	case 2:
		if cmd, done := s.afterContainer(a); done {
			return cmd
		}
	case 3:
		return ret(nil)
	}
	for s.I < len(s.Iterable) {
		e := s.Iterable[s.I]
		glyph := e.Glyph
		if GlyphIsBody(glyph) || GlyphIsStatue(glyph) {
			glyph = -1
		}
		item := a.Inventory.ItemManager.GetItemFromText(e.Name, e.Category, glyph, nil)
		s.Item = item

		ii.AllItems = append(ii.AllItems, item)
		ii.AllLetters = append(ii.AllLetters, e.Letter)

		if item.Equipped {
			equip(ii, item)
		}

		if item.IsPossibleContainer() || (item.IsContainer() && ii.RecheckContainers) {
			s.PC = 2
			return call(CheckContainerContent(item))
		}
		if cmd, done := s.afterContainer(a); done {
			return cmd
		}
	}
	ii.RecheckContainers = false
	return ret(nil)
}

// afterContainer finishes the current entry: restart if the inventory changed, else count its weight.
func (s *itemsUpdateF) afterContainer(a *world.Agent) (Cmd, bool) {
	ii := a.Inventory.Items
	if a.LastObservation.InvStrs != s.Prev {
		s.PC = 3
		return call(UpdateItems(false)), true
	}
	ii.TotalWeight += s.Item.Weight(true)
	// weight is sometimes unambiguous for unidentified items. All exceptions:
	// {'helmet': 30, 'helm of brilliance': 50, 'helm of opposite alignment': 50, 'helm of telepathy': 50}
	// {'leather gloves': 10, 'gauntlets of fumbling': 10, 'gauntlets of power': 30, 'gauntlets of dexterity': 10}
	// {'speed boots': 20, 'water walking boots': 15, 'jumping boots': 20, 'elven boots': 15, 'fumble boots': 20, 'levitation boots': 15}
	// {'luckstone': 10, 'loadstone': 500, 'touchstone': 10, 'flint': 10}
	s.I++
	s.PC = 1
	return Cmd{}, false
}

// equip records an equipped item in its slot.
func equip(ii *world.InventoryItems, item *world.Item) {
	o := item.Objs[0]
	type slotDef struct {
		weapon bool // (O.Weapon, O.WepTool), sub None
		sub    int
		ptr    **world.Item
		name   string
	}
	for _, sl := range []slotDef{
		{true, 0, &ii.MainHand, "main_hand"},
		{false, O_ARM_SHIELD, &ii.OffHand, "off_hand"}, // TODO: twoweapon support
		{false, O_ARM_SUIT, &ii.Suit, "suit"},
		{false, O_ARM_HELM, &ii.Helm, "helm"},
		{false, O_ARM_GLOVES, &ii.Gloves, "gloves"},
		{false, O_ARM_BOOTS, &ii.Boots, "boots"},
		{false, O_ARM_CLOAK, &ii.Cloak, "cloak"},
		{false, O_ARM_SHIRT, &ii.Shirt, "shirt"},
	} {
		var match bool
		if sl.weapon {
			match = o.Kind == KWeapon || o.Kind == KWepTool
		} else {
			match = o.Kind == KArmor && sl.sub == o.Sub
		}
		if match {
			Assert(*sl.ptr == nil, sl.name)
			*sl.ptr = item
			break
		}
	}
}
