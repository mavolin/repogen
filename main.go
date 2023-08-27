package main

import (
	"errors"
	"golang.org/x/tools/go/packages"
	"path/filepath"
	"repogen/module/crud"
	"repogen/module/parseid"
	"repogen/module/search"
	"repogen/module/setter"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	load, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedDeps | packages.NeedCompiledGoFiles | packages.NeedSyntax,
	}, ".")
	if err != nil {
		return err
	}

	if len(load) == 0 {
		return errors.New("failed to load current directory (does it contain any go files?)")
	} else if len(load) != 1 {
		return errors.New("expected to only load a single package")
	}

	pkg := load[0]
	if len(pkg.CompiledGoFiles) != len(pkg.Syntax) {
		return errors.New("len(CompiledGoFiles) != len(Syntax)")
	}

	wd, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	errChan := make(chan error)

	go func() { errChan <- parseid.Generate(pkg) }()
	go func() { errChan <- crud.Generate(pkg, wd) }()
	go func() { errChan <- search.Generate(pkg) }()
	go func() { errChan <- setter.Generate(pkg) }()

	var errs []error
	for i := 0; i < 4; i++ {
		if err := <-errChan; err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}