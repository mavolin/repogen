package optionutil

import (
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
)

func SetOmit[T any](val omit.Val[T], col string, setCols *[]string, colI uint8, setColsInt *uint64) T {
	if val.IsSet() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI
	}
	return val.GetOrZero()
}

func SetOmitConvert[T any, C any](
	val omit.Val[T], conv func(T) C, col string, setCols *[]string, colI uint8, setColsInt *uint64,
) C {
	if val.IsSet() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI
		return conv(val.MustGet())
	}
	var z C
	return z
}

func SetOmitArray[T any, W ~[]T](val omit.Val[[]T], col string, setCols *[]string, colI uint8, setColsInt *uint64) W {
	if val.IsSet() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI
	}
	return W(val.GetOrZero())
}

func SetOmitArrayConvert[T any, C any, W ~[]C](
	val omit.Val[[]T], conv func(T) C, col string, setCols *[]string, colI uint8, setColsInt *uint64,
) W {
	if val.IsSet() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI

		get := val.GetOrZero()
		if get == nil {
			return nil
		}

		a := make(W, len(get))
		for i, t := range get {
			a[i] = conv(t)
		}
		return a
	}
	return nil
}

func SetOmitNull[T any, W any](
	val omitnull.Val[T], constr func(T, bool) W, col string, setCols *[]string, colI uint8, setColsInt *uint64,
) W {
	if val.IsSet() || val.IsNull() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI
		return constr(val.GetOrZero(), val.IsSet())
	}
	var z W
	return z
}

func SetOmitNullConvert[T any, C any, W any](
	val omitnull.Val[T], constr func(C, bool) W, conv func(T) C, col string, setCols *[]string, colI uint8,
	setColsInt *uint64,
) W {
	if val.IsSet() || val.IsNull() {
		*setCols = append(*setCols, col)
		*setColsInt = 1 << colI
		return constr(conv(val.GetOrZero()), val.IsSet())
	}
	var z W
	return z
}

func ConvertPtr[T any, C any](val *T, conv func(T) C) *C {
	if val == nil {
		return nil
	}

	c := conv(*val)
	return &c
}

func ConvertSlice[T any, TA ~[]T, C any, CA []C](val TA, conv func(T) C) CA {
	if val == nil {
		return nil
	}

	res := make(CA, len(val))
	for i, t := range val {
		res[i] = conv(t)
	}

	return res
}
