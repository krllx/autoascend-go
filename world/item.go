package world

import (
	"fmt"
	"iter"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
)

// Item has identity semantics (Python Item defines no __eq__): compare *Item pointers.
type Item struct {
	Objs        []*Obj
	Glyphs      []int
	Count       int
	Status      int
	Modifier    *int // None -> nil
	Equipped    bool
	AtReady     bool
	MonsterID   int // None -> -1
	ShopStatus  int
	Price       int
	DmgBonus    *float64
	ToHitBonus  *float64
	Naming      string
	Comment     string
	Uses        string // None -> ""
	Text        string // None -> ""
	TextNone    bool
	Content     *ContainerContent // None -> nil
	ContainerID *string           // None -> nil
	Category    int
}

// NewItem mirrors Item.__init__ (all arguments explicit; text nil = None).
func NewItem(objs []*Obj, glyphs []int, count, status int, modifier *int, equipped, atReady bool,
	monsterID, shopStatus, price int, dmgBonus, toHitBonus *float64, naming, comment, uses string,
	text *string) *Item {
	Assert(len(objs) >= 1)
	allObj := true
	for _, g := range glyphs {
		if !GlyphIsObject(g) {
			allObj = false
			break
		}
	}
	Assert(len(glyphs) >= 1 && allObj)

	it := &Item{
		Objs:       objs,
		Glyphs:     glyphs,
		Count:      count,
		Status:     status,
		Modifier:   modifier,
		Equipped:   equipped,
		Uses:       uses,
		AtReady:    atReady,
		MonsterID:  monsterID,
		ShopStatus: shopStatus,
		Price:      price,
		DmgBonus:   dmgBonus,
		ToHitBonus: toHitBonus,
		Naming:     naming,
		Comment:    comment,
	}
	if text == nil {
		it.TextNone = true
	} else {
		it.Text = *text
	}
	it.Content = nil
	it.ContainerID = nil

	it.Category = O_GetCategory(it.Objs[0])
	for _, g := range it.Glyphs {
		Assert(ObjClassOf(GlyphToObj(g)) == it.Category)
	}
	return it
}

type ContainerContent struct {
	Items  []*Item
	Locked bool
}

func NewContainerContent() *ContainerContent {
	c := &ContainerContent{}
	c.Reset()
	return c
}

func (c *ContainerContent) Reset() {
	c.Items = []*Item{}
	c.Locked = false
}

func (c *ContainerContent) Weight() float64 {
	if c.Locked {
		return 100000
	}
	s := 0.0
	for _, item := range c.Items {
		s += item.Weight(true)
	}
	return s
}

func (it *Item) DisplayGlyphs() []int {
	if it.IsCorpse() {
		Assert(it.MonsterID != -1)
		return []int{NH_GLYPH_BODY_OFF + it.MonsterID}
	}
	if it.IsStatue() {
		Assert(it.MonsterID != -1)
		return []int{NH_GLYPH_STATUE_OFF + it.MonsterID}
	}
	return it.Glyphs
}

func (it *Item) IsUnambiguous() bool { return len(it.Objs) == 1 }

func (it *Item) CanBeDroppedFromInventory() bool {
	k := it.Objs[0].Kind
	return !(((k == KWeapon || k == KWepTool) && it.Status == perception.Item_CURSED && it.Equipped) ||
		(k == KArmor && it.Equipped) ||
		(it.IsUnambiguous() && it.Object() == objLoadstone && it.Status == perception.Item_CURSED) ||
		(it.Category == NH_BALL_CLASS && it.Equipped))
}

func (it *Item) Weight(withContent bool) float64 {
	return float64(it.Count) * it.UnitWeight(withContent)
}

func (it *Item) UnitWeight(withContent bool) float64 {
	if it.IsCorpse() {
		return float64(MON_Permonst(it.MonsterID).Cwt)
	}

	if it.IsPossibleContainer() {
		return 100000
	}

	o := it.Objs[0]
	if o == objGlobs[0] || o == objGlobs[1] || o == objGlobs[2] || o == objGlobs[3] {
		Assert(it.IsUnambiguous())
		return 10000 // weight is unknown
	}

	var weight float64
	for i, obj := range it.Objs {
		if obj.WtNone {
			Raise("TypeError", "'>' not supported between instances of 'NoneType' and 'int'")
		}
		if i == 0 || obj.Wt > weight {
			weight = obj.Wt
		}
	}

	if it.IsContainer() && withContent {
		weight += it.Content.Weight()
	}

	return weight
}

func (it *Item) Object() *Obj {
	Assert(it.IsUnambiguous())
	return it.Objs[0]
}

// ######## WEAPON

