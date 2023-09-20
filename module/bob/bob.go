package bob

import (
	"embed"
	"errors"
	"fmt"
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

const outName = "bob.repogen.go"

//go:embed *.gotpl
var templates embed.FS

var tpl = template.Must(template.ParseFS(templates, "template.gotpl"))

type (
	Data struct {
		ModelsPackage string
		RepoPackage   string

		Entities []Entity
	}

	Entity struct {
		GetterName       string
		QualGetterName   string
		SetterName       string
		QualSetterName   string
		ModelsGetterName string
		ModelsSetterName string

		NoUnwrap, NoWrap bool

		Fields []Field
	}
	Field struct {
		GetterName string
		GetterType Type
		SetterName string
		SetterType SetterType

		ModelsName string
		ModelsType Type

		RelName string
		RelType Type

		NoUnwrap, NoWrap bool

		UnwrapFunc, WrapFunc string
	}
	Type struct {
		// ex 1: type Foo []string, ex 2: type Bar []*string
		Type     string // 1: Foo, 2: Bar
		Elem     string // 1: string, 2: *string
		TrueElem string // 1: string, 2: string

		IsNullable bool
		IsArray    bool
	}
	SetterType struct {
		Type       string
		Elem       string
		IsNullable bool
		IsArray    bool
	}
)

func Generate(pkg *packages.Package, packagePath string) error {
	mdirs, err := findModelsDirectives(pkg, packagePath)
	if err != nil {
		return err
	}

	for _, mdir := range mdirs {
		es, err := findEntities(pkg, mdir)
		if err != nil {
			return err
		}

		path := filepath.Join(filepath.FromSlash(mdir.Path), outName)

		if len(es) == 0 {
			_ = os.Remove(path)
			return nil
		}

		out, err := os.Create(path)
		if err != nil {
			return wrapErr(err)
		}

		in, done, err := goimports.Pipe(out)

		data := Data{
			ModelsPackage: mdir.Pkg.Name,
			RepoPackage:   pkg.Name,
			Entities:      es,
		}

		if err := tpl.Execute(in, data); err != nil {
			return wrapErr(err)
		}

		if err := in.Close(); err != nil {
			return wrapErr(err)
		}

		if err = done(); err != nil {
			// so the user can make sense of goimports err
			_ = tpl.Execute(out, data)
			_ = out.Close()
			return wrapErr(err)
		}

		if err := out.Close(); err != nil {
			return wrapErr(err)
		}

		return nil
	}

	return nil
}

type ModelsDirective struct {
	Path string
	Pkg  *packages.Package
}

func findModelsDirectives(pkg *packages.Package, packagePath string) ([]ModelsDirective, error) {
	var models []ModelsDirective

	for i, path := range pkg.CompiledGoFiles {
		if filepath.Dir(path) != packagePath {
			continue
		}

		file := pkg.Syntax[i]
		for _, cg := range file.Comments {
			for _, dir := range pkgutil.ParseDirectives(cg) {
				if dir.Module != "bob" || dir.Directive != "models" {
					continue
				}

				load, err := packages.Load(&packages.Config{
					Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps,
				}, "."+string(filepath.Separator)+filepath.FromSlash(dir.Args))
				if err != nil {
					return nil, pkgutil.PosError(pkg, cg.Pos(), fmt.Errorf("%s: failed to load: %w", dir.Args, err))
				}

				if len(load) == 0 {
					return nil, pkgutil.PosError(pkg, cg.Pos(), errors.New("%s: failed to load directory as package"))
				} else if len(load) != 1 {
					return nil, pkgutil.PosError(pkg, cg.Pos(),
						errors.New("%s: expected to only load a single package"))
				}

				models = append(models, ModelsDirective{Path: dir.Args, Pkg: load[0]})
			}
		}
	}

	return models, nil
}

func findEntities(pkg *packages.Package, mdir ModelsDirective) ([]Entity, error) {
	scope := pkg.Types.Scope()
	es := make([]Entity, 0, len(scope.Names()))

	for _, name := range scope.Names() {
		getterObj := scope.Lookup(name)

		dirs := pkgutil.FindDirectives(pkg, getterObj, "bob")
		if len(dirs) == 0 {
			continue
		}

		getter, ok := pkgutil.ElemType(getterObj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, getterObj, "cannot generate wrapper for non-struct type")
		}

		e := Entity{
			GetterName:     getterObj.Name(),
			QualGetterName: pkgutil.NameInPackage(mdir.Pkg, getterObj.Type()),
		}

		e.SetterName = getterObj.Name() + "Setter"
		setterDirs := pkgutil.FindDirectives(pkg, getterObj, "setter")
		for i := len(setterDirs) - 1; i >= 0; i-- {
			setterDir := setterDirs[i]
			if setterDir.Directive == "" {
				if setterDir.Args != "" {
					e.SetterName = setterDir.Args
				}
				break
			}
		}
		setterObj := pkg.Types.Scope().Lookup(e.SetterName)
		if setterObj == nil {
			return nil,
				objErr(pkg, getterObj,
					fmt.Sprintf("found no setter named %q (did you forget a repogen:setter directive?)", e.SetterName))
		}
		setter, ok := pkgutil.ElemType(setterObj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, setterObj, "setter must be struct")
		}
		e.QualSetterName = pkgutil.NameInPackage(mdir.Pkg, setterObj.Type())

		e.ModelsGetterName = e.GetterName
		for _, dir := range dirs {
			switch dir.Directive {
			case "":
				if dir.Args != "" {
					e.ModelsGetterName = dir.Args
				}
			case "ops":
				e.NoWrap = true
				e.NoUnwrap = true

				for _, s := range strings.Split(dir.Args, " ") {
					switch s {
					case "unwrap":
						e.NoUnwrap = false
					case "wrap":
						e.NoWrap = false
					default:
						return nil,
							objErr(pkg, getterObj, fmt.Sprintf("invalid op %q", s))
					}
				}
			default:
				return nil, objErr(pkg, getterObj, fmt.Sprintf("unrecognized directive %q", dir.Directive))
			}
		}
		e.ModelsSetterName = e.ModelsGetterName + "Setter"

		modelObj := mdir.Pkg.Types.Scope().Lookup(e.ModelsGetterName)
		if modelObj == nil {
			return nil, objErr(pkg, getterObj, fmt.Sprintf("found no model type named %q", e.ModelsGetterName))
		}

		model, ok := pkgutil.ElemType(modelObj.Type()).(*types.Struct)
		if !ok {
			return nil, objErr(pkg, getterObj, fmt.Sprintf("expected models.%s to be struct", e.ModelsGetterName))
		}

		relationsField := pkgutil.LookupField(model, "R")
		var relations *types.Struct
		if relationsField != nil {
			relations, _ = pkgutil.ElemType(relationsField.Type()).(*types.Struct)
		}

		var err error
		e.Fields, err = findFields(pkg, mdir, getterObj, modelObj, getter, setter, model, relations)
		if err != nil {
			return nil, err
		}

		es = append(es, e)
	}

	return es, nil
}

