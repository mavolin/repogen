package search

import (
	"embed"
	"fmt"
	"github.com/mavolin/repogen/internal/goimports"
	"github.com/mavolin/repogen/internal/pkgutil"
	"github.com/mavolin/repogen/internal/util"
	"go/types"
	"golang.org/x/tools/go/packages"
	"os"
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
		return wrapErr(err)
	}

	in, done, err := goimports.Pipe(out)

	data := Data{
		Package:  pkg.Name,
		Entities: es,
	}

	if err := tpl.Execute(in, data); err != nil {
		return wrapErr(err)
	}

	if err := in.Close(); err != nil {
		return wrapErr(err)
	}

	if err = done(); err != nil {
		_ = tpl.Execute(out, data) // so the user can make sense of goimports err
		return wrapErr(err)
	}

	if err := out.Close(); err != nil {
		return wrapErr(err)
	}

	return nil
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

		s, ok := pkgutil.ElemType(obj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, obj, "cannot generate interface for non-struct type")
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
				return nil, objErr(pkg, obj, fmt.Sprintf("search: unrecognized directive %q", dir.Directive))
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

	var includeDeleted bool

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		if f.Name() == "DeletedAt" || f.Name() == "DeletedBy" {
			if !includeDeleted {
				fields = append(fields, Field{Name: "IncludeDeleted", Type: "bool"})
				includeDeleted = true
			}
		}

		tag := util.ParseStructTag(s.Tag(i))
		if tag == nil {
			continue
		}

		search, ok := tag["search"]
		if !ok {
			continue
		}

		settyp := util.Settyp(pkg, pkg, s.Tag(i), f.Type())
		if settyp == nil {
			return nil, objErr(pkg, obj, "cannot create setter for non-named type")
		}

		split := strings.Split(search, " ")
		switch split[0] {
		case "":
			fields = append(fields, Field{
				Name: f.Name(),
				Type: settyp.OptionType(),
			})
		case "range":
			if len(split) != 3 && len(split) != 1 {
				return nil, objErr(pkg, obj,
					fmt.Sprintf(`invalid range directive %q, expected "range" or "range <fromVar> <untilVar"`, search))
			}

			if len(split) == 1 {
				fields = append(fields,
					Field{Name: f.Name() + "From", Type: settyp.OptionType()},
					Field{Name: f.Name() + "Until", Type: settyp.OptionType()})
				continue
			}

			fields = append(fields,
				Field{Name: split[1], Type: settyp.OptionType()},
				Field{Name: split[2], Type: settyp.OptionType()})
		default:
			fields = append(fields, Field{
				Name: search,
				Type: settyp.OptionType(),
			})
		}
	}

	return fields, nil
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("search: %w", err)
}

func objErr(pkg *packages.Package, obj types.Object, s string) error {
	return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("search: %s: %s", obj.Name(), s))
}