func (it *Item) IsWeapon() bool { return it.Category == NH_WEAPON_CLASS }

// GetWeaponBonus returns (min(hits), min(dmgs)).
func (it *Item) GetWeaponBonus(large bool) (float64, float64) {
	Assert(it.IsWeapon())

	var hits, dmgs []float64
	for _, weapon := range it.Objs {
		var dmg float64
		if large {
			dmg = WEA_ExpectedDamage(weapon.DamageLarge)
		} else {
			dmg = WEA_ExpectedDamage(weapon.DamageSmall)
		}
		toHit := float64(1 + weapon.Hitbon)
		if it.Modifier != nil {
			dmg += float64(max(0, *it.Modifier))
			toHit += float64(*it.Modifier)
		}

		if it.DmgBonus != nil {
			dmg += *it.DmgBonus
		}
		if it.ToHitBonus != nil {
			toHit += *it.ToHitBonus
		}

		dmgs = append(dmgs, dmg)
		hits = append(hits, toHit)
	}

	// assume the worse
	return pyMinList(hits), pyMinList(dmgs)
}

func pyMinList(xs []float64) float64 {
	if len(xs) == 0 {
		Raise("ValueError", "min() arg is an empty sequence")
	}
	r := xs[0]
	for _, v := range xs[1:] {
		if v < r {
			r = v
		}
	}
	return r
}

func (it *Item) IsLauncher() bool {
	if !it.IsWeapon() || !it.IsUnambiguous() {
		return false
	}
	return Contains(itemLaunchers, it.Object().Name)
}

var (
	itemArrows        = []string{"arrow", "elven arrow", "orcish arrow", "silver arrow", "ya"}
	itemArrowsAndBolt = append(append([]string{}, itemArrows...), "crossbow bolt")
	itemBows          = []string{"bow", "elven bow", "orcish bow", "yumi"}
	itemLaunchers     = []string{"bow", "elven bow", "orcish bow", "yumi", "crossbow", "sling"}
	itemThrown        = []string{"dagger", "orcish dagger", "dagger silver", "athame dagger", "elven dagger",
		"worm tooth", "knife", "stiletto", "scalpel", "crysknife", "dart", "shuriken"}
)

// IsFiredProjectile mirrors is_fired_projectile(launcher); launcher nil means None.
func (it *Item) IsFiredProjectile(launcher *Item) bool {
	if !it.IsWeapon() || !it.IsUnambiguous() {
		return false
	}

	if launcher == nil {
		return Contains(itemArrowsAndBolt, it.Object().Name)
	}
	launcherName := launcher.Object().Name
	if launcherName == "crossbow" {
		return it.Object().Name == "crossbow bolt"
	} else if launcherName == "sling" {
		return false
	}
	Assert(Contains(itemBows, launcherName), launcherName)
	return Contains(itemArrows, it.Object().Name)
}

func (it *Item) IsThrownProjectile() bool {
	if !it.IsWeapon() || !it.IsUnambiguous() {
		return false
	}
	return Contains(itemThrown, it.Object().Name)
}

func (it *Item) String() string {
	if !it.TextNone {
		return it.Text
	}
	mod := ""
	if it.Modifier != nil {
		mod = fmt.Sprint(*it.Modifier)
	}
	names := make([]string, len(it.Objs))
	for i, o := range it.Objs {
		names[i] = o.Name
	}
	return fmt.Sprintf("%d_%d_%s_%s", it.Count, it.Status, mod, strings.Join(names, ","))
}

// ######## ARMOR

func (it *Item) IsArmor() bool { return it.Category == NH_ARMOR_CLASS }

func (it *Item) GetAc() int {
	Assert(it.IsArmor())
	m := 0
	if it.Modifier != nil {
		m = *it.Modifier
	}
	return it.Object().Ac - m
}

// ######## WAND

func (it *Item) IsWand() bool { return it.Objs[0].Kind == KWand }

var beamWandNames = []string{"cancellation", "locking", "make invisible",
	"nothing", "opening", "polymorph", "probing", "slow monster",
	"speed monster", "striking", "teleportation", "undead turning"}

var rayWandNames = []string{"cold", "death", "digging", "fire", "lightning", "magic missile", "sleep"}

// Objects the predicates compare items with: O.from_name of fixed names, looked up once.
var (
	objLoadstone = O_FromName1("loadstone")
	objGlobs     = []*Obj{O_FromName1("glob of gray ooze"), O_FromName1("glob of brown pudding"),
		O_FromName1("glob of green slime"), O_FromName1("glob of black pudding")}
	objCorpse     = O_FromName1("corpse")
	objStatue     = O_FromName1("statue")
	wandSleep     = O_FromName("sleep", NH_WAND_CLASS)
	wandDigging   = O_FromName("digging", NH_WAND_CLASS)
	beamWandTypes = wandsNamed(beamWandNames)
	rayWandTypes  = wandsNamed(rayWandNames)
)

