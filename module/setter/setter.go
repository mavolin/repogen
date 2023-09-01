package setter

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

		dirs := pkgutil.FindDirectives(pkg, obj, "setter")
		if len(dirs) == 0 {
			continue
		}

		s, ok := pkgutil.ElemType(obj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, obj, "cannot generate interface for non-struct type")
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
				return nil, objErr(pkg, obj, fmt.Sprintf("unrecognized directive %q", dir.Directive))
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

		tag := util.ParseStructTag(s.Tag(i))

		name := tag["set"]
		if name == "-" {
			continue
		} else if name == "" {
			switch f.Name() {
			case "ID", "CreatedAt", "CreatedBy", "UpdatedAt", "UpdatedBy", "DeletedAt", "DeletedBy":
				continue
			default:
				name = f.Name()
			}
		}

		settyp := util.Settyp(pkg, pkg, s.Tag(i), f.Type())
		if settyp == nil {
			return nil, pkgutil.PosError(pkg, obj.Pos(),
				fmt.Errorf("%s.%s: setter: cannot create setter for not-named type", obj.Name(), f.Name()))
		}

		fields = append(fields, Field{Name: name, Type: settyp.OptionType()})
	}

	return fields, nil
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("setter: %w", err)
}

func objErr(pkg *packages.Package, obj types.Object, s string) error {
	return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("setter: %s: %s", obj.Name(), s))
}
