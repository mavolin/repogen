package pkgutil

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"strconv"
	"strings"
)

func PosError(pkg *packages.Package, pos token.Pos, err error) error {
	position := pkg.Fset.Position(pos)
	return fmt.Errorf("%s:%d:%d: %w", position.Filename, position.Line, position.Column, err)
}

func BaseType(t types.Type) types.Type {
	for {
		u := t.Underlying()
		if u == t || u == nil {
			break
		}

		t = u
	}

	return t
}

func FileForPos(pkg *packages.Package, pos token.Pos) *ast.File {
	position := pkg.Fset.Position(pos)

	for _, file := range pkg.Syntax {
		filename := pkg.Fset.Position(file.Name.Pos()).Filename
		if filename == position.Filename {
			return file
		}
	}

	return nil
}

func TypeName(currentPkg *packages.Package, t types.Type) string {
	var b strings.Builder

	for {
		switch typ := t.(type) {
		case *types.Slice:
			b.WriteString("[]")
			t = typ.Elem()
		case *types.Array:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(int(typ.Len())))
			b.WriteByte(']')
			t = typ.Elem()
		case *types.Pointer:
			b.WriteByte('*')
			t = typ.Elem()
		case *types.Named:
			b.WriteString(Qual(currentPkg, typ.Obj()))
			return b.String()
		case *types.Basic:
			b.WriteString(typ.Name())
			return b.String()
		default:
			return ""
		}
	}
}

func Unptr(currentPkg *packages.Package, t types.Type) (unptr string, ok bool) {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return TypeName(currentPkg, t), false
	}

	return TypeName(currentPkg, ptr.Elem()), true
}

func Qual(currentPkg *packages.Package, n *types.TypeName) string {
	if n.Pkg() == nil || n.Pkg().Path() == currentPkg.PkgPath {
		return n.Name()
	}

	return n.Pkg().Name() + "." + n.Name()
}
