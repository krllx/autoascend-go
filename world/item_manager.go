package world

import (
	"fmt"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
)

// Port of autoascend/item/item_manager.py (ItemManager).

type ItemManager struct {
	agent *Agent

	ObjectToGlyph                    map[*Obj]int
	GlyphToObject                    map[int]*Obj
	LastObjectGlyphMappingUpdateStep int
	LastObjectGlyphMappingUpdateNone bool
	glyphToPriceRange                map[int][2]int
	IsNotBagOfTricks                 map[int]bool
	ContainerContents                map[string]*ContainerContent // container_id -> ContainerContent
	lastContainerIdentifier          int
	GlyphToPossibleWandTypes         map[int][]*Obj
	AlreadyEngravedGlyphs            map[int]bool
}

func NewItemManager(a *Agent) *ItemManager {
	return &ItemManager{
		agent:                            a,
		ObjectToGlyph:                    map[*Obj]int{},
		GlyphToObject:                    map[int]*Obj{},
		LastObjectGlyphMappingUpdateNone: true,
		glyphToPriceRange:                map[int][2]int{},
		IsNotBagOfTricks:                 map[int]bool{},
		ContainerContents:                map[string]*ContainerContent{},
		lastContainerIdentifier:          0,
		GlyphToPossibleWandTypes:         map[int][]*Obj{},
		AlreadyEngravedGlyphs:            map[int]bool{},
	}
}

// PositionID encodes the Python position tuple (dungeon, level, y, x) used as a container identifier.
// It cannot collide with string identifiers (comments never contain '(' or ',').
func PositionID(dungeon, level, y, x int) *string {
	s := fmt.Sprintf("(%d, %d, %d, %d)", dungeon, level, y, x)
	return &s
}

// pySlice mirrors s[i:j] for 0 <= i, j (clamped like Python; empty if j < i).
func PySlice(s string, i, j int) string {
	if i < 0 {
		i += len(s)
		if i < 0 {
			i = 0
		}
	}
	if j < 0 {
		j += len(s)
		if j < 0 {
			j = 0
		}
	}
	if i > len(s) {
		i = len(s)
	}
	if j > len(s) {
		j = len(s)
	}
	if j < i {
		return ""
	}
	return s[i:j]
}

func (m *ItemManager) getNewContainerIdentifier() string {
	ret := m.lastContainerIdentifier
	m.lastContainerIdentifier++
	return fmt.Sprint(ret)
}

func (m *ItemManager) buyPriceIdentification() {
	a := m.agent
	one, two, three, four := 1.0, 2.0, 3.0, 4.0
	var charismaMultiplier float64
	if a.Blstats.Charisma <= 5 {
		charismaMultiplier = 2
	} else if a.Blstats.Charisma <= 7 {
		charismaMultiplier = 1.5
	} else if a.Blstats.Charisma <= 10 {
		charismaMultiplier = one + one/three
	} else if a.Blstats.Charisma <= 15 {
		charismaMultiplier = 1
	} else if a.Blstats.Charisma <= 17 {
		charismaMultiplier = three / four
	} else if a.Blstats.Charisma <= 18 {
		charismaMultiplier = two / three
	} else {
		charismaMultiplier = one / two
	}

	items := a.Inventory.Items
	dunce := O_FromName1("dunce cap")
	var dupe [2]float64
	if (a.Character.Role == Character_TOURIST && a.Blstats.ExperienceLevel < 15) ||
		(items.Shirt != nil && items.Suit == nil && items.Cloak == nil) ||
		(items.Helm != nil && items.Helm.IsUnambiguous() && items.Helm.Object() == dunce) {
		dupe = [2]float64{four / three, four / three}
	} else if items.Helm != nil && ContainsObj(items.Helm.Objs, dunce) {
		dupe = [2]float64{four / three, 1}
	} else {
		dupe = [2]float64{1, 1}
	}

	if a.Inventory.ItemsBelowMeNone {
		Raise("TypeError", "'NoneType' object is not iterable")
	}
	for _, item := range a.Inventory.ItemsBelowMe {
		k := item.Objs[0].Kind
		if item.ShopStatus == perception.Item_FOR_SALE && !item.IsUnambiguous() && len(item.Glyphs) == 1 &&
			(k == KArmor || k == KRing || k == KWand || k == KScroll || k == KPotion || k == KTool) {
			Assert(FloorMod(item.Price, item.Count) == 0)
			low := int(Div(Div(Div(Div(float64(item.Price), float64(item.Count)), charismaMultiplier),
				four/three), dupe[0]))
			if k == KWeapon || k == KArmor {
				low = 0 // +10 zorkmoids for each point of enchantment
			}

			if low <= 5 {
				low = 0
			}
			high := int(Div(Div(Div(float64(item.Price), float64(item.Count)), charismaMultiplier), dupe[1])) + 1
			if r, ok := m.glyphToPriceRange[item.Glyphs[0]]; ok {
				l, h := r[0], r[1]
				low = max(low, l)
				high = min(high, h)
			}
			Assert(low <= high, low, high)
			m.glyphToPriceRange[item.Glyphs[0]] = [2]int{low, high}

			// update mapping for that object
			m.PossibleObjectsFromGlyph(item.Glyphs[0])
		}
	}
}

