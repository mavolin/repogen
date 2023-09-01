package crud

import (
	"embed"
	"fmt"
	"github.com/iancoleman/strcase"
	"github.com/mavolin/repogen/internal/goimports"
	"github.com/mavolin/repogen/internal/pkgutil"
	"github.com/mavolin/repogen/internal/util"
	"go/types"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const outName = "crud.repogen.go"

//go:embed *.gotpl
var templates embed.FS

var tpl = template.Must(template.ParseFS(templates, "template.gotpl"))

type (
	Data struct {
		Package  string
		Entities []Entity
		Extra    []string
		Base     []string
	}

	Entity struct {
		Repository                                  string
		Singular, Plural                            string
		Create, Get, Search, Edit, Delete           bool
		CreatedByType, UpdatedByType, DeletedByType string
		Extra                                       []string
		SearchType                                  string

		PKs []Param
	}
	Param struct {
		Name string
		Type string
	}
)

func Generate(pkg *packages.Package, packagePath string) error {
	es, err := findEntities(pkg)
	if err != nil {
		return err
	}

	if len(es) == 0 {
		_ = os.Remove(outName)
		return nil
	}

	extra, err := findExtra(pkg, packagePath)
	if err != nil {
		return err
	}

	base, err := findBase(pkg, packagePath)
	if err != nil {
		return err
	}

	out, err := os.Create(outName)
	if err != nil {
		return wrapErr(err)
	}

	in, done, err := goimports.Pipe(out)

	data := Data{
		Package:  pkg.Name,
		Entities: es,
		Extra:    extra,
		Base:     base,
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

func findExtra(pkg *packages.Package, packagePath string) ([]string, error) {
	var extra []string

	for i, path := range pkg.CompiledGoFiles {
		if filepath.Dir(path) != packagePath {
			continue
		}

		file := pkg.Syntax[i]
		for _, cg := range file.Comments {
			for _, dir := range pkgutil.ParseDirectives(cg) {
				if dir.Module == "repo" && dir.Directive == "extra" {
					extra = append(extra, dir.Args)
				}
			}
		}
	}

	return extra, nil
}

func findBase(pkg *packages.Package, packagePath string) ([]string, error) {
	var base []string

	for i, path := range pkg.CompiledGoFiles {
		if filepath.Dir(path) != packagePath {
			continue
		}

		file := pkg.Syntax[i]
		for _, cg := range file.Comments {
			for _, dir := range pkgutil.ParseDirectives(cg) {
				if dir.Module == "repo" && dir.Directive == "base" {
					base = append(base, dir.Args)
				}
			}
		}
	}

	return base, nil
}

func findEntities(pkg *packages.Package) ([]Entity, error) {
	scope := pkg.Types.Scope()
	es := make([]Entity, 0, len(scope.Names()))

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)

		dirs := pkgutil.FindDirectives(pkg, obj, "crud")
		if len(dirs) == 0 {
			continue
		}

		s, ok := pkgutil.ElemType(obj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, obj, "cannot generate interface for non-struct type")
		}

		e := Entity{
			Repository: obj.Name() + "Repository",
			Singular:   obj.Name(),
			Plural:     util.Plural(pkg, obj),
			Create:     true,
			Get:        true,
			Search:     true,
			Edit:       true,
			Delete:     true,
			SearchType: obj.Name() + "SearchData",
		}

		sdirs := pkgutil.FindDirectives(pkg, obj, "search")
		for i := len(sdirs) - 1; i >= 0; i-- {
			sdir := sdirs[i]
			if sdir.Directive == "" {
				if sdir.Args != "" {
					e.SearchType = sdir.Args
				}
				break
			}
		}

		for _, dir := range dirs {
			switch dir.Directive {
			case "":
				if dir.Args != "" {
					e.Repository = dir.Args
				}
			case "extra":
				e.Extra = append(e.Extra, dir.Args)
			case "ops":
				if err := parseOps(pkg, obj, &e, dir.Args); err != nil {
					return nil, err
				}
			default:
				return nil, objErr(pkg, obj, fmt.Sprintf("unrecognized directive %q", dir.Directive))
			}
		}

		var err error
		e.PKs, err = findPKs(pkg, obj, s)
		if err != nil {
			return nil, err
		}
		if len(e.PKs) == 0 {
			return nil, objErr(pkg, obj, "need at least one pk")
		}

		e.CreatedByType, err = findUpdatedByType(pkg, obj, s, "CreatedBy")
		if err != nil {
			return nil, err
		}
		e.UpdatedByType, err = findUpdatedByType(pkg, obj, s, "UpdatedBy")
		if err != nil {
			return nil, err
		}
		e.DeletedByType, err = findUpdatedByType(pkg, obj, s, "CreatedBy")
		if err != nil {
			return nil, err
		}

		es = append(es, e)
	}

	return es, nil
}

func parseOps(pkg *packages.Package, obj types.Object, e *Entity, args string) error {
	if args == "" {
		e.Create, e.Get, e.Search, e.Edit, e.Delete = true, true, true, true, true
		return nil
	}

	ops := strings.Split(args, " ")
	for _, op := range ops {
		switch op {
		case "create":
			e.Create = true
		case "get":
			e.Get = true
		case "search":
			e.Search = true
		case "edit":
			e.Edit = true
		case "delete":
			e.Delete = true
		default:
			return objErr(pkg, obj, fmt.Sprintf("unknown crud operation %q", op))
		}
	}

	return nil
}

func findPKs(pkg *packages.Package, obj types.Object, s *types.Struct) ([]Param, error) {
	pks := make([]Param, 0, 3)

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		tag := util.ParseStructTag(s.Tag(i))
		if _, pk := tag["pk"]; !pk {
			continue
		}

		pk := Param{
			Name: strcase.ToLowerCamel(f.Name()),
			Type: pkgutil.NameInPackage(pkg, f.Type()),
		}
		if pk.Type == "" {
			return nil, objErr(pkg, obj, "pk must be a named type")
		}

		pks = append(pks, pk)
	}

	return pks, nil
}

func findUpdatedByType(pkg *packages.Package, obj types.Object, s *types.Struct, name string) (string, error) {
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		if f.Name() != name {
			continue
		}

		settyp := util.Settyp(pkg, pkg, s.Tag(i), f.Type())
		if settyp == nil {
			return "", objErr(pkg, obj, fmt.Sprintf("%s must be a named type", name))
		}

		return settyp.Unptr(), nil
	}

	return "", nil
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("crud: %w", err)
}

func objErr(pkg *packages.Package, obj types.Object, s string) error {
	return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("crud: %s: %s", obj.Name(), s))
}
