package utils

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

var concatFuncs = map[reflect.Type]any{
	TypeOf[string]():        concatStrings,
	TypeOf[int8]():          useLast[int8],
	TypeOf[int16]():         useLast[int16],
	TypeOf[int32]():         useLast[int32],
	TypeOf[int64]():         useLast[int64],
	TypeOf[int]():           useLast[int],
	TypeOf[uint8]():         useLast[uint8],
	TypeOf[uint16]():        useLast[uint16],
	TypeOf[uint32]():        useLast[uint32],
	TypeOf[uint64]():        useLast[uint64],
	TypeOf[uint]():          useLast[uint],
	TypeOf[bool]():          useLast[bool],
	TypeOf[float32]():       useLast[float32],
	TypeOf[float64]():       useLast[float64],
	TypeOf[time.Time]():     useLast[time.Time],
	TypeOf[time.Duration](): useLast[time.Duration],
}

func useLast[T any](s []T) (T, error) {
	return s[len(s)-1], nil
}

func concatStrings(ss []string) (string, error) {
	var n int
	for _, s := range ss {
		n += len(s)
	}

	var b strings.Builder
	b.Grow(n)
	for _, s := range ss {
		_, err := b.WriteString(s)
		if err != nil {
			return "", err
		}
	}

	return b.String(), nil
}

func GetConcatFunc(typ reflect.Type) func(reflect.Value) (reflect.Value, error) {
	if fn, ok := concatFuncs[typ]; ok {
		return func(a reflect.Value) (reflect.Value, error) {
			rvs := reflect.ValueOf(fn).Call([]reflect.Value{a})
			var err error
			if !rvs[1].IsNil() {
				err = rvs[1].Interface().(error)
			}
			return rvs[0], err
		}
	}

	return nil
}

// ConcatItems the caller should ensure len(items) > 1
func ConcatItems[T any](items []T) (T, error) {
	typ := TypeOf[T]()
	v := reflect.ValueOf(items)

	var cv reflect.Value
	var err error

	// handle map kind
	if typ.Kind() == reflect.Map {
		cv, err = concatMaps(v)
	} else {
		cv, err = ConcatSliceValue(v)
	}

	if err != nil {
		var t T
		return t, err
	}

	return cv.Interface().(T), nil
}

func ConcatSliceValue(val reflect.Value) (reflect.Value, error) {
	elmType := val.Type().Elem()

	if val.Len() == 1 {
		return val.Index(0), nil
	}

	f := GetConcatFunc(elmType)
	if f != nil {
		return f(val)
	}

	// if all elements in the slice are empty, return an empty value
	// if there is exactly one non-empty element in the slice, return that non-empty element
	// otherwise, throw an error.
	var filtered reflect.Value
	for i := 0; i < val.Len(); i++ {
		oneVal := val.Index(i)
		if !oneVal.IsZero() {
			if filtered.IsValid() {
				return reflect.Value{}, fmt.Errorf("cannot concat multiple non-zero value of type %s", elmType)
			}

			filtered = oneVal
		}
	}
	if !filtered.IsValid() {
		filtered = reflect.New(elmType).Elem()
	}

	return filtered, nil
}

func concatMaps(ms reflect.Value) (reflect.Value, error) {
	typ := ms.Type().Elem()

	rms := reflect.MakeMap(reflect.MapOf(typ.Key(), TypeOf[[]any]()))
	ret := reflect.MakeMap(typ)

	n := ms.Len()
	for i := 0; i < n; i++ {
		m := ms.Index(i)

		for _, key := range m.MapKeys() {
			vals := rms.MapIndex(key)
			if !vals.IsValid() {
				var s []any
				vals = reflect.ValueOf(s)
			}

			val := m.MapIndex(key)
			vals = reflect.Append(vals, val)
			rms.SetMapIndex(key, vals)
		}
	}

	for _, key := range rms.MapKeys() {
		vals := rms.MapIndex(key)

		anyVals := vals.Interface().([]any)
		if len(anyVals) == 1 {
			ele := anyVals[0]
			if ele == nil { // we cannot SetMapIndex with nil because it will delete the key
				ret.SetMapIndex(key, reflect.Zero(typ.Elem()))
				continue
			}

			ret.SetMapIndex(key, reflect.ValueOf(ele))
			continue
		}

		v, err := toSliceValue(anyVals)
		if err != nil {
			return reflect.Value{}, err
		}

		var cv reflect.Value

		if v.Type().Elem().Kind() == reflect.Map {
			cv, err = concatMaps(v)
		} else {
			cv, err = ConcatSliceValue(v)
		}

		if err != nil {
			return reflect.Value{}, err
		}

		ret.SetMapIndex(key, cv)
	}

	return ret, nil
}

func toSliceValue(vs []any) (reflect.Value, error) {
	typ := reflect.TypeOf(vs[0])

	ret := reflect.MakeSlice(reflect.SliceOf(typ), len(vs), len(vs))
	ret.Index(0).Set(reflect.ValueOf(vs[0]))

	for i := 1; i < len(vs); i++ {
		v := vs[i]
		vt := reflect.TypeOf(v)
		if typ != vt {
			return reflect.Value{}, fmt.Errorf("unexpected slice element type. Got %v, expected %v", typ, vt)
		}

		ret.Index(i).Set(reflect.ValueOf(v))
	}

	return ret, nil
}
