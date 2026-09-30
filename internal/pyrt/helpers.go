package pyrt

// Small generic helpers for the Python idioms the bot logic uses most.

// Any is Python `any(f(x) for x in xs)`: stops at the first hit.
func Any[T any](xs []T, f func(T) bool) bool {
	for _, x := range xs {
		if f(x) {
			return true
		}
	}
	return false
}

// All is Python `all(f(x) for x in xs)`: stops at the first miss.
func All[T any](xs []T, f func(T) bool) bool {
	for _, x := range xs {
		if !f(x) {
			return false
		}
	}
	return true
}

// Filter is Python `[x for x in xs if f(x)]`. Never returns nil (an empty list is not None).
func Filter[T any](xs []T, f func(T) bool) []T {
	out := []T{}
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

// MinBy is Python `min(xs, key=key)`: the first element with the smallest key.
// Raises ValueError on an empty list.
func MinBy[T any, K int | int32 | float64](xs []T, key func(T) K) T {
	if len(xs) == 0 {
		Raise("ValueError", "min() arg is an empty sequence")
	}
	best, bestKey := xs[0], key(xs[0])
	for _, x := range xs[1:] {
		if k := key(x); k < bestKey {
			best, bestKey = x, k
		}
	}
	return best
}
