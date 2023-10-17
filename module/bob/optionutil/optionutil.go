package optionutil

import (
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
)

func SetOmitArray[E any, A, B ~[]E](val omit.Val[A]) omit.Val[B] {
	return omit.Map(val, func(ts A) B { return B(ts) })
}

func SetOmitArrayConvert[EA, EB any, A ~[]EA, B ~[]EB](val omit.Val[A], conv func(EA) EB) omit.Val[B] {
	return omit.Map(val, func(a A) B {
		b := make(B, len(a))
		for i, ea := range a {
			b[i] = conv(ea)
		}
		return b
	})
}

func SetOmitNullArray[E any, A, B ~[]E](val omitnull.Val[A]) omitnull.Val[B] {
	return omitnull.Map(val, func(a A) B { return B(a) })
}

func SetOmitNullArrayConvert[EA, EB any, A ~[]EA, B ~[]EB](val omitnull.Val[A], conv func(EA) EB) omitnull.Val[B] {
	return omitnull.Map(val, func(a A) B {
		b := make(B, len(a))
		for i, ea := range a {
			b[i] = conv(ea)
		}
		return b
	})
}

func ConvertNullPtr[A any, B any](val null.Val[A], conv func(A) B) *B {
	if val.IsNull() {
		return nil
	}

	b := conv(val.GetOrZero())
	return &b
}

func ConvertSlice[EA, EB any, A ~[]EA, B ~[]EB](a A, conv func(EA) EB) B {
	if a == nil {
		return B{} // yes, this is correct
	}

	b := make(B, len(a))
	for i, ea := range a {
		b[i] = conv(ea)
	}
	return b
}

func ConvertNullSlice[EA, EB any, A ~[]EA, B ~[]EB](val null.Val[A], conv func(EA) EB) B {
	if val.IsNull() {
		return nil
	}

	a := val.GetOrZero()
	b := make(B, len(a))
	for i, ea := range a {
		b[i] = conv(ea)
	}
	return b
}
