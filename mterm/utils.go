package mterm

import (
	"cmp"
)

// getParams helper function to get params from a slice
// if the slice is smaller than the number of params, it will leave the rest as
// is
func getParams[S ~[]T, T any](param S, out ...*T) {
	for i, p := range param {
		if i >= len(out) {
			break
		}
		*out[i] = p
	}
}

// fill fills a slice with a value
func fill[S ~[]T, T any](s S, v T) {
	for i := range s {
		s[i] = v
	}
}

// grow grows a slice by n elements
func grow[S ~[]T, T any](s S, n int) S {
	l := len(s) + n
	if l <= cap(s) {
		return s[:l]
	}
	ns := make([]T, l)
	copy(ns, s)
	return ns
}

// clamp returns the value clamped between s and b
// similar to min(max(value, smallest),biggest)
func clamp[T cmp.Ordered](v T, s, b T) T {
	if v < s {
		return s
	}
	if v > b {
		return b
	}
	return v
}
