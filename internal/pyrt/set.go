package pyrt

// PySet reproduces CPython 3.11 set layout (setobject.c) so iteration order matches Python for
// sets of ints and 2-tuples of ints. Use it wherever Python iterates over a set (for x in s,
// list(s), next(iter(s)), min/max with ties, dict comprehensions over a set).

type SetKey struct {
	A, B  int
	Tuple bool // false: the int A; true: the tuple (A, B)
}

func IntKey(a int) SetKey     { return SetKey{A: a} }
func PairKey(a, b int) SetKey { return SetKey{A: a, B: b, Tuple: true} }

const (
	xxPrime1 uint64 = 11400714785074694791
	xxPrime2 uint64 = 14029467366897019727
	xxPrime5 uint64 = 2870177450012600261
)

func pyHashInt(v int) int64 {
	// valid for |v| < 2**61-1
	if v == -1 {
		return -2
	}
	return int64(v)
}

func pyHashTuple(items ...int) int64 {
	acc := xxPrime5
	for _, it := range items {
		lane := uint64(pyHashInt(it))
		acc += lane * xxPrime2
		acc = (acc << 31) | (acc >> 33)
		acc *= xxPrime1
	}
	acc += uint64(len(items)) ^ (xxPrime5 ^ 3527539)
	if acc == ^uint64(0) {
		return 1546275796
	}
	return int64(acc)
}

func (k SetKey) hash() int64 {
	if k.Tuple {
		return pyHashTuple(k.A, k.B)
	}
	return pyHashInt(k.A)
}

type setEntry struct {
	key   SetKey
	hash  int64
	state uint8 // 0 empty, 1 active, 2 dummy
}

type PySet struct {
	table []setEntry
	mask  int
	fill  int
	used  int
}

const setMinSize = 8
const linearProbes = 9

func NewPySet(keys ...SetKey) *PySet {
	s := &PySet{table: make([]setEntry, setMinSize), mask: setMinSize - 1}
	for _, k := range keys {
		s.Add(k)
	}
	return s
}

func (s *PySet) Len() int { return s.used }

func (s *PySet) lookup(k SetKey, h int64) int {
	mask := uint64(s.mask)
	perturb := uint64(h)
	i := uint64(h) & mask
	for {
		probes := 0
		if i+linearProbes <= mask {
			probes = linearProbes
		}
		j := i
		for {
			e := &s.table[j]
			if e.state == 0 {
				return -1
			}
			if e.state == 1 && e.hash == h && e.key == k {
				return int(j)
			}
			j++
			if probes == 0 {
				break
			}
			probes--
		}
		perturb >>= 5
		i = (i*5 + 1 + perturb) & mask
	}
}

func (s *PySet) Contains(k SetKey) bool { return s.lookup(k, k.hash()) >= 0 }

func (s *PySet) Add(k SetKey) {
	h := k.hash()
	mask := uint64(s.mask)
	perturb := uint64(h)
	i := uint64(h) & mask
	free := -1
	for {
		probes := 0
		if i+linearProbes <= mask {
			probes = linearProbes
		}
		j := i
		for {
			e := &s.table[j]
			if e.state == 0 {
				if free >= 0 {
					s.used++
					s.table[free] = setEntry{key: k, hash: h, state: 1}
					return
				}
				s.fill++
				s.used++
				*e = setEntry{key: k, hash: h, state: 1}
				if s.fill*5 < s.mask*3 {
					return
				}
				if s.used > 50000 {
					s.resize(s.used * 2)
				} else {
					s.resize(s.used * 4)
				}
				return
			}
			if e.state == 1 && e.hash == h && e.key == k {
				return
			}
			if e.state == 2 && free < 0 {
				free = int(j)
			}
			j++
			if probes == 0 {
				break
			}
			probes--
		}
		perturb >>= 5
		i = (i*5 + 1 + perturb) & mask
	}
}

func insertClean(table []setEntry, mask uint64, k SetKey, h int64) {
	perturb := uint64(h)
	i := uint64(h) & mask
	for {
		if table[i].state == 0 {
			table[i] = setEntry{key: k, hash: h, state: 1}
			return
		}
		if i+linearProbes <= mask {
			for j := uint64(1); j <= linearProbes; j++ {
				if table[i+j].state == 0 {
					table[i+j] = setEntry{key: k, hash: h, state: 1}
					return
				}
			}
		}
		perturb >>= 5
		i = (i*5 + 1 + perturb) & mask
	}
}

func (s *PySet) resize(minused int) {
	newsize := setMinSize
	for newsize <= minused {
		newsize <<= 1
	}
	old := s.table
	s.table = make([]setEntry, newsize)
	s.mask = newsize - 1
	s.fill = s.used
	for _, e := range old {
		if e.state == 1 {
			insertClean(s.table, uint64(s.mask), e.key, e.hash)
		}
	}
}

// Discard mirrors set.discard (leaves a dummy).
func (s *PySet) Discard(k SetKey) {
	j := s.lookup(k, k.hash())
	if j < 0 {
		return
	}
	s.table[j].state = 2
	s.table[j].hash = -1
	s.used--
}

// Remove mirrors set.remove (KeyError if missing).
func (s *PySet) Remove(k SetKey) {
	if !s.Contains(k) {
		Raise("KeyError", "%v", k)
	}
	s.Discard(k)
}

// Items returns keys in CPython iteration order.
func (s *PySet) Items() []SetKey {
	r := make([]SetKey, 0, s.used)
	for _, e := range s.table {
		if e.state == 1 {
			r = append(r, e.key)
		}
	}
	return r
}

// merge mirrors set_merge(so, other).
func (s *PySet) merge(o *PySet) {
	if o == s || o.used == 0 {
		return
	}
	if (s.fill+o.used)*5 >= s.mask*3 {
		s.resize((s.used + o.used) * 2)
	}
	if s.fill == 0 && s.mask == o.mask && o.fill == o.used {
		copy(s.table, o.table)
		s.fill = o.fill
		s.used = o.used
		return
	}
	if s.fill == 0 {
		s.fill = o.used
		s.used = o.used
		for _, e := range o.table {
			if e.state == 1 {
				insertClean(s.table, uint64(s.mask), e.key, e.hash)
			}
		}
		return
	}
	for _, e := range o.table {
		if e.state == 1 {
			s.Add(e.key)
		}
	}
}

// Copy mirrors set(s) / s.copy().
func (s *PySet) Copy() *PySet {
	r := NewPySet()
	r.merge(s)
	return r
}

// Update mirrors s.update(o) / s |= o for a set argument.
func (s *PySet) Update(o *PySet) { s.merge(o) }

// Union mirrors s.union(*others) / set.union(s, *others).
func (s *PySet) Union(others ...*PySet) *PySet {
	r := s.Copy()
	for _, o := range others {
		if o == s {
			continue
		}
		r.merge(o)
	}
	return r
}

// SetFromList mirrors set(list) (adds one by one).
func SetFromList(keys []SetKey) *PySet { return NewPySet(keys...) }
