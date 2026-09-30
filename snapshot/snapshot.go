// Package snapshot saves and restores the bot state: a graph of plain Go values.
//
// Unlike encoding/gob it keeps the shape of the graph: two references to one object (a pointer, a slice
// backing array, a map) stay one object after Load, cycles are fine, unexported fields are written too.
// Pointers into immutable tables (RegisterStatic) are written as references and restored to the same
// addresses. Concrete types stored in interfaces must be registered (Register).
//
// Save refuses what cannot be data: funcs, channels, unsafe pointers, structs from packages outside this
// module (a regexp, a mutex), and pointers into the middle of another saved object (such an alias would
// silently become two objects). The errors name the path to the offending value, so a failing Save is also a
// check that the state holds no closures.
//
// Snapshots are only read back by the same build (types are matched by name, layouts are not versioned).
package snapshot

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

const module = "aago"

var magic = []byte("GBS1")

// ---- registries (filled from init functions, read-only afterwards)

var reg struct {
	sync.RWMutex
	types     map[string]reflect.Type
	static    []reflect.Value // by static id
	staticIdx map[ptrKey]int
	mustBe    map[reflect.Type]bool // pointer types that may only point into registered tables
}

// RequireStatic declares pointer types (e.g. (*nle.Obj)(nil)) whose values must all be registered with
// RegisterStatic: Save fails on any other pointer of these types instead of copying the object.
func RequireStatic(ptrs ...any) {
	reg.Lock()
	defer reg.Unlock()
	if reg.mustBe == nil {
		reg.mustBe = map[reflect.Type]bool{}
	}
	for _, p := range ptrs {
		reg.mustBe[reflect.TypeOf(p)] = true
	}
}

// Register makes the dynamic types of vals known to Load (types stored in interface fields: frames, nodes).
func Register(vals ...any) {
	reg.Lock()
	defer reg.Unlock()
	if reg.types == nil {
		reg.types = map[string]reflect.Type{}
	}
	for _, v := range vals {
		t := reflect.TypeOf(v)
		reg.types[typeKey(t)] = t
	}
}

// RegisterStatic registers pointers into immutable tables (e.g. nle.Objects[i], &nle.Permonsts[i]): Save writes
// them as references, Load restores the same pointers. Registration order must not depend on the run.
func RegisterStatic(ptrs ...any) {
	reg.Lock()
	defer reg.Unlock()
	if reg.staticIdx == nil {
		reg.staticIdx = map[ptrKey]int{}
	}
	for _, p := range ptrs {
		v := reflect.ValueOf(p)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			panic("snapshot.RegisterStatic: not a non-nil pointer")
		}
		k := ptrKey{v.Pointer(), v.Type()}
		if _, ok := reg.staticIdx[k]; ok {
			continue
		}
		reg.staticIdx[k] = len(reg.static)
		reg.static = append(reg.static, v)
	}
}

func typeKey(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return "*" + typeKey(t.Elem())
	}
	if t.Name() != "" && t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.String()
}

type ptrKey struct {
	addr uintptr
	t    reflect.Type
}

// ---- type properties

var plainCache sync.Map // reflect.Type -> bool

// plain reports whether values of t are bytes with no references (copied as a block).
func plain(t reflect.Type) bool {
	if v, ok := plainCache.Load(t); ok {
		return v.(bool)
	}
	var p bool
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		p = true
	case reflect.Array:
		p = plain(t.Elem())
	case reflect.Struct:
		p = structAllowed(t)
		for i := 0; p && i < t.NumField(); i++ {
			p = plain(t.Field(i).Type) && t.Field(i).Tag.Get("snapshot") != "-"
		}
	}
	plainCache.Store(t, p)
	return p
}

// structAllowed: struct types must come from this module (or be unnamed).
func structAllowed(t reflect.Type) bool {
	return t.PkgPath() == "" || t.PkgPath() == module || strings.HasPrefix(t.PkgPath(), module+"/")
}

// ---- encoder

