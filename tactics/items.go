package tactics

import (
	"sort"

	"aago/assess"
	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// randomTarget mirrors: nonzero(mask & (dis == dis[mask].min())); i = rng.randint(len); target.
func randomTarget(inv *world.Inventory, mask *BoolGrid, dis *I32Grid) (int, int) {
	first := true
	var mn int32
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			if mask[y][x] && (first || dis[y][x] < mn) {
				mn = dis[y][x]
				first = false
			}
		}
	}
	if first {
		Raise("ValueError", "zero-size array to reduction operation minimum which has no identity")
	}
	var m BoolGrid
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			m[y][x] = mask[y][x] && dis[y][x] == mn
		}
	}
	nz := m.Nonzero()
	i := inv.Agent.Rng.RandInt1(len(nz))
	return nz[i].Y, nz[i].X
}

func goToDefaultOpts() skills.GoToOpts {
	return skills.GoToOpts{StopOneBefore: false, MaxSteps: 0, Callback: nil, Fast: false} // callback: lambda: False
}

// belowMeLen mirrors len(self.items_below_me) (TypeError when None).
func belowMeLen(inv *world.Inventory) []*world.Item {
	if inv.ItemsBelowMeNone {
		Raise("TypeError", "object of type 'NoneType' has no len()")
	}
	return inv.ItemsBelowMe
}

// ---- walking to items and containers

// goToRandomTargetF picks a random closest cell of Mask and walks there.
type goToRandomTargetF struct {
	Mask BoolGrid
	Dis  I32Grid
	PC   int
}

func (s *goToRandomTargetF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	s.PC = 1
	ty, tx := randomTarget(c.W.Inventory, &s.Mask, &s.Dis)
	return call(skills.GoTo(ty, tx, goToDefaultOpts()))
}

type checkItemsN struct{}

func (checkItemsN) Check(a *world.Agent) (bool, Frame) {
	mask := Isin(a.Glyphs(), G_OBJECTS, G_BODIES, G_STATUES)
	if !mask.Any() {
		return false, nil
	}
	dis := a.Bfs()
	lvl := a.CurrentLevel()
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			mask[y][x] = mask[y][x] && lvl.ItemCount[y][x] == 0
		}
	}
	if !mask.Any() {
		return false, nil
	}
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			mask[y][x] = mask[y][x] && dis[y][x] > 0
		}
	}
	if !mask.Any() {
		return false, nil
	}
	return true, &goToRandomTargetF{Mask: mask, Dis: dis}
}

type goToUncheckedContainersN struct{}

func (goToUncheckedContainersN) Check(a *world.Agent) (bool, Frame) {
	lvl := a.CurrentLevel()
	mask := lvl.ItemCount.Ne(0)
	if !mask.Any() {
		return false, nil
	}
	dis := a.Bfs()
	mask = mask.And(dis.Gt(0))
	if !mask.Any() {
		return false, nil
	}
	for _, p := range mask.Nonzero() {
		for _, item := range a.CurrentLevel().Items[p.Y][p.X] {
			if !item.IsPossibleContainer() {
				mask[p.Y][p.X] = false
			}
		}
	}
	if !mask.Any() {
		return false, nil
	}
	return true, &goToRandomTargetF{Mask: mask, Dis: dis}
}

// ---- check_containers: untrap chests below me and look inside

type checkContainersN struct{}

func (checkContainersN) Check(a *world.Agent) (bool, Frame) {
	items := belowMeIter(a.Inventory)
	for i, item := range items {
		if item.IsPossibleContainer() {
			return true, &checkContainersF{Items: items, I: i}
		}
	}
	return false, nil
}

type checkContainersF struct {
	Items []*world.Item
	I     int
	PC    int // 0: process Items[I], 1: untrapped, 2: looked
}

func (s *checkContainersF) Resume(c *Ctx) Cmd {
	for {
		switch s.PC {
		case 0:
			item := s.Items[s.I]
			if item.IsChest() && !(item.IsUnambiguous() && item.Object().Name == "ice box") {
				s.PC = 1
				return call(sense.Atom(skills.UntrapContainer()))
			}
			s.PC = 2
			return call(sense.CheckContainerContent(item))
		case 1:
			if r := c.Ret.(skills.UntrapResult); r.Fail && world.CheckIfTriggeredContainerTrap(r.Msg) {
				Panic("triggered trap while looting")
			}
			s.PC = 2
			return call(sense.CheckContainerContent(s.Items[s.I]))
		case 2:
			for s.I++; s.I < len(s.Items); s.I++ {
				if s.Items[s.I].IsPossibleContainer() {
					break
				}
			}
			if s.I == len(s.Items) {
				return ret(nil)
			}
			s.PC = 0
		}
	}
}

