package nle

import (
	. "aago/internal/pyrt"
)

// Port of autoascend/objects/utils.py on top of tables precomputed in the arena image.

var objRank = map[int]int{}

func init() {
	for r, i := range objectSortOrder {
		objRank[i] = r
	}
}

// ObjLess mirrors Python's tuple ordering of object namedtuples (sorted(objs)).
func ObjLess(a, b *Obj) bool { return objRank[a.Idx] < objRank[b.Idx] }

func objsFromIdx(ix []int) []*Obj {
	r := make([]*Obj, len(ix))
	for i, v := range ix {
		r[i] = Objects[v]
	}
	return r
}

// ObjClassOf mirrors ord(nh.objclass(i).oc_class).
func ObjClassOf(i int) int { return ObjClasses[i].OcClass }

// O_PossibilitiesFromGlyph mirrors O.possibilities_from_glyph (returns a fresh copy).
func O_PossibilitiesFromGlyph(g int) []*Obj {
	Assert(GlyphIsObject(g), g)
	if e, ok := possFromGlyphErr[g]; ok {
		Raise(e, "possibilities_from_glyph(%d)", g)
	}
	ix, ok := possFromGlyph[g]
	Assert(ok, g)
	return objsFromIdx(ix)
}

// O_PossibleGlyphsFromObject mirrors O.possible_glyphs_from_object (fresh copy).
func O_PossibleGlyphsFromObject(o *Obj) []int {
	return append([]int(nil), possibleGlyphsFromObject[o.Idx]...)
}

// O_DescToGlyphs mirrors O.desc_to_glyphs(desc, category); category < 0 means None.
func O_DescToGlyphs(desc string, category int) []int {
	var r []int
	for i, o := range Objects {
		if o != nil && category >= 0 && ObjClassOf(i) == category && !o.DescNone && o.Desc == desc {
			r = append(r, i+NH_GLYPH_OBJ_OFF)
		}
	}
	Assert(len(r) > 0, desc)
	return r
}

// objectsByName lists the named objects by name, in table order (read-only after init).
var objectsByName = func() map[string][]int {
	m := map[string][]int{}
	for i, o := range Objects {
		if o != nil && !o.NameNone {
			m[o.Name] = append(m[o.Name], i)
		}
	}
	return m
}()

// O_FromName mirrors O.from_name(name, category); category < 0 means None.
func O_FromName(name string, category int) *Obj {
	var ret *Obj
	n := 0
	for _, i := range objectsByName[name] {
		if category < 0 || ObjClassOf(i) == category {
			ret = Objects[i]
			n++
		}
	}
	if n != 1 {
		Assert(false, name, category) // not Assert(n == 1, ...): boxing the arguments allocates on every call
	}
	return ret
}

// O_FromName1 mirrors O.from_name(name) (no category).
func O_FromName1(name string) *Obj { return O_FromName(name, -1) }

// O_GetCategory mirrors O.get_category(obj).
func O_GetCategory(o *Obj) int { return objCategory[o.Idx] }

// SortObjs mirrors sorted(objs) (stable).
func SortObjs(objs []*Obj) []*Obj {
	r := append([]*Obj(nil), objs...)
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && ObjLess(r[j], r[j-1]); j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
	return r
}

func ContainsObj(objs []*Obj, o *Obj) bool {
	for _, x := range objs {
		if x == o {
			return true
		}
	}
	return false
}