type region struct {
	start, end uintptr
	path       string // only in the second, diagnostic pass
}

// pathElem is one step of the path to the value being written (rendered only for error messages).
type pathElem struct {
	name string // field name, type name or "->"
	idx  int    // element index when name == ""
}

type encoder struct {
	buf       []byte
	ptrs      map[ptrKey]int
	slices    map[sliceKey]int
	maps      map[uintptr]int
	nextID    int
	types     map[reflect.Type]int
	regions   []region
	path      []pathElem
	errs      []string
	withPaths bool // record region paths (second pass, after an alias was found)
	trace     []traceEntry
	tracing   bool
}

type traceEntry struct {
	off  int
	path string
}

func (e *encoder) pathString() string {
	var b strings.Builder
	for _, p := range e.path {
		if p.name == "" {
			b.WriteString("[" + strconv.Itoa(p.idx) + "]")
		} else {
			b.WriteString(p.name)
		}
	}
	return b.String()
}

func (e *encoder) push(name string, idx int) { e.path = append(e.path, pathElem{name, idx}) }
func (e *encoder) pop()                      { e.path = e.path[:len(e.path)-1] }

func (e *encoder) addRegion(start, end uintptr) {
	r := region{start: start, end: end}
	if e.withPaths {
		r.path = e.pathString()
	}
	e.regions = append(e.regions, r)
}

type sliceKey struct {
	data     uintptr
	len, cap int
	t        reflect.Type
}

// Save encodes the graph reachable from root (a non-nil pointer).
func Save(root any) ([]byte, error) {
	v := reflect.ValueOf(root)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, errors.New("snapshot.Save: root must be a non-nil pointer")
	}
	reg.RLock()
	defer reg.RUnlock()
	e := encode(v, false)
	if e.aliased() {
		e = encode(v, true) // again, to name the paths
		e.checkRegions()
	}
	if len(e.errs) > 0 {
		return nil, fmt.Errorf("snapshot: %d problems:\n  %s", len(e.errs), strings.Join(summarize(e.errs, 40), "\n  "))
	}
	return e.buf, nil
}

// Explain names the value whose encoding covers byte offset off of Save(root) (a diagnostic: to find what
// differs between two snapshots).
func Explain(root any, off int) string {
	reg.RLock()
	defer reg.RUnlock()
	e := &encoder{ptrs: map[ptrKey]int{}, slices: map[sliceKey]int{}, maps: map[uintptr]int{},
		types: map[reflect.Type]int{}, tracing: true}
	v := reflect.ValueOf(root)
	e.encodeRoot(v)
	best := ""
	for _, t := range e.trace {
		if t.off <= off {
			best = t.path
		}
	}
	return best
}

var indexRe = regexp.MustCompile(`\[\d+\]`)

// summarize merges errors that differ only in element indices and keeps at most n lines.
func summarize(errs []string, n int) []string {
	count := map[string]int{}
	var order []string
	for _, e := range errs {
		k := indexRe.ReplaceAllString(e, "[*]")
		if count[k] == 0 {
			order = append(order, k)
		}
		count[k]++
	}
	sort.Strings(order)
	var out []string
	for _, k := range order {
		if c := count[k]; c > 1 {
			k += fmt.Sprintf(" (x%d)", c)
		}
		out = append(out, k)
	}
	if len(out) > n {
		out = append(out[:n], fmt.Sprintf("... and %d more", len(out)-n))
	}
	return out
}

func encode(v reflect.Value, withPaths bool) *encoder {
	e := &encoder{ptrs: map[ptrKey]int{}, slices: map[sliceKey]int{}, maps: map[uintptr]int{},
		types: map[reflect.Type]int{}, withPaths: withPaths}
	e.encodeRoot(v)
	return e
}