// ---- go_to_item_to_pickup

type goToItemToPickupN struct{}

func (goToItemToPickupN) Check(a *world.Agent) (bool, Frame) {
	inv := a.Inventory
	level := a.CurrentLevel()
	dis := a.Bfs()

	// TODO: free (no charge) items
	mask := dis.Gt(0).AndNot(level.ShopInterior)
	if !mask.Any() {
		return false, nil
	}
	cur := a.CurrentLevel()
	mask = mask.And(cur.ItemCount.Ne(0))

	items0 := NewOMap[*world.Item, Pos]()
	positions := mask.Nonzero()
	sort.SliceStable(positions, func(i, j int) bool {
		return dis[positions[i].Y][positions[i].X] < dis[positions[j].Y][positions[j].X]
	})
	for _, p := range positions {
		for _, i := range level.Items[p.Y][p.X] {
			Assert(!items0.Has(i))
			items0.Set(i, p)
		}
	}
	if items0.Len() == 0 {
		return false, nil
	}
	items := NewOMap[*world.Item, Pos]()
	for _, item := range items0.Keys() {
		pos := items0.MustGet(item)
		for i := range world.EachItem([]*world.Item{item}) {
			items.Set(i, pos)
		}
	}
	freeItems := []*world.Item{}
	for i := range world.EachItem(inv.Items.AllItems) {
		if i.CanBeDroppedFromInventory() {
			freeItems = append(freeItems, i)
		}
	}
	forcedItems := []*world.Item{}
	for i := range world.EachItem(inv.Items.AllItems) {
		if !i.CanBeDroppedFromInventory() {
			forcedItems = append(forcedItems, i)
		}
	}
	itemSplit := assess.Split(a.GlobalLogic.ItemPriority, append(append([]*world.Item{}, freeItems...), items.Keys()...),
		forcedItems, float64(a.Character.CarryingCapacity()))
	counts := assess.SumCols(itemSplit.Values())
	counts = counts[min(len(freeItems), len(counts)):]
	Assert(len(counts) == items.Len())
	s := 0
	for _, c := range counts {
		s += c
	}
	if s == 0 {
		return false, nil
	}
	keys := items.Keys()
	pos := make([]Pos, len(keys))
	for i, k := range keys {
		pos[i] = items.MustGet(k)
	}
	return true, &goToItemToPickupF{Keys: keys, Pos: pos, Counts: append([]int(nil), counts...), Dis: dis}
}

type goToItemToPickupF struct {
	Keys   []*world.Item
	Pos    []Pos // items[key]
	Counts []int
	Dis    I32Grid
	PC     int
}

func (s *goToItemToPickupF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	type kc struct {
		pos Pos
		c   int
	}
	var zs []kc
	for j := 0; j < min(len(s.Keys), len(s.Counts)); j++ {
		zs = append(zs, kc{s.Pos[j], s.Counts[j]})
	}
	dis := &s.Dis
	sort.SliceStable(zs, func(i, j int) bool {
		return dis[zs[i].pos.Y][zs[i].pos.X] < dis[zs[j].pos.Y][zs[j].pos.X]
	})
	for _, z := range zs {
		if z.c != 0 {
			s.PC = 1
			return call(skills.GoTo(z.pos.Y, z.pos.X, goToDefaultOpts()))
		}
	}
	Assert(false)
	return Cmd{}
}

// ---- gather_items

// gatherItems is the gather_items composite: pick up / drop, look into containers, dress, identify wands,
// walk to containers and items.
func GatherItems(inv *world.Inventory) flow.Node {
	return flow.Repeat(flow.Before(flow.Before(flow.Before(flow.Before(flow.Before(
		pickupAndDropItemsN{}, checkContainersN{}), wearBestStuffN{}), wandEngraveIdentifyN{}), goToUncheckedContainersN{}),
		flow.Preempt(flow.Every(5, flow.Repeat(flow.Before(checkItemsN{}, goToItemToPickupN{}))),
			[]flow.Node{pickupAndDropItemsN{}, checkContainersN{}}, true)))
}
