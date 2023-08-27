package setter

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

const outName = "setter.repogen.go"

//go:embed *.gotpl
var templates embed.FS

var tpl = template.Must(template.ParseFS(templates, "template.gotpl"))

type (
	Data struct {
		Package  string
		Entities []Entity
	}

	Entity struct {
		SetterType string
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
			SetterType: obj.Name() + "Setter",
		}
		var extra []Field

		for _, dir := range dirs {
			switch dir.Directive {
			case "":
				if dir.Args != "" {
					e.SetterType = dir.Args
				}
			case "extra":
				name, typ, _ := strings.Cut(dir.Args, " ")
				extra = append(extra, Field{Name: name, Type: typ})
			default:
				return nil, pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("%s: search: unrecognized directive %q", obj.Name(), dir.Directive))
			}
		}

		var err error
		e.Fields, err = listFields(pkg, obj, s)
		if err != nil {
			return nil, err
		}

		e.Fields = append(e.Fields, extra...)
		es = append(es, e)
	}

	return es, nil
}

func listFields(pkg *packages.Package, obj types.Object, s *types.Struct) ([]Field, error) {
	fields := make([]Field, 0, s.NumFields())

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		name := f.Name()
		_ = name

		tag := util.ParseStructTag(s.Tag(i))
		var setter string
		if tag != nil {
			setter = tag["setter"]
		}

		var typ string
		unptr, ok := pkgutil.Unptr(pkg, f.Type())
		if unptr == "" {
			return nil, pkgutil.PosError(pkg, obj.Pos(),
				fmt.Errorf("%s.%s: setter: cannot setter for not named type", obj.Name(), f.Name()))
		}

		if ok {
			typ = "omitnull.Val[" + unptr + "]"
		} else {
			typ = "omit.Val[" + unptr + "]"
		}

		switch setter {
		case "":
			switch f.Name() {
			case "ID":
			case "CreatedAt", "CreatedBy":
			case "UpdatedAt", "UpdatedBy":
			case "DeletedAt", "DeletedBy":
			default:
				fields = append(fields, Field{
					Name: f.Name(),
					Type: typ,
				})
			}
		case "-":
		default:
			fields = append(fields, Field{
				Name: setter,
				Type: typ,
			})
		}
	}

	return fields, nil
}
