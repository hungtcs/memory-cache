package memory_cache

import (
	"fmt"
	"reflect"
)

func SafeCall(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	fn()

	return nil
}

func Pointer[T any](v T) *T {
	return &v
}

func IsZeroValue(v any) bool {
	rv := reflect.ValueOf(v)
	zero := reflect.Zero(rv.Type())
	return reflect.DeepEqual(rv.Interface(), zero.Interface())
}