func findFields(
	pkg *packages.Package, mdir ModelsDirective, getterObj, modelObj types.Object,
	getter, setter, model, relations *types.Struct,
) ([]Field, error) {
	fields := make([]Field, 0, getter.NumFields())

	for i := 0; i < getter.NumFields(); i++ {
		getterf := getter.Field(i)

		tag := util.ParseStructTag(getter.Tag(i))
		if tag["bob"] == "-" || (tag["wrap"] == "-" && tag["unwrap"] == "-") {
			continue
		}

		f := Field{
			GetterName: getterf.Name(),
		}

		getterTyp := resolveType(mdir.Pkg, getterf.Type())
		if getterTyp == nil {
			return nil, objErr(pkg, getterObj, fmt.Sprintf("%s: field must be of named typed", f.GetterName))
		}
		f.GetterType = *getterTyp

		f.SetterName = f.GetterName
		set := tag["set"]
		switch set {
		case "":
			switch getterf.Name() {
			case "ID",
				"CreatedAt", "CreatedBy", "CreatedByID",
				"UpdatedAt", "UpdatedBy", "UpdatedByID",
				"DeletedAt", "DeletedBy", "DeletedByID":
				f.SetterName = ""
				f.NoUnwrap = true
			}
		case "-":
			f.NoUnwrap = true
			f.SetterName = ""
		default:
			f.SetterName = set
		}
		unwrap := tag["unwrap"]
		switch unwrap {
		case "-":
			f.SetterName = ""
			f.NoUnwrap = true
		default:
			f.UnwrapFunc = unwrap
		}

		if !f.NoUnwrap {
			setterf := pkgutil.LookupField(setter, f.SetterName)
			if setterf == nil {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: no field named %q on this type's setter", f.GetterName, f.SetterName))
			}

			settyp := util.Settyp(mdir.Pkg, pkg, getter.Tag(i), getterf.Type())
			f.SetterType = SetterType{
				Type:       settyp.Type,
				Elem:       settyp.Elem(),
				IsNullable: settyp.IsPtr,
				IsArray:    settyp.IsSlice,
			}
			if f.SetterType.IsArray && tag["rel"] != "" {
				f.NoUnwrap = true
			}
		}

		if !(f.SetterType.IsArray && tag["rel"] != "") {
			f.ModelsName = f.GetterName
			if name := tag["bob"]; name != "" {
				f.ModelsName = name
			}

			modelsf := pkgutil.LookupField(model, f.ModelsName)
			if modelsf == nil {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: model has no corresponding field named %q", f.GetterName, f.ModelsName))
			}
			modelsTypeName := pkgutil.NameInPackage(mdir.Pkg, modelsf.Type())
			if modelsTypeName == "" {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: model's corresponding field %q is not of named type", f.GetterName, f.ModelsName))
			}

			modelsType := resolveType(mdir.Pkg, modelsf.Type())
			f.ModelsType = *modelsType
		}

		if rel := tag["rel"]; rel != "" {
			f.RelName = rel

			if relations == nil {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: field declared as relation, but model has no field \".R\"", f.GetterName))
			}

			relf := pkgutil.LookupField(relations, f.RelName)
			if relf == nil {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: field declared as relation, but model has no field \".R.%s\"", f.GetterName,
						f.RelName))
			}
			relType := resolveType(mdir.Pkg, relf.Type())
			if relType == nil {
				return nil, objErr(pkg, getterObj,
					fmt.Sprintf("%s: model's corresponding relation %q is not of named type", f.GetterName, f.RelName))
			}
			f.RelType = *relType
		}

		wrap := tag["wrap"]
		switch wrap {
		case "-":
			f.NoWrap = true
		default:
			f.WrapFunc = wrap
		}

		fields = append(fields, f)
	}

	return fields, nil
}

