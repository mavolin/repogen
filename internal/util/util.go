package util

import (
	"github.com/mavolin/repogen/internal/pkgutil"
	"go/types"
	"golang.org/x/tools/go/packages"
	"reflect"
	"regexp"
	"strings"
)

func Plural(pkg *packages.Package, obj types.Object) string {
	dirs := pkgutil.FindDirectives(pkg, obj, "plural")
	if len(dirs) == 0 {
		return obj.Name() + "s"
	}

	return dirs[0].Args
}

type SettypType struct {
	Type    string
	IsPtr   bool
	IsSlice bool
}

func (t SettypType) Unptr() string {
	if t.IsPtr {
		return t.Type[1:]
	}

	return t.Type
}

func (t SettypType) Unslice() string {
	if t.IsSlice {
		if t.IsPtr {
			return t.Type[3:]
		}

		return t.Type[2:]
	}

	return t.Type
}

func (t SettypType) Elem() string {
	if t.IsSlice {
		return t.Unslice()
	} else if t.IsPtr {
		return t.Unptr()
	}

	return t.Type
}

func (t SettypType) OptionType() string {
	if t.IsPtr {
		return "omitnull.Val[" + t.Unptr() + "]"
	}

	return "omit.Val[" + t.Type + "]"
}

func Settyp(pkg, tagPkg *packages.Package, tagStr string, fieldTyp types.Type) *SettypType {
	tag := ParseStructTag(tagStr)

	settyp := tag["settyp"]
	if settyp != "" {
		isPtr := strings.HasPrefix(settyp, "*")
		var isSlice bool
		if isPtr {
			isSlice = strings.HasPrefix(settyp[1:], "[]")
		} else {
			isSlice = strings.HasPrefix(settyp, "[]")
		}

		if !strings.Contains(settyp, ".") && pkg.PkgPath != tagPkg.PkgPath && !isPrimitive(settyp) {
			var pre string
			if isPtr {
				pre = "*"
				settyp = settyp[1:]
			}
			if isSlice {
				pre = "[]"
				settyp = settyp[2:]
			}

			settyp = pre + tagPkg.Name + "." + settyp
		}

		return &SettypType{Type: settyp, IsPtr: isPtr, IsSlice: isSlice}
	}

	_, isPtr := fieldTyp.(*types.Pointer)
	_, isSlice := fieldTyp.(*types.Slice)
	typ := pkgutil.NameInPackage(pkg, fieldTyp)
	if typ == "" {
		return nil
	}

	if tag["rel"] != "" {
		typ += "Setter"
	}

	return &SettypType{Type: typ, IsPtr: isPtr, IsSlice: isSlice}
}

var primitiveRegexp = regexp.MustCompile(`^*?(?:\[])?(?:bool|string|u?int(?:8|16|32|64)?|float(?:32|64)|complex(?:64|128))$`)

func isPrimitive(s string) bool {
	return primitiveRegexp.MatchString(s)
}

type StructTag map[string]string

func ParseStructTag(tag string) StructTag {
	tag = reflect.StructTag(tag).Get("repogen")
	if tag == "" {
		return StructTag{}
	}

	t := make(StructTag)

	var keyStart int
	var key string
	var valStart int
	for i := 0; i < len(tag); i++ {
		b := tag[i]

		switch b {
		case ' ':
			if valStart > 0 {
				continue
			}

			t[tag[keyStart:i]] = ""
			keyStart = i + 1
		case ':':
			key = tag[keyStart:i]
			keyStart = 0

			if len(tag) <= i+1 || tag[i+1] != '\'' {
				return nil
			}
			valStart = i + 1
			i++
		case '\'':
			if valStart <= 0 {
				return nil
			}

			t[key] = tag[valStart+1 : i]

			key = ""
			valStart = 0

			if i+1 != len(tag) && tag[i+1] != ' ' {
				return nil
			}

			i += 2
			keyStart = i
		}
	}

	if tag[len(tag)-1] != '\'' {
		t[tag[keyStart:]] = ""
	}

	return t
}
