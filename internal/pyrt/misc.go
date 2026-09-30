package pyrt

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// OMap is an insertion-ordered map (Python dict semantics).
type OMap[K comparable, V any] struct {
	keys []K
	m    map[K]V
}

func NewOMap[K comparable, V any]() *OMap[K, V] { return &OMap[K, V]{m: map[K]V{}} }

func (o *OMap[K, V]) Len() int { return len(o.keys) }
func (o *OMap[K, V]) Has(k K) bool {
	_, ok := o.m[k]
	return ok
}
func (o *OMap[K, V]) Get(k K) (V, bool) {
	v, ok := o.m[k]
	return v, ok
}

// MustGet mirrors d[k] (KeyError if missing).
func (o *OMap[K, V]) MustGet(k K) V {
	v, ok := o.m[k]
	if !ok {
		Raise("KeyError", "%v", k)
	}
	return v
}

// GetOr mirrors d.get(k, def).
func (o *OMap[K, V]) GetOr(k K, def V) V {
	if v, ok := o.m[k]; ok {
		return v
	}
	return def
}

// GetOrInsert mirrors defaultdict access d[k] (inserts the default on miss).
func (o *OMap[K, V]) GetOrInsert(k K, def func() V) V {
	if v, ok := o.m[k]; ok {
		return v
	}
	v := def()
	o.Set(k, v)
	return v
}

func (o *OMap[K, V]) Set(k K, v V) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

// Del mirrors `del d[k]` (KeyError if missing).
func (o *OMap[K, V]) Del(k K) {
	if _, ok := o.m[k]; !ok {
		Raise("KeyError", "%v", k)
	}
	delete(o.m, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = o.keys[:i+copy(o.keys[i:], o.keys[i+1:])]
			break
		}
	}
}

// Keys returns a copy of keys in insertion order.
func (o *OMap[K, V]) Keys() []K { return append([]K(nil), o.keys...) }

func (o *OMap[K, V]) Values() []V {
	r := make([]V, len(o.keys))
	for i, k := range o.keys {
		r[i] = o.m[k]
	}
	return r
}

// Copy mirrors dict.copy() (shallow).
func (o *OMap[K, V]) Copy() *OMap[K, V] {
	r := NewOMap[K, V]()
	for _, k := range o.keys {
		r.Set(k, o.m[k])
	}
	return r
}

// ---- numbers

// FloorDiv mirrors Python // on ints.
func FloorDiv(a, b int) int {
	if b == 0 {
		Raise("ZeroDivisionError", "integer division or modulo by zero")
	}
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// FloorMod mirrors Python % on ints.
func FloorMod(a, b int) int {
	if b == 0 {
		Raise("ZeroDivisionError", "integer division or modulo by zero")
	}
	m := a % b
	if m != 0 && ((m < 0) != (b < 0)) {
		m += b
	}
	return m
}

// FloorDivF mirrors Python // on floats.
func FloorDivF(a, b float64) float64 {
	if b == 0 {
		Raise("ZeroDivisionError", "float floor division by zero")
	}
	return math.Floor(a / b)
}

// Div mirrors Python / (ZeroDivisionError on zero).
func Div(a, b float64) float64 {
	if b == 0 {
		Raise("ZeroDivisionError", "division by zero")
	}
	return a / b
}

func Sign(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

func Abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func B2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PyMaxF mirrors builtin max(a, b) for floats (keeps a unless b > a; NaN-sensitive like Python).
func PyMaxF(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// PyMinF mirrors builtin min(a, b).
func PyMinF(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

// ---- strings

func isPySpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// PyStrip mirrors str.strip() without arguments.
func PyStrip(s string) string { return strings.TrimFunc(s, isPySpace) }

// PySplit mirrors str.split() without arguments.
func PySplit(s string) []string { return strings.FieldsFunc(s, isPySpace) }

// FindAll mirrors re.findall for patterns with >= 1 group: each match -> groups (non-participating = "").
// For a pattern without groups, each match is [whole match].
func FindAll(re *regexp.Regexp, s string) [][]string {
	ms := re.FindAllStringSubmatch(s, -1)
	r := make([][]string, len(ms))
	for i, m := range ms {
		if len(m) == 1 {
			r[i] = m
		} else {
			r[i] = m[1:]
		}
	}
	return r
}

// Search mirrors re.search: nil if no match, else groups (index 0 = whole match; non-participating = "").
func Search(re *regexp.Regexp, s string) []string { return re.FindStringSubmatch(s) }

// Index mirrors list.index (ValueError if missing).
func Index[T comparable](xs []T, v T) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	Raise("ValueError", "%v is not in list", v)
	return -1
}

func Contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// PyIdx mirrors Python negative indexing for a sequence of length n (IndexError when out of range).
func PyIdx(i, n int) int {
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		Raise("IndexError", "index out of range")
	}
	return i
}
