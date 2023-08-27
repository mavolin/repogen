package crud

import (
	"embed"
	"errors"
	"fmt"
	"github.com/iancoleman/strcase"
	"go/types"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
	"repogen/internal/goimports"
	"repogen/internal/pkgutil"
	"repogen/internal/util"
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
		return fmt.Errorf("crud: %w", err)
	}

	in, done, err := goimports.Pipe(out)

	data := Data{
		Package:  pkg.Name,
		Entities: es,
		Extra:    extra,
		Base:     base,
	}

	if err := tpl.Execute(in, data); err != nil {
		return fmt.Errorf("crud: %w", err)
	}

	if err := in.Close(); err != nil {
		return fmt.Errorf("crud: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("crud: %w", err)
	}

	return done()
}

func findExtra(pkg *packages.Package, packagePath string) ([]string, error) {
	var extra []string

	pkg.Types.Scope().Len()

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

		s, ok := pkgutil.BaseType(obj.Type()).(*types.Struct)
		if !ok {
			return nil, pkgutil.PosError(pkg, obj.Pos(), errors.New("crud: cannot generate interface for non-struct type"))
		}

		e := Entity{
			Repository: obj.Name() + "Repository",
			Singular:   obj.Name(),
			Plural:     util.Plural(pkg, obj),
			SearchType: obj.Name() + "SearchData",
		}

		for _, dir := range dirs {
			switch dir.Directive {
			case "":
				if err := parsePrimary(pkg, obj, &e, dir.Args); err != nil {
					return nil, err
				}
			case "extra":
				e.Extra = append(e.Extra, dir.Args)
			case "search":
				e.SearchType = dir.Args
			case "repository":
				e.Repository = dir.Args
			case "*by":
				e.CreatedByType, e.UpdatedByType, e.DeletedByType = dir.Args, dir.Args, dir.Args
			case "createdby":
				e.CreatedByType = dir.Args
			case "updatedby":
				e.UpdatedByType = dir.Args
			case "deletedby":
				e.DeletedByType = dir.Args
			default:
				return nil, pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("%s: crud: unrecognized directive %q", obj.Name(), dir.Directive))
			}
		}

		var err error
		e.PKs, err = findPKs(pkg, obj, s)
		if err != nil {
			return nil, err
		}
		if len(e.PKs) == 0 {
			return nil, pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("%s: crud: need at least one pk", obj.Name()))
		}

		if e.CreatedByType == "" {
			e.CreatedByType, err = findUpdatedByType(pkg, obj, s, "CreatedBy")
			if err != nil {
				return nil, err
			}
		}
		if e.UpdatedByType == "" {
			e.UpdatedByType, err = findUpdatedByType(pkg, obj, s, "UpdatedBy")
			if err != nil {
				return nil, err
			}
		}
		if e.DeletedByType == "" {
			e.DeletedByType, err = findUpdatedByType(pkg, obj, s, "CreatedBy")
			if err != nil {
				return nil, err
			}
		}

		es = append(es, e)
	}

	return es, nil
}

func parsePrimary(pkg *packages.Package, obj types.Object, e *Entity, args string) error {
	if len(args) == 0 {
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
			return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("unknown crud operation %q", op))
		}
	}

	return nil
}

func findPKs(pkg *packages.Package, obj types.Object, s *types.Struct) ([]Param, error) {
	pks := make([]Param, 0, 3)

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		tag := util.ParseStructTag(s.Tag(i))
		if tag == nil {
			continue
		}

		if _, pk := tag["pk"]; !pk {
			continue
		}

		pk := Param{
			Name: strcase.ToLowerCamel(f.Name()),
			Type: pkgutil.TypeName(pkg, f.Type()),
		}
		if pk.Type == "" {
			return nil, pkgutil.PosError(pkg, obj.Pos(), errors.New("pk must be a named type"))
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

		tag := util.ParseStructTag(s.Tag(i))
		name := tag["unrel"]
		if name != "" {
			return name, nil
		}

		name = pkgutil.TypeName(pkg, f.Type())
		if name == "" {
			return "", pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("%s must be a named type", name))
		}

		return name, nil
	}

	return "", nil
}
