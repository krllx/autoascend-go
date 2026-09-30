package assess

import (
	"math"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/world"
)

// Port of autoascend/item/inventory.py: strategy helpers and low-level strategies.

// ######## STRATEGIES helpers

// GetBestMeleeWeapon mirrors get_best_melee_weapon(items, allow_unknown_status=...); items == nil means None.
func GetBestMeleeWeapon(inv *world.Inventory, items []*world.Item, allowUnknownStatus bool) *world.Item {
	it, _, isNone := getBestMeleeWeapon(inv, items, allowUnknownStatus)
	if isNone {
		return nil
	}
	return it
}

// GetBestMeleeWeaponDPS mirrors get_best_melee_weapon(..., return_dps=True) (the monk case returns None, so
// unpacking it raises TypeError).
func GetBestMeleeWeaponDPS(inv *world.Inventory, items []*world.Item, allowUnknownStatus bool) (*world.Item, float64) {
	it, dps, isNone := getBestMeleeWeapon(inv, items, allowUnknownStatus)
	if isNone {
		Raise("TypeError", "cannot unpack non-iterable NoneType object")
	}
	return it, dps
}

func getBestMeleeWeapon(inv *world.Inventory, items []*world.Item, allowUnknownStatus bool) (*world.Item, float64, bool) {
	a := inv.Agent
	if a.Character.Role == world.Character_MONK {
		return nil, 0, true
	}

	if items == nil {
		items = inv.Items.AllItems
	}
	// select the best
	var bestItem *world.Item
	bestDps := CalcDPS(a.Character.GetMeleeBonus(nil, false))
	for item := range world.EachItem(items) {
		if item.IsWeapon() &&
			(item.Status == perception.Item_UNCURSED || item.Status == perception.Item_BLESSED ||
				(allowUnknownStatus && item.Status == perception.Item_UNKNOWN)) {
			toHit, dmg := a.Character.GetMeleeBonus(item, false)
			dps := CalcDPS(toHit, dmg)
			// dps = item.get_dps(large_monster=False)  # TODO: what about monster size
			if bestDps < dps {
				bestDps = dps
				bestItem = item
			}
		}
	}
	return bestItem, bestDps, false
}

type RangedCombo struct{ Launcher, Ammo *world.Item }

// GetRangedCombinations: items == nil means None (use inv.Items); additionalAmmo nil = [].
func GetRangedCombinations(inv *world.Inventory, items []*world.Item, throwing, allowBestMelee, allowWieldedMelee,
	allowUnknownStatus bool, additionalAmmo []*world.Item) []RangedCombo {
	if items == nil {
		items = inv.Items.AllItems
	}
	items = world.FlattenItems(items)
	var launchers, ammoList []*world.Item
	for _, i := range items {
		if i.IsLauncher() {
			launchers = append(launchers, i)
		}
	}
	for _, i := range items {
		if i.IsFiredProjectile(nil) {
			ammoList = append(ammoList, i)
		}
	}
	validCombinations := []RangedCombo{}

	// TODO: should this condition be used here
	anyCursed := false
	for _, l := range launchers {
		if l.Equipped && l.Status == perception.Item_CURSED {
			anyCursed = true
			break
		}
	}
	if anyCursed {
		var n []*world.Item
		for _, l := range launchers {
			if l.Equipped {
				n = append(n, l)
			}
		}
		launchers = n
	}

	for _, launcher := range launchers {
		for _, ammo := range append(append([]*world.Item{}, ammoList...), additionalAmmo...) {
			if ammo.IsFiredProjectile(launcher) {
				if launcher.Status == perception.Item_UNCURSED || launcher.Status == perception.Item_BLESSED ||
					(allowUnknownStatus && launcher.Status == perception.Item_UNKNOWN) {
					validCombinations = append(validCombinations, RangedCombo{launcher, ammo})
				}
			}
		}
	}

	if throwing {
		var bestMeleeWeapon *world.Item
		if !allowBestMelee {
			bestMeleeWeapon = GetBestMeleeWeapon(inv, nil, false)
		}
		var wieldedMeleeWeapon *world.Item
		if !allowWieldedMelee {
			wieldedMeleeWeapon = inv.Items.MainHand
		}
		for _, i := range items {
			if i.IsThrownProjectile() && i != bestMeleeWeapon && i != wieldedMeleeWeapon {
				validCombinations = append(validCombinations, RangedCombo{nil, i})
			}
		}
	}

	return validCombinations
}

// GetBestRangedSet: items == nil means None; additionalAmmo nil = [].
func GetBestRangedSet(inv *world.Inventory, items []*world.Item, throwing, allowBestMelee, allowWieldedMelee,
	allowUnknownStatus bool, additionalAmmo []*world.Item) (launcher, ammo *world.Item) {
	if items == nil {
		items = inv.Items.AllItems
	}
	items = world.FlattenItems(items)

	var bestLauncher, bestAmmo *world.Item
	bestDps := math.Inf(-1)
	for _, c := range GetRangedCombinations(inv, items, throwing, allowBestMelee, allowWieldedMelee,
		allowUnknownStatus, additionalAmmo) {
		toHit, dmg := inv.Agent.Character.GetRangedBonus(c.Launcher, c.Ammo, false)
		dps := CalcDPS(toHit, dmg)
		if dps > bestDps {
			bestLauncher, bestAmmo, bestDps = c.Launcher, c.Ammo, dps
		}
	}
	return bestLauncher, bestAmmo
}

// GetBestArmorset returns O_ARM_NUM slots (nil = None); items == nil means None.
func GetBestArmorset(inv *world.Inventory, items []*world.Item, allowUnknownStatus bool) []*world.Item {
	if items == nil {
		items = inv.Items.AllItems
	}
	items = world.FlattenItems(items)

	bestItems := make([]*world.Item, O_ARM_NUM)
	bestAc := make([]int, O_ARM_NUM)
	bestAcNone := make([]bool, O_ARM_NUM)
	for i := range bestAcNone {
		bestAcNone[i] = true
	}
	for _, item := range items {
		if !item.IsArmor() || !item.IsUnambiguous() {
			continue
		}

		// TODO: consider other always allowed items than dragon hide
		isDragonscaleArmor := item.Object().Metal == O_DRAGON_HIDE

		allowed := item.Status == perception.Item_UNCURSED || item.Status == perception.Item_BLESSED ||
			(allowUnknownStatus && item.Status == perception.Item_UNKNOWN)
		if !allowed && !isDragonscaleArmor {
			continue
		}

		slot := item.Object().Sub
		ac := item.GetAc()

		if inv.Agent.Character.Role == world.Character_MONK && slot == O_ARM_SUIT {
			continue
		}

		s := PyIdx(slot, O_ARM_NUM)
		if bestAcNone[s] || bestAc[s] > ac {
			bestAc[s], bestAcNone[s] = ac, false
			bestItems[s] = item
		}
	}

	return bestItems
}

// sumCols mirrors np.array(list_of_equal_length_lists).sum(0).
func SumCols(rows [][]int) []int {
	if len(rows) == 0 {
		Raise("ValueError", "empty")
	}
	n := len(rows[0])
	for _, r := range rows {
		if len(r) != n {
			Raise("ValueError", "setting an array element with a sequence. The requested array has an inhomogeneous shape")
		}
	}
	ret := make([]int, n)
	for _, r := range rows {
		for i, v := range r {
			ret[i] += v
		}
	}
	return ret
}
