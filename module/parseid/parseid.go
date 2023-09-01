package parseid

import (
	"embed"
	"fmt"
	"github.com/mavolin/repogen/internal/goimports"
	"github.com/mavolin/repogen/internal/pkgutil"
	"go/types"
	"golang.org/x/tools/go/packages"
	"html/template"
	"os"
)

const outName = "parse_id.repogen.go"

//go:embed *.gotpl
var templates embed.FS

type (
	Data struct {
		Package string

		IDs []ID
	}

	ID struct {
		Type     string
		FuncName string
		Signed   bool
		Bits     int
	}
)

var tpl = template.Must(template.ParseFS(templates, "template.gotpl"))

func Generate(pkg *packages.Package) error {
	ids, err := findIDs(pkg)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		_ = os.Remove(outName)
		return nil
	}

	out, err := os.Create(outName)
	if err != nil {
		return wrapErr(err)
	}

	in, done, err := goimports.Pipe(out)

	data := Data{
		Package: pkg.Name,
		IDs:     ids,
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

func findIDs(pkg *packages.Package) ([]ID, error) {
	scope := pkg.Types.Scope()
	ids := make([]ID, 0, len(scope.Names()))

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)

		dirs := pkgutil.FindDirectives(pkg, obj, "parseid")
		if len(dirs) == 0 {
			continue
		} else if len(dirs) > 1 {
			return nil, objErr(pkg, obj, "conflicting directives, only use a single parseid directive")
		}

		t := pkgutil.BaseType(obj.Type())
		basic, ok := t.(*types.Basic)
		if !ok || basic.Info()&types.IsInteger == 0 {
			return nil, objErr(pkg, obj, "type must be `(int|uint)(8|16|32|64)?`")
		}

		id := ID{
			Type:     obj.Name(),
			FuncName: "Parse" + obj.Name(),
			Signed:   basic.Info()&types.IsUnsigned != 0,
		}
		if dirs[0].Args != "" {
			id.FuncName = dirs[0].Args
		}

		switch basic.Kind() {
		case types.Uint8, types.Int8:
			id.Bits = 8
		case types.Uint16, types.Int16:
			id.Bits = 16
		case types.Uint32, types.Int32:
			id.Bits = 32
		case types.Uint64, types.Uint, types.Int64, types.Int:
			id.Bits = 64
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("parseid: %w", err)
}

func objErr(pkg *packages.Package, obj types.Object, s string) error {
	return pkgutil.PosError(pkg, obj.Pos(), fmt.Errorf("parseid: %s: %s", obj.Name(), s))
}