func (m *ItemManager) PriceIdentification() {
	if m.agent.Character.Prop.Hallu() {
		return
	}
	if !m.agent.CurrentLevel().ShopInterior[m.agent.Blstats.Y][m.agent.Blstats.X] {
		return
	}

	m.buyPriceIdentification()
}

// possibilitiesUnion mirrors set.union(*(set(self.possible_objects_from_glyph(glyph)) for glyph in glyphs))
// (membership only; the calls are made in order for their side effects).
func (m *ItemManager) possibilitiesUnion(glyphs []int) map[*Obj]bool {
	if len(glyphs) == 0 {
		Raise("TypeError", "descriptor 'union' of 'set' object needs an argument")
	}
	r := map[*Obj]bool{}
	for _, g := range glyphs {
		for _, o := range m.PossibleObjectsFromGlyph(g) {
			r[o] = true
		}
	}
	return r
}

func (m *ItemManager) UpdatePossibleObjects(item *Item) {
	poss := m.possibilitiesUnion(item.Glyphs)
	objs := []*Obj{}
	for _, o := range item.Objs {
		if poss[o] {
			objs = append(objs, o)
		}
	}
	item.Objs = objs
	Assert(len(item.Objs) > 0)
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GetItemFromText: category/glyph < 0 mean None; position nil means None.
func (m *ItemManager) GetItemFromText(text string, category, glyph int, position *string) *Item {
	// position acts as a container identifier if the container is not called. If the item is in inventory set it to None

	if m.agent.Character.Prop.Hallu() {
		glyph = -1
	}

	var pt perception.ParsedText
	if r := Try(func() {
		pt = perception.ParseText(text, category, glyph)
		category = O_GetCategory(pt.Objs[0])
	}); r != nil {
		// TODO: when blind, it may not work as expected, e.g. "a shield", "a gem", "a potion", etc
		if m.agent.Character.Prop.Blind() {
			obj := O_FromName1("unknown")
			glyphs := O_PossibleGlyphsFromObject(obj)
			t := text
			return NewItem([]*Obj{obj}, glyphs, 1, perception.Item_UNKNOWN, nil, false, false, -1, perception.Item_NOT_SHOP, 0,
				nil, nil, "", "", "", &t)
		}
		panic(r)
	}
	objs, glyphs, status := pt.Objs, pt.Glyphs, pt.Status

	poss := m.possibilitiesUnion(glyphs)
	{
		n := []*Obj{}
		for _, o := range objs {
			if poss[o] {
				n = append(n, o)
			}
		}
		objs = n
	}
	if len(objs) == 0 {
		for _, g := range glyphs {
			if Objects[g-NH_GLYPH_OBJ_OFF] == nil {
				Raise("AttributeError", "'NoneType' object has no attribute 'desc'")
			}
		}
		Assert(false, text)
	}

	if status == perception.Item_UNKNOWN && (m.agent.Character.Role == Character_PRIEST ||
		(pt.Modifier != nil && category != NH_ARMOR_CLASS && category != NH_RING_CLASS)) {
		// TODO: amulets of yendor
		status = perception.Item_UNCURSED
	}

	var oldObjs []*Obj
	var oldGlyphs []int
	first := true
	for first || !perception.ObjsEqual(oldObjs, objs) || !intsEqual(oldGlyphs, glyphs) {
		first = false
		oldObjs = objs
		oldGlyphs = glyphs

		{
			n := []*Obj{}
			for _, o := range objs {
				g, ok := m.ObjectToGlyph[o]
				if !ok || Contains(glyphs, g) {
					n = append(n, o)
				}
			}
			objs = n
		}
		{
			n := []int{}
			for _, g := range glyphs {
				o, ok := m.GlyphToObject[g]
				if !ok || ContainsObj(objs, o) {
					n = append(n, g)
				}
			}
			glyphs = n
		}
		if len(objs) == 1 && len(glyphs) == 1 {
			g2o, ok := m.GlyphToObject[glyphs[0]]
			Assert(!ok || g2o == objs[0])
			m.GlyphToObject[glyphs[0]] = objs[0]
			o2g, ok := m.ObjectToGlyph[objs[0]]
			Assert(!ok || o2g == glyphs[0])
			m.ObjectToGlyph[objs[0]] = glyphs[0]
		} else if o2g, ok := m.ObjectToGlyph[PyIdxObj(objs, 0)]; len(objs) == 1 && ok {
			glyphs = []int{o2g}
		} else if g2o, ok := m.GlyphToObject[PyIdxInt(glyphs, 0)]; len(glyphs) == 1 && ok {
			objs = []*Obj{g2o}
		}
	}

	t := text
	item := NewItem(objs, glyphs, pt.Count, status, pt.Modifier, pt.Equipped, pt.AtReady, pt.MonsterID,
		pt.ShopStatus, pt.ShopPrice, pt.DmgBonus, pt.ToHitBonus, pt.Naming, pt.Comment, pt.Uses, &t)

	if item.IsPossibleContainer() || item.IsContainer() {
		var identifier string
		if item.Comment != "" {
			identifier = item.Comment
		} else {
			if position != nil {
				identifier = *position
			} else {
				identifier = m.getNewContainerIdentifier()
			}
		}
		id := identifier
		item.ContainerID = &id
		if c, ok := m.ContainerContents[identifier]; ok {
			item.Content = c
			if len(item.Glyphs) == 1 && !m.IsNotBagOfTricks[item.Glyphs[0]] {
				m.IsNotBagOfTricks[item.Glyphs[0]] = true
				m.UpdatePossibleObjects(item)
			}
		}
	}

	// FIXME: it gives a better score. Implement it in item equipping
	if item.Status == perception.Item_UNKNOWN {
		item.Status = perception.Item_UNCURSED
	}

	return item
}

// PyIdxObj returns xs[i] or nil when out of range (used only where Python short-circuits the access).
func PyIdxObj(xs []*Obj, i int) *Obj {
	if i < len(xs) {
		return xs[i]
	}
	return nil
}

// PyIdxInt returns xs[i] or -1 when out of range (used only where Python short-circuits the access).
func PyIdxInt(xs []int, i int) int {
	if i < len(xs) {
		return xs[i]
	}
	return -1
}

// PossibleObjectsFromGlyph gets possible objects and updates glyph_to_object and object_to_glyph when
// the object isn't unambiguous.
func (m *ItemManager) PossibleObjectsFromGlyph(glyph int) []*Obj {
	Assert(GlyphIsObject(glyph))
	if o, ok := m.GlyphToObject[glyph]; ok {
		return []*Obj{o}
	}

	objs := []*Obj{}
	for _, obj := range O_PossibilitiesFromGlyph(glyph) {
		if _, ok := m.ObjectToGlyph[obj]; ok {
			continue
		}
		if r, ok := m.glyphToPriceRange[glyph]; ok && obj.HasCost() && !(r[0] <= obj.Cost && obj.Cost <= r[1]) {
			continue
		}
		if wt, ok := m.GlyphToPossibleWandTypes[glyph]; ok && !ContainsObj(wt, obj) {
			continue
		}
		if m.IsNotBagOfTricks[glyph] && obj.Name == "bag of tricks" {
			continue
		}
		objs = append(objs, obj)
	}

	if len(objs) == 1 {
		m.GlyphToObject[glyph] = objs[0]
		_, ok := m.ObjectToGlyph[objs[0]]
		Assert(!ok)
		m.ObjectToGlyph[objs[0]] = glyph

		// update objects with have the same possible glyph
		for _, g := range O_PossibleGlyphsFromObject(objs[0]) {
			m.PossibleObjectsFromGlyph(g)
		}
	}
	if len(objs) == 0 {
		if Objects[glyph-NH_GLYPH_OBJ_OFF] == nil {
			Raise("AttributeError", "'NoneType' object has no attribute 'desc'")
		}
		if _, ok := m.glyphToPriceRange[glyph]; !ok {
			Raise("KeyError", "%d", glyph)
		}
		Assert(false, glyph)
	}
	return objs
}
