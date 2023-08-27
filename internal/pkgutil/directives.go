package pkgutil

import (
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/packages"
	"slices"
	"strings"
)

type RepogenDirective struct {
	// Module is the repogen module this directive is for.
	Module string
	// Directive is the optional directive separated through a colon from the
	// module.
	Directive string

	Args string
}

func ParseDirective(comment string) *RepogenDirective {
	if !strings.HasPrefix(comment, "//repogen:") {
		return nil
	}

	comment = comment[len("//repogen:"):]

	for i, b := range comment {
		switch b {
		case ' ':
			if i == 0 {
				return nil
			}
			return &RepogenDirective{
				Module: comment[:i],
				Args:   comment[i+1:],
			}
		case ':':
			if i == 0 {
				return nil
			}
			directive, args, _ := strings.Cut(comment[i+1:], " ")
			return &RepogenDirective{
				Module:    comment[:i],
				Directive: directive,
				Args:      args,
			}
		}
	}

	return &RepogenDirective{Module: comment}
}

func ParseDirectives(cg *ast.CommentGroup) []RepogenDirective {
	dirs := make([]RepogenDirective, 0, len(cg.List))

	for _, c := range cg.List {
		if dir := ParseDirective(c.Text); dir != nil {
			dirs = append(dirs, *dir)
		}
	}

	if len(dirs) == 0 {
		return nil
	}

	return dirs
}

func FindDirectives(pkg *packages.Package, obj types.Object, mod string) []RepogenDirective {
	file := FileForPos(pkg, obj.Pos())

	for _, cg := range file.Comments {
		cpos := pkg.Fset.Position(cg.End())
		if cpos.Line != pkg.Fset.Position(obj.Pos()).Line-1 {
			continue
		}

		dirs := ParseDirectives(cg)
		return slices.DeleteFunc(dirs, func(dir RepogenDirective) bool {
			return dir.Module != mod
		})
	}

	return nil
}
