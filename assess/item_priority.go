package assess

import (
	"sort"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/world"
)

// Port of global_logic.py ItemPriority and item/item_priority_base.py ItemPriorityBase.split.
//
// ItemPriority decides what the agent keeps in its inventory: split_ walks the items it could carry
// in priority order (bag, gold, best weapon and armor, healing potions, Excalibur candidate, missiles,
// food, sacrificial corpses, then light unidentified items) and takes each while the weight allows.

const ItemPriority_MAX_NUMBER_OF_ITEMS = 26*2 - 1

// logicSortedItems mirrors sorted(items, key=key) with float keys: keys are computed in list order,
// then a stable sort is applied.
func logicSortedItems(items []*world.Item, key func(*world.Item) float64) []*world.Item {
	keys := make([]float64, len(items))
	for i, it := range items {
		keys[i] = key(it)
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	r := make([]*world.Item, len(items))
	for i, j := range idx {
		r[i] = items[j]
	}
	return r
}

func logicObjName(o *Obj, name string) bool { return !o.NameNone && o.Name == name }

// Split mirrors ItemPriorityBase.split.
func Split(p *world.ItemPriority, items, forcedItems []*world.Item, weightCapacity float64) world.SplitResult {
	ret := split_(p, items, forcedItems, weightCapacity)
	Assert(ret.Has(nil))
	// counts = np.array(list(ret.values())).sum(0)
	counts := make([]int, len(items))
	for _, cs := range ret.Values() {
		for i := range items {
			counts[i] += cs[i]
		}
	}
	for i, item := range items {
		if !(0 <= counts[i] && counts[i] <= item.Count) {
			Assert(false)
		}
	}
	for _, cs := range ret.Values() {
		for i, item := range items {
			if i >= len(cs) {
				break
			}
			if !(0 <= cs[i] && cs[i] <= item.Count) {
				Assert(false)
			}
		}
	}
	for _, item := range items {
		if !(!ret.Has(item) || item.IsContainer()) {
			Assert(false)
		}
	}
	for i, item := range items {
		if !(!ret.Has(item) || ret.MustGet(item)[i] == 0) {
			Assert(false)
		}
	}
	return ret
}

// split_ mirrors ItemPriority._split.
func split_(p *world.ItemPriority, items, forcedItems []*world.Item, weightCapacity float64) world.SplitResult {
	a := p.Agent
	remainingWeight := weightCapacity
	retInv := NewOMap[*world.Item, int]()
	for _, item := range forcedItems {
		remainingWeight -= float64(item.Weight(true)) // conversion prevents FMA fusion
		retInv.Set(item, item.Count)
	}

	retBag := NewOMap[*world.Item, int]()
	var bag *world.Item

	addItem := func(item *world.Item, count *int, toBag bool) {
		Assert(item != nil)
		var ret *OMap[*world.Item, int]
		if toBag && bag != nil && item != bag {
			ret = retBag
		} else {
			ret = retInv
		}

		if remainingWeight < 0 ||
			(ret == retInv && ret.Len() >= ItemPriority_MAX_NUMBER_OF_ITEMS) {
			return
		}

		howManyAlreadyTotal := retInv.GetOr(item, 0) + retBag.GetOr(item, 0)
		howManyAlready := ret.GetOr(item, 0)
		maxToAdd := int(FloorDivF(remainingWeight, item.UnitWeight(false)))
		if count != nil {
			maxToAdd = min(maxToAdd, *count)
		}
		ret.Set(item, min(item.Count, howManyAlreadyTotal+maxToAdd)-(howManyAlreadyTotal-howManyAlready))
		remainingWeight -= float64(item.UnitWeight(false) * float64(ret.MustGet(item)-howManyAlready)) // no FMA
	}

	for _, item := range items {
		if item.IsContainer() && (item.Status == perception.Item_UNCURSED || item.Status == perception.Item_BLESSED) &&
			!item.Objs[0].DescNone && item.Objs[0].Desc == "bag" {
			bag = item // TODO: select the best
			addItem(bag, nil, false)
		}
	}

	if p.DropGoldTillTurn < float64(a.Blstats.Time) {
		for _, item := range items {
			if item.Category == NH_COIN_CLASS {
				addItem(item, nil, false)
			}
		}
	}

	for _, allowUnknownStatus := range []bool{false, true} {
		all := make([]*world.Item, 0, len(forcedItems)+len(items))
		all = append(all, forcedItems...)
		all = append(all, items...)
		item := GetBestMeleeWeapon(a.Inventory, all, allowUnknownStatus)
		if item != nil {
			addItem(item, nil, false)
		}

		all2 := make([]*world.Item, 0, len(forcedItems)+len(items))
		all2 = append(all2, forcedItems...)
		all2 = append(all2, items...)
		for _, item := range GetBestArmorset(a.Inventory, all2, allowUnknownStatus) {
			if item != nil {
				addItem(item, nil, false)
			}
		}
	}

	for _, item := range items {
		if item.IsUnambiguous() {
			obj := item.Object()
			if obj == potionHealing || obj == potionExtraHealing || obj == potionFullHealing {
				addItem(item, nil, false)
			}

			role := a.Character.Role
			if (role == world.Character_RANGER || role == world.Character_ROGUE ||
				role == world.Character_SAMURAI || role == world.Character_TOURIST) &&
				(item.IsLauncher() || item.IsFiredProjectile(nil)) {
				addItem(item, nil, false)
			}
		}
	}

	if a.Character.Alignment == world.Character_LAWFUL {
		longSwords := Filter(items, func(i *world.Item) bool { return logicObjName(i.Objs[0], "long sword") })
		for _, item := range logicSortedItems(longSwords, func(i *world.Item) float64 {
			return -CalcDPS(a.Character.GetMeleeBonus(i, false))
		}) {
			addItem(item, nil, false) // only the best one
			break
		}
	}

	projectiles := Filter(items, func(i *world.Item) bool { return i.IsThrownProjectile() })
	for _, item := range logicSortedItems(projectiles, func(i *world.Item) float64 {
		return -CalcDPS(a.Character.GetRangedBonus(nil, i, false))
	}) {
		addItem(item, nil, false)
	}

	food := Filter(items, func(i *world.Item) bool { return i.IsFood() && !i.IsCorpse() })
	for _, item := range logicSortedItems(food, func(x *world.Item) float64 {
		return -x.NutritionPerWeight() - float64(1000*float64(B2i(logicObjName(x.Objs[0], "sprig of wolfsbane"))))
	}) {
		addItem(item, nil, false)
	}

	if p.TakeSacrificialCorpses {
		for _, item := range items {
			if a.GlobalLogic.CanSacrify(item) {
				addItem(item, nil, false)
			}
		}
	}

	// TODO: take nh.COIN_CLASS once shopping is implemented.
	// You have to drop all coins not to be attacked by a vault guard

	for _, item := range logicSortedItems(items, func(i *world.Item) float64 { return i.UnitWeight(false) }) {
		c := item.Category
		if c == NH_POTION_CLASS || c == NH_RING_CLASS || c == NH_AMULET_CLASS || c == NH_WAND_CLASS ||
			c == NH_SCROLL_CLASS || c == NH_TOOL_CLASS {
			if (item.Objs[0].Kind != KContainer || !item.IsChest()) && !item.IsPossibleContainer() {
				toBag := !ContainsObj(item.Objs, wandCancellation) &&
					!item.IsOffensiveUsableWand()
				toBag = false
				if !item.IsContainer() { // remove condition to pick up bags
					addItem(item, nil, toBag)
				}
			}
		}
	}

	categories := []int{NH_WEAPON_CLASS, NH_ARMOR_CLASS, NH_TOOL_CLASS, NH_FOOD_CLASS, NH_GEM_CLASS, NH_AMULET_CLASS,
		NH_RING_CLASS, NH_POTION_CLASS, NH_SCROLL_CLASS, NH_SPBOOK_CLASS, NH_WAND_CLASS}
	for _, item := range logicSortedItems(items, func(i *world.Item) float64 { return i.UnitWeight(false) }) {
		if Contains(categories, item.Category) && item.Objs[0].Kind != KContainer && !item.IsCorpse() {
			if item.Status == perception.Item_UNKNOWN {
				toBag := !ContainsObj(item.Objs, wandCancellation) &&
					!item.IsOffensiveUsableWand()
				toBag = false
				if !item.IsContainer() { // remove condition to pick up bags
					addItem(item, nil, toBag)
				}
			}
		}
	}

	r := NewOMap[*world.Item, []int]()
	inv := make([]int, len(items))
	for i, item := range items {
		inv[i] = retInv.GetOr(item, 0)
	}
	r.Set(nil, inv)
	if bag != nil {
		b := make([]int, len(items))
		for i, item := range items {
			b[i] = retBag.GetOr(item, 0)
		}
		r.Set(bag, b)
	}
	for _, item := range items {
		if item.IsContainer() && item != bag && retInv.GetOr(item, 0) != 0 {
			r.Set(item, make([]int, len(items)))
		}
	}
	return r
}

// Objects compared by name in the split, looked up once.
var (
	potionHealing      = O_FromName("healing", NH_POTION_CLASS)
	potionExtraHealing = O_FromName("extra healing", NH_POTION_CLASS)
	potionFullHealing  = O_FromName("full healing", NH_POTION_CLASS)
	wandCancellation   = O_FromName("cancellation", NH_WAND_CLASS)
)
