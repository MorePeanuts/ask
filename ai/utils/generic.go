package utils

import "reflect"

// CopyMap copies a map to a new map.
func CopyMap[K comparable, V any](src map[K]V) map[K]V {
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// TypeOf returns the type of T.
// eg. TypeOf[int] returns reflect.TypeOf(int).
// eg. TypeOf[*int] returns reflect.TypeOf(*int).
func TypeOf[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

// Reverse returns a new slice with elements in reversed order.
func Reverse[S ~[]E, E any](s S) S {
	d := make(S, len(s))
	for i := 0; i < len(s); i++ {
		d[i] = s[len(s)-i-1]
	}

	return d
}