func resolveType(pkg *packages.Package, typ types.Type) *Type {
	var t Type

	if named, ok := typ.(*types.Named); ok {
		if named.Obj().Pkg().Name() == "null" && named.Obj().Name() == "Val" {
			typ = named.TypeArgs().At(0)
			t.IsNullable = true
		}
	}

	t.Type = pkgutil.NameInPackage(pkg, typ)
	if t.Type == "" {
		return nil
	}

	cmp := t.Type
	if named, ok := typ.(*types.Named); ok {
		cmp = pkgutil.NameInPackage(pkg, named.Underlying())
	}

	switch {
	case strings.HasPrefix(cmp, "[]"):
		t.IsArray = true
		t.Elem = cmp[2:]
	case strings.HasPrefix(cmp, "*"):
		t.IsNullable = true
		t.Elem = cmp[1:]
	default:
		t.Elem = t.Type
	}
	t.TrueElem = t.Elem

loop:
	for {
		switch {
		case strings.HasPrefix(t.TrueElem, "[]"):
			t.TrueElem = t.TrueElem[2:]
		case strings.HasPrefix(t.TrueElem, "*"):
			t.TrueElem = t.TrueElem[1:]
		default:
			break loop
		}
	}

	return &t
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("bob: %w", err)
}

func objErr(pkg *packages.Package, obj types.Object, s string) error {
	return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("bob: %s: %s", obj.Name(), s))
}