func (e *encoder) encodeRoot(v reflect.Value) {
	e.buf = append(e.buf, magic...)
	e.push(typeKey(v.Type()), 0)
	// the root object is id 0; Load decodes its content into the caller's value
	e.ptrs[ptrKey{v.Pointer(), v.Type()}] = 0
	e.nextID = 1
	if v.Type().Elem().Size() > 0 {
		e.addRegion(v.Pointer(), v.Pointer()+v.Type().Elem().Size())
	}
	e.value(v.Elem())
}

func (e *encoder) fail(format string, args ...any) {
	e.errs = append(e.errs, e.pathString()+": "+fmt.Sprintf(format, args...))
}

func (e *encoder) uvarint(x uint64) { e.buf = binary.AppendUvarint(e.buf, x) }
func (e *encoder) varint(x int64)   { e.buf = binary.AppendVarint(e.buf, x) }

func (e *encoder) raw(p unsafe.Pointer, n uintptr) {
	if n > 0 {
		e.buf = append(e.buf, unsafe.Slice((*byte)(p), n)...)
	}
}

// value encodes v, which must be addressable for block copies of plain data (pointer targets always are).
func (e *encoder) value(v reflect.Value) {
	t := v.Type()
	if e.tracing {
		e.trace = append(e.trace, traceEntry{len(e.buf), e.pathString()})
	}
	if plain(t) && v.CanAddr() {
		e.raw(unsafe.Pointer(v.UnsafeAddr()), t.Size())
		return
	}
	switch t.Kind() {
	case reflect.Bool:
		if v.Bool() {
			e.buf = append(e.buf, 1)
		} else {
			e.buf = append(e.buf, 0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.varint(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.uvarint(v.Uint())
	case reflect.Float32:
		e.buf = binary.LittleEndian.AppendUint32(e.buf, math.Float32bits(float32(v.Float())))
	case reflect.Float64:
		e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(v.Float()))
	case reflect.Complex64, reflect.Complex128:
		c := v.Complex()
		e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(real(c)))
		e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(imag(c)))
	case reflect.String:
		s := v.String()
		e.uvarint(uint64(len(s)))
		e.buf = append(e.buf, s...)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			e.push("", i)
			e.value(v.Index(i))
			e.pop()
		}
	case reflect.Struct:
		if !structAllowed(t) {
			e.fail("struct %s from outside the module", t)
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Tag.Get("snapshot") == "-" {
				continue
			}
			e.push("."+f.Name, 0)
			e.value(field(v, i))
			e.pop()
		}
	case reflect.Slice:
		e.slice(v)
	case reflect.Map:
		e.mapValue(v)
	case reflect.Pointer:
		e.pointer(v)
	case reflect.Interface:
		e.iface(v)
	default: // Func, Chan, UnsafePointer
		e.fail("%s cannot be saved", t)
	}
}

