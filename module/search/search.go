package search

import (
	"embed"
	"errors"
	"fmt"
	"go/types"
	"golang.org/x/tools/go/packages"
	"os"
	"repogen/internal/goimports"
	"repogen/internal/pkgutil"
	"repogen/internal/util"
	"strings"
	"text/template"
)

const outName = "search.repogen.go"

//go:embed *.gotpl
var templates embed.FS

var tpl = template.Must(template.ParseFS(templates, "template.gotpl"))

type (
	Data struct {
		Package  string
		Entities []Entity
	}

	Entity struct {
		SearchType string
		Fields     []Field
	}
	Field struct {
		Name string
		Type string
	}
)

func Generate(pkg *packages.Package) error {
	es, err := findEntities(pkg)
	if err != nil {
		return err
	}

	if len(es) == 0 {
		_ = os.Remove(outName)
		return nil
	}

	out, err := os.Create(outName)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	in, done, err := goimports.Pipe(out)

	data := Data{
		Package:  pkg.Name,
		Entities: es,
	}

	if err := tpl.Execute(in, data); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if err := in.Close(); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	return done()
}

func findEntities(pkg *packages.Package) ([]Entity, error) {
	scope := pkg.Types.Scope()
	es := make([]Entity, 0, len(scope.Names()))

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)

		dirs := pkgutil.FindDirectives(pkg, obj, "search")
		if len(dirs) == 0 {
			continue
		}

		s, ok := pkgutil.BaseType(obj.Type()).(*types.Struct)
		if !ok {
			return nil, pkgutil.PosError(pkg, obj.Pos(), errors.New("search: cannot generate interface for non-struct type"))
		}

		e := Entity{
			SearchType: obj.Name() + "SearchData",
		}

		for _, dir := range dirs {
			switch dir.Directive {
			case "":
				if dir.Args != "" {
					e.SearchType = dir.Args
				}
			case "extra":
				name, typ, _ := strings.Cut(dir.Args, " ")
				e.Fields = append(e.Fields, Field{Name: name, Type: typ})
			default:
				return nil, pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("%s: search: unrecognized directive %q", obj.Name(), dir.Directive))
			}
		}

		fields, err := findSearchFields(pkg, obj, s)
		if err != nil {
			return nil, err
		}

		e.Fields = append(e.Fields, fields...)
		es = append(es, e)
	}

	return es, nil
}

func findSearchFields(pkg *packages.Package, obj types.Object, s *types.Struct) ([]Field, error) {
	fields := make([]Field, 0, s.NumFields())

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		tag := util.ParseStructTag(s.Tag(i))
		if tag == nil {
			continue
		}

		search, ok := tag["search"]
		if !ok {
			continue
		}

		var typ string
		unptr, ok := pkgutil.Unptr(pkg, f.Type())
		if unptr == "" {
			return nil, pkgutil.PosError(pkg, obj.Pos(),
				fmt.Errorf("%s.%s: search: cannot search for not named type", obj.Name(), f.Name()))
		}

		if ok {
			typ = "omitnull.Val[" + unptr + "]"
		} else {
			typ = "omit.Val[" + unptr + "]"
		}

		split := strings.Split(search, " ")
		switch split[0] {
		case "":
			fields = append(fields, Field{
				Name: f.Name(),
				Type: typ,
			})
		case "range":
			if len(split) != 3 && len(split) != 1 {
				return nil, pkgutil.PosError(pkg, obj.Pos(),
					fmt.Errorf("%s.%s: search: invalid range directive, need two or no names after range", obj.Name(), f.Name()))
			}

			if len(split) == 1 {
				fields = append(fields,
					Field{Name: f.Name() + "From", Type: typ}, Field{Name: f.Name() + "Until", Type: typ})
				continue
			}

			fields = append(fields, Field{Name: split[1], Type: typ}, Field{Name: split[2], Type: typ})
		default:
			return nil, pkgutil.PosError(pkg, obj.Pos(),
				fmt.Errorf("%s.%s: search: invalid search directive %q", obj.Name(), f.Name(), search))
		}
	}

	return fields, nil
}
