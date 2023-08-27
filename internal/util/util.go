package util

import (
	"go/types"
	"golang.org/x/tools/go/packages"
	"reflect"
	"repogen/internal/pkgutil"
)

func Plural(pkg *packages.Package, obj types.Object) string {
	dirs := pkgutil.FindDirectives(pkg, obj, "plural")
	if len(dirs) == 0 {
		return obj.Name() + "s"
	}

	return dirs[0].Args
}

type StructTag map[string]string

func ParseStructTag(tag string) StructTag {
	tag = reflect.StructTag(tag).Get("repogen")
	if tag == "" {
		return nil
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