// field returns struct field i of v, readable even if unexported.
func field(v reflect.Value, i int) reflect.Value {
	f := v.Field(i)
	if f.CanInterface() || !v.CanAddr() {
		return f
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func (e *encoder) slice(v reflect.Value) {
	if v.IsNil() {
		e.buf = append(e.buf, 0)
		return
	}
	if v.Cap() == 0 {
		e.buf = append(e.buf, 3)
		return
	}
	k := sliceKey{v.Pointer(), v.Len(), v.Cap(), v.Type()}
	if id, ok := e.slices[k]; ok {
		e.buf = append(e.buf, 1)
		e.uvarint(uint64(id))
		return
	}
	id := e.nextID
	e.nextID++
	e.slices[k] = id
	et := v.Type().Elem()
	e.addRegion(k.data, k.data+uintptr(v.Cap())*et.Size())
	e.buf = append(e.buf, 2)
	e.uvarint(uint64(v.Len()))
	e.uvarint(uint64(v.Cap()))
	if plain(et) {
		e.raw(v.UnsafePointer(), uintptr(v.Len())*et.Size())
		return
	}
	for i := 0; i < v.Len(); i++ {
		e.push("", i)
		e.value(v.Index(i))
		e.pop()
	}
}

func (e *encoder) mapValue(v reflect.Value) {
	if v.IsNil() {
		e.buf = append(e.buf, 0)
		return
	}
	p := v.Pointer()
	if id, ok := e.maps[p]; ok {
		e.buf = append(e.buf, 1)
		e.uvarint(uint64(id))
		return
	}
	id := e.nextID
	e.nextID++
	e.maps[p] = id
	e.buf = append(e.buf, 2)
	e.uvarint(uint64(v.Len()))
	keys := v.MapKeys()
	e.sortKeys(keys)
	kt, vt := v.Type().Key(), v.Type().Elem()
	for _, k := range keys {
		kk := reflect.New(kt).Elem() // addressable copies
		kk.Set(k)
		vv := reflect.New(vt).Elem()
		vv.Set(v.MapIndex(k))
		e.push("[key]", 0)
		e.value(kk)
		e.path[len(e.path)-1].name = "[value]"
		e.value(vv)
		e.pop()
	}
}

// sortKeys orders map keys so that equal states give equal bytes: scalars and plain structs by value, pointers
// by their static id or, when all are already saved, by object id (nil first). Other keys keep the map's order.
func (e *encoder) sortKeys(keys []reflect.Value) {
	if len(keys) < 2 {
		return
	}
	switch keys[0].Kind() {
	case reflect.Pointer:
		ids := make(map[uintptr]int, len(keys))
		for _, k := range keys {
			pk := ptrKey{k.Pointer(), k.Type()}
			if pk.addr == 0 {
				ids[0] = -1
			} else if sid, ok := reg.staticIdx[pk]; ok {
				ids[pk.addr] = sid
			} else if id, ok := e.ptrs[pk]; ok {
				ids[pk.addr] = len(reg.static) + id
			} else {
				return
			}
		}
		sort.Slice(keys, func(i, j int) bool { return ids[keys[i].Pointer()] < ids[keys[j].Pointer()] })
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Int() < keys[j].Int() })
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Uint() < keys[j].Uint() })
	case reflect.String:
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	case reflect.Float32, reflect.Float64:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Float() < keys[j].Float() })
	case reflect.Struct, reflect.Array:
		if plain(keys[0].Type()) {
			b := make([]string, len(keys))
			for i, k := range keys {
				kk := reflect.New(k.Type()).Elem()
				kk.Set(k)
				b[i] = string(unsafe.Slice((*byte)(unsafe.Pointer(kk.UnsafeAddr())), k.Type().Size()))
			}
			idx := make([]int, len(keys))
			for i := range idx {
				idx[i] = i
			}
			sort.Slice(idx, func(i, j int) bool { return b[idx[i]] < b[idx[j]] })
			out := make([]reflect.Value, len(keys))
			for i, j := range idx {
				out[i] = keys[j]
			}
			copy(keys, out)
		}
	}
}

func (e *encoder) pointer(v reflect.Value) {
	if v.IsNil() {
		e.buf = append(e.buf, 0)
		return
	}
	k := ptrKey{v.Pointer(), v.Type()}
	if sid, ok := reg.staticIdx[k]; ok {
		e.buf = append(e.buf, 3)
		e.uvarint(uint64(sid))
		return
	}
	if reg.mustBe[k.t] {
		e.fail("%s does not point into a registered table (snapshot.RegisterStatic)", k.t)
		return
	}
	if id, ok := e.ptrs[k]; ok {
		e.buf = append(e.buf, 1)
		e.uvarint(uint64(id))
		return
	}
	id := e.nextID
	e.nextID++
	e.ptrs[k] = id
	et := v.Type().Elem()
	if et.Size() > 0 {
		e.addRegion(k.addr, k.addr+et.Size())
	}
	e.buf = append(e.buf, 2)
	e.push("->", 0)
	e.value(v.Elem())
	e.pop()
}

