package planning

import "reflect"

// deepCopy returns a copy of v that shares no map, slice or pointer with it:
// a memoized State goes to callers that change its maps and slices. It walks
// the value by reflection, so a field added later is copied too
// (TestBuild_MemoCopiesShareNothing checks it). Unexported fields (time.Time's
// location) are copied as they are; they are immutable.
func deepCopy[T any](v T) T {
	src := reflect.ValueOf(&v).Elem()
	dst := reflect.New(src.Type()).Elem()
	copyValue(dst, src)
	return dst.Interface().(T)
}

func copyValue(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		p := reflect.New(src.Type().Elem())
		copyValue(p.Elem(), src.Elem())
		dst.Set(p)
	case reflect.Map:
		if src.IsNil() {
			return
		}
		m := reflect.MakeMapWithSize(src.Type(), src.Len())
		it := src.MapRange()
		for it.Next() {
			k := reflect.New(src.Type().Key()).Elem()
			copyValue(k, it.Key())
			e := reflect.New(src.Type().Elem()).Elem()
			copyValue(e, it.Value())
			m.SetMapIndex(k, e)
		}
		dst.Set(m)
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		s := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := range src.Len() {
			copyValue(s.Index(i), src.Index(i))
		}
		dst.Set(s)
	case reflect.Struct:
		dst.Set(src) // unexported fields as they are
		for i := range src.NumField() {
			if dst.Field(i).CanSet() {
				copyValue(dst.Field(i), src.Field(i))
			}
		}
	case reflect.Interface:
		if src.IsNil() {
			return
		}
		e := reflect.New(src.Elem().Type()).Elem()
		copyValue(e, src.Elem())
		dst.Set(e)
	default:
		dst.Set(src)
	}
}
