package pyrt

// RandomState reproduces numpy.random.RandomState (legacy MT19937) for the calls AutoAscend makes.
type RandomState struct {
	key [624]uint32
	pos int
}

func NewRandomState(seed uint32) *RandomState {
	r := &RandomState{}
	s := seed
	for pos := 0; pos < 624; pos++ {
		r.key[pos] = s
		s = 1812433253*(s^(s>>30)) + uint32(pos) + 1
	}
	r.pos = 624
	return r
}

func (r *RandomState) gen() {
	const n, m = 624, 397
	const matrixA, upper, lower = 0x9908b0df, 0x80000000, 0x7fffffff
	k := &r.key
	var i int
	for i = 0; i < n-m; i++ {
		y := (k[i] & upper) | (k[i+1] & lower)
		k[i] = k[i+m] ^ (y >> 1) ^ (-(y & 1) & matrixA)
	}
	for ; i < n-1; i++ {
		y := (k[i] & upper) | (k[i+1] & lower)
		k[i] = k[i+(m-n)] ^ (y >> 1) ^ (-(y & 1) & matrixA)
	}
	y := (k[n-1] & upper) | (k[0] & lower)
	k[n-1] = k[m-1] ^ (y >> 1) ^ (-(y & 1) & matrixA)
	r.pos = 0
}

func (r *RandomState) Next32() uint32 {
	if r.pos == 624 {
		r.gen()
	}
	y := r.key[r.pos]
	r.pos++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

func (r *RandomState) Next64() uint64 {
	hi := uint64(r.Next32())
	return hi<<32 | uint64(r.Next32())
}

// RandomSample mirrors random_sample() (one double).
func (r *RandomState) RandomSample() float64 {
	a := r.Next32() >> 5
	b := r.Next32() >> 6
	return (float64(a)*67108864.0 + float64(b)) / 9007199254740992.0
}

func genMask(m uint64) uint64 {
	m |= m >> 1
	m |= m >> 2
	m |= m >> 4
	m |= m >> 8
	m |= m >> 16
	m |= m >> 32
	return m
}

func (r *RandomState) interval(max uint64) uint64 {
	if max == 0 {
		return 0
	}
	mask := genMask(max)
	if max <= 0xffffffff {
		for {
			v := uint64(r.Next32()) & mask
			if v <= max {
				return v
			}
		}
	}
	for {
		v := r.Next64() & mask
		if v <= max {
			return v
		}
	}
}

// RandInt mirrors RandomState.randint(low, high) (high exclusive); ValueError if low >= high.
func (r *RandomState) RandInt(low, high int) int {
	if low >= high {
		Raise("ValueError", "low >= high")
	}
	rng := uint64(high - 1 - low)
	if rng == 0 {
		return low
	}
	if rng <= 0xffffffff {
		if rng == 0xffffffff {
			return low + int(r.Next32())
		}
		mask := genMask(rng)
		for {
			v := uint64(r.Next32()) & mask
			if v <= rng {
				return low + int(v)
			}
		}
	}
	mask := genMask(rng)
	for {
		v := r.Next64() & mask
		if v <= rng {
			return low + int(v)
		}
	}
}

// RandInt1 mirrors randint(high) == randint(0, high).
func (r *RandomState) RandInt1(high int) int { return r.RandInt(0, high) }

// ShuffleLen mirrors the legacy untyped shuffle path; swap(i, j) is called for each step.
func (r *RandomState) ShuffleLen(n int, swap func(i, j int)) {
	for i := n - 1; i >= 1; i-- {
		j := int(r.interval(uint64(i)))
		swap(i, j)
	}
}

// ChoiceP mirrors choice(range(n), p=p) for a 1-D probability vector (p already normalized by the caller).
func (r *RandomState) ChoiceP(p []float64) int {
	cdf := make([]float64, len(p))
	s := 0.0
	for i, v := range p {
		s += v
		cdf[i] = s
	}
	last := cdf[len(cdf)-1]
	for i := range cdf {
		cdf[i] /= last
	}
	u := r.RandomSample()
	// searchsorted side='right': first i with cdf[i] > u
	lo, hi := 0, len(cdf)
	for lo < hi {
		mid := (lo + hi) / 2
		if cdf[mid] <= u {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