func (e *encoder) iface(v reflect.Value) {
	if v.IsNil() {
		e.buf = append(e.buf, 0)
		return
	}
	c := v.Elem()
	t := c.Type()
	if _, ok := reg.types[typeKey(t)]; !ok {
		e.fail("type %s is not registered (snapshot.Register)", typeKey(t))
		return
	}
	id, ok := e.types[t]
	if !ok {
		id = len(e.types) + 1
		e.types[t] = id
		e.uvarint(uint64(id))
		k := typeKey(t)
		e.uvarint(uint64(len(k)))
		e.buf = append(e.buf, k...)
	} else {
		e.uvarint(uint64(id))
	}
	if t.Kind() != reflect.Pointer {
		cc := reflect.New(t).Elem() // addressable copy of the value held in the interface
		cc.Set(c)
		c = cc
	}
	e.push("("+t.String()+")", 0)
	e.value(c)
	e.pop()
}

func (e *encoder) sortRegions() {
	r := e.regions
	sort.Slice(r, func(i, j int) bool {
		if r[i].start != r[j].start {
			return r[i].start < r[j].start
		}
		return r[i].end > r[j].end
	})
}

// aliased reports whether some saved pointer or slice points into another saved object.
func (e *encoder) aliased() bool {
	e.sortRegions()
	var maxEnd uintptr
	for i, r := range e.regions {
		if i > 0 && r.start < maxEnd {
			return true
		}
		maxEnd = max(maxEnd, r.end)
	}
	return false
}

// checkRegions names every such alias.
func (e *encoder) checkRegions() {
	e.sortRegions()
	var maxEnd uintptr
	var owner string
	for i, r := range e.regions {
		if i > 0 && r.start < maxEnd {
			e.errs = append(e.errs, fmt.Sprintf("%s: points into the object saved at %s (an alias Load cannot keep)",
				r.path, owner))
			continue
		}
		if r.end > maxEnd {
			maxEnd, owner = r.end, r.path
		}
	}
}

// ---- decoder

type decoder struct {
	buf   []byte
	objs  map[int]reflect.Value // id -> pointer / slice / map value
	types []reflect.Type
	err   error
}

// Load decodes data into root, a pointer to a zero value of the type that was saved.
func Load(data []byte, root any) (err error) {
	v := reflect.ValueOf(root)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("snapshot.Load: root must be a non-nil pointer")
	}
	if len(data) < len(magic) || string(data[:len(magic)]) != string(magic) {
		return errors.New("snapshot.Load: not a snapshot")
	}
	reg.RLock()
	defer reg.RUnlock()
	d := &decoder{buf: data[len(magic):], objs: map[int]reflect.Value{0: v}, types: []reflect.Type{nil}}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("snapshot.Load: %v", r)
		}
	}()
	d.value(v.Elem())
	if len(d.buf) != 0 {
		return fmt.Errorf("snapshot.Load: %d trailing bytes", len(d.buf))
	}
	return nil
}

func (d *decoder) byte1() byte {
	b := d.buf[0]
	d.buf = d.buf[1:]
	return b
}

func (d *decoder) uvarint() uint64 {
	x, n := binary.Uvarint(d.buf)
	if n <= 0 {
		panic("bad uvarint")
	}
	d.buf = d.buf[n:]
	return x
}

func (d *decoder) varint() int64 {
	x, n := binary.Varint(d.buf)
	if n <= 0 {
		panic("bad varint")
	}
	d.buf = d.buf[n:]
	return x
}

func (d *decoder) raw(p unsafe.Pointer, n uintptr) {
	if n > 0 {
		copy(unsafe.Slice((*byte)(p), n), d.buf[:n])
		d.buf = d.buf[n:]
	}
}

