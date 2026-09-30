package tactics

import (
	"aago/assess"
	"aago/flow"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/skills"
	"aago/world"
)

// ---- wear_best_stuff

type wearBestStuffN struct{}

var armorSlots = []int{O_ARM_SHIELD, O_ARM_HELM, O_ARM_GLOVES, O_ARM_BOOTS, O_ARM_SHIRT, O_ARM_SUIT, O_ARM_CLOAK}

func slotItem(ii *world.InventoryItems, slot int) *world.Item {
	switch slot {
	case O_ARM_SHIELD:
		return ii.OffHand
	case O_ARM_HELM:
		return ii.Helm
	case O_ARM_GLOVES:
		return ii.Gloves
	case O_ARM_BOOTS:
		return ii.Boots
	case O_ARM_SHIRT:
		return ii.Shirt
	case O_ARM_SUIT:
		return ii.Suit
	case O_ARM_CLOAK:
		return ii.Cloak
	}
	panic("bad slot")
}

// wearStep is one `while 1` iteration of wear_best_stuff up to the action; nil means the for-else break.
func wearStep(a *world.Agent) Frame {
	inv := a.Inventory
	ii := inv.Items
	bestArmorset := assess.GetBestArmorset(inv, nil, false)

	// TODO: twoweapon
	for _, slot := range armorSlots {
		cur := slotItem(ii, slot)
		if bestArmorset[slot] == cur || (cur != nil && cur.Status == perception.Item_CURSED) {
			continue
		}
		if slot == O_ARM_BOOTS && a.FeetStuckTurn == a.Blstats.Time {
			continue // not in the Python bot: boots on or off were refused this turn (see Agent.FeetStuckTurn)
		}
		additionalCond := true
		if slot == O_ARM_SHIELD {
			additionalCond = additionalCond && (ii.MainHand == nil || world.ObjBi(ii.MainHand.Objs[0]) == 0)
		}
		if slot == O_ARM_GLOVES {
			additionalCond = additionalCond && (ii.MainHand == nil || ii.MainHand.Status != perception.Item_CURSED)
		}
		if slot == O_ARM_SHIRT || slot == O_ARM_SUIT {
			additionalCond = additionalCond && (ii.Cloak == nil || ii.Cloak.Status != perception.Item_CURSED)
		}
		if slot == O_ARM_SHIRT {
			additionalCond = additionalCond && (ii.Suit == nil || ii.Suit.Status != perception.Item_CURSED)
		}
		if additionalCond {
			return &wearSlotF{Slot: slot, Best: bestArmorset[slot]}
		}
	}
	return nil
}

// wearSlotF takes off what is in the way of the best item for Slot, or wears it.
type wearSlotF struct {
	Slot int
	Best *world.Item
	PC   int
}

func (s *wearSlotF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	s.PC = 1
	inv := c.W.Inventory
	ii := inv.Items
	slot := s.Slot
	if (slot == O_ARM_SHIRT || slot == O_ARM_SUIT) && ii.Cloak != nil {
		return call(skills.Takeoff(ii.Cloak))
	}
	if slot == O_ARM_SHIRT && ii.Suit != nil {
		return call(skills.Takeoff(ii.Suit))
	}
	if cur := slotItem(ii, slot); cur != nil {
		return call(skills.Takeoff(cur))
	}
	Assert(s.Best != nil)
	return call(skills.Wear(s.Best, true))
}

func (wearBestStuffN) Check(a *world.Agent) (bool, Frame) {
	act := wearStep(a)
	if act == nil {
		return false, nil
	}
	return true, flow.LoopFrom(wearStepper{}, act)
}

// wearStepper and arrangeStepper are the loops of wear_best_stuff and arrange_items (see loopF).
type wearStepper struct{}

func (wearStepper) Step(a *world.Agent) Frame { return wearStep(a) }