func wandsNamed(names []string) []*Obj {
	types := make([]*Obj, len(names))
	for i, w := range names {
		types[i] = O_FromName(w, NH_WAND_CLASS)
	}
	return types
}

func (it *Item) IsBeamWand() bool {
	if !it.IsWand() {
		return false
	}
	for _, obj := range it.Objs {
		if !ContainsObj(beamWandTypes, obj) {
			return false
		}
	}
	return true
}

func (it *Item) IsRayWand() bool {
	if !it.IsWand() {
		return false
	}
	for _, obj := range it.Objs {
		if !ContainsObj(rayWandTypes, obj) {
			return false
		}
	}
	return true
}

func (it *Item) IsOffensiveUsableWand() bool {
	if len(it.Objs) != 1 {
		return false
	}
	if !it.IsRayWand() {
		return false
	}
	if it.Uses == "no charges" {
		return false
	}
	if it.Objs[0] == wandSleep {
		return false
	}
	if it.Objs[0] == wandDigging {
		return false
	}
	return true
}

// ######## FOOD

// IsFood mirrors is_food (returns None (falsy) for non-food).
func (it *Item) IsFood() bool {
	if it.Objs[0].Kind == KFood {
		Assert(it.IsUnambiguous())
		return true
	}
	return false
}

func (it *Item) NutritionPerWeight() float64 {
	Assert(it.IsFood())
	return Div(float64(it.Object().Nutrition), PyMaxF(it.UnitWeight(true), 1))
}

func (it *Item) IsCorpse() bool {
	if it.Objs[0] == objCorpse {
		Assert(it.IsUnambiguous())
		return true
	}
	return false
}

// ######## STATUE

func (it *Item) IsStatue() bool {
	if it.Objs[0] == objStatue {
		Assert(it.IsUnambiguous())
		return true
	}
	return false
}

// ######## CONTAINER

func (it *Item) IsChest() bool {
	if it.IsUnambiguous() && it.Object().Name == "bag of tricks" {
		return false
	}
	Assert(it.IsPossibleContainer() || it.IsContainer(), it.Objs)
	Assert(it.Objs[0].Kind == KContainer, it.Objs)
	return objDescStr(it.Objs[0]) != "bag"
}

// objDescStr returns the desc for comparisons with strings (None never equals a string).
func objDescStr(o *Obj) string {
	if o.DescNone {
		return "\x00<None>"
	}
	return o.Desc
}

func (it *Item) IsContainer() bool { return it.Content != nil }

func (it *Item) IsPossibleContainer() bool {
	if it.IsContainer() {
		return false
	}
	if it.IsUnambiguous() && it.Object().Name == "bag of tricks" {
		return false
	}
	for _, obj := range it.Objs {
		if obj.Kind == KContainer {
			return true
		}
	}
	return false
}

// FlattenItems mirrors item.utils.flatten_items.
// EachItem iterates what FlattenItems returns (each item, then the contents of a container) without building
// the slice.
func EachItem(items []*Item) iter.Seq[*Item] {
	return func(yield func(*Item) bool) { yieldItems(items, yield) }
}

func yieldItems(items []*Item, yield func(*Item) bool) bool {
	for _, item := range items {
		if !yield(item) {
			return false
		}
		if item.IsContainer() && !yieldItems(item.Content.Items, yield) {
			return false
		}
	}
	return true
}

func FlattenItems(items []*Item) []*Item {
	ret := make([]*Item, 0, len(items))
	for _, item := range items {
		ret = append(ret, item)
		if item.IsContainer() {
			ret = append(ret, FlattenItems(item.Content.Items)...)
		}
	}
	return ret
}

func FindEquivalentItem(item *Item, items []*Item) *Item {
	Assert(item.Text != "")
	for _, i := range items {
		Assert(i.Text != "")
		if i.Text == item.Text {
			return i
		}
	}
	Assert(false, item, items)
	return nil
}

func CheckIfTriggeredContainerTrap(message string) bool {
	return (strings.Contains(message, "A cloud of ") && strings.Contains(message, " gas billows from ")) ||
		strings.Contains(message, "Suddenly you are frozen in place!") ||
		strings.Contains(message, "A tower of flame bursts from ") ||
		strings.Contains(message, "You are jolted by a surge of electricity!") ||
		strings.Contains(message, "But luckily ") ||
		false
}