// value decodes into v (addressable, settable through unsafe if unexported).
func (d *decoder) value(v reflect.Value) {
	t := v.Type()
	if plain(t) {
		d.raw(unsafe.Pointer(v.UnsafeAddr()), t.Size())
		return
	}
	switch t.Kind() {
	case reflect.Bool:
		v.SetBool(d.byte1() != 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(d.varint())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(d.uvarint())
	case reflect.Float32:
		v.SetFloat(float64(math.Float32frombits(binary.LittleEndian.Uint32(d.buf))))
		d.buf = d.buf[4:]
	case reflect.Float64:
		v.SetFloat(math.Float64frombits(binary.LittleEndian.Uint64(d.buf)))
		d.buf = d.buf[8:]
	case reflect.Complex64, reflect.Complex128:
		re := math.Float64frombits(binary.LittleEndian.Uint64(d.buf))
		im := math.Float64frombits(binary.LittleEndian.Uint64(d.buf[8:]))
		d.buf = d.buf[16:]
		v.SetComplex(complex(re, im))
	case reflect.String:
		n := d.uvarint()
		v.SetString(string(d.buf[:n]))
		d.buf = d.buf[n:]
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			d.value(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).Tag.Get("snapshot") == "-" {
				continue
			}
			d.value(settable(v.Field(i)))
		}
	case reflect.Slice:
		d.slice(v)
	case reflect.Map:
		d.mapValue(v)
	case reflect.Pointer:
		d.pointer(v)
	case reflect.Interface:
		d.iface(v)
	default:
		panic(fmt.Sprintf("cannot load %s", t))
	}
}

func settable(f reflect.Value) reflect.Value {
	if f.CanSet() {
		return f
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

func (d *decoder) slice(v reflect.Value) {
	switch d.byte1() {
	case 0:
		v.SetZero()
	case 3:
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	case 1:
		v.Set(d.objs[int(d.uvarint())])
	case 2:
		id := len(d.objs)
		n, c := int(d.uvarint()), int(d.uvarint())
		s := reflect.MakeSlice(v.Type(), n, c)
		d.objs[id] = s
		et := v.Type().Elem()
		if plain(et) {
			d.raw(s.UnsafePointer(), uintptr(n)*et.Size())
		} else {
			for i := 0; i < n; i++ {
				d.value(s.Index(i))
			}
		}
		v.Set(s)
	default:
		panic("bad slice tag")
	}
}

func (d *decoder) mapValue(v reflect.Value) {
	switch d.byte1() {
	case 0:
		v.SetZero()
	case 1:
		v.Set(d.objs[int(d.uvarint())])
	case 2:
		id := len(d.objs)
		n := int(d.uvarint())
		m := reflect.MakeMapWithSize(v.Type(), n)
		d.objs[id] = m
		kt, vt := v.Type().Key(), v.Type().Elem()
		for i := 0; i < n; i++ {
			k := reflect.New(kt).Elem()
			d.value(k)
			x := reflect.New(vt).Elem()
			d.value(x)
			m.SetMapIndex(k, x)
		}
		v.Set(m)
	default:
		panic("bad map tag")
	}
}

func (d *decoder) pointer(v reflect.Value) {
	switch d.byte1() {
	case 0:
		v.SetZero()
	case 1:
		v.Set(d.objs[int(d.uvarint())])
	case 3:
		v.Set(reg.static[int(d.uvarint())])
	case 2:
		id := len(d.objs)
		p := reflect.New(v.Type().Elem())
		d.objs[id] = p
		v.Set(p) // before the content: cycles resolve to p
		d.value(p.Elem())
	default:
		panic("bad pointer tag")
	}
}

func (d *decoder) iface(v reflect.Value) {
	id := int(d.uvarint())
	if id == 0 {
		v.SetZero()
		return
	}
	if id == len(d.types) {
		n := d.uvarint()
		k := string(d.buf[:n])
		d.buf = d.buf[n:]
		t, ok := reg.types[k]
		if !ok {
			panic(fmt.Sprintf("type %s is not registered", k))
		}
		d.types = append(d.types, t)
	}
	t := d.types[id]
	c := reflect.New(t).Elem()
	d.value(c)
	v.Set(c)
}

// Clone returns a deep copy of *v made through Save and Load.
func Clone[T any](v *T) (*T, error) {
	b, err := Save(v)
	if err != nil {
		return nil, err
	}
	out := new(T)
	if err := Load(b, out); err != nil {
		return nil, err
	}
	return out, nil
}
