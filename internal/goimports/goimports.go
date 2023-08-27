package goimports

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
)

func Pipe(out io.Writer) (in io.WriteCloser, done func() error, err error) {
	var stderr bytes.Buffer
	stderr.Grow(2048)

	goimports := exec.Command("goimports")
	goimports.Stdout = out
	goimports.Stderr = &stderr
	in, err = goimports.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	if err := goimports.Start(); err != nil {
		return nil, nil, err
	}
	return in, func() error {
		err := goimports.Wait()
		if stderr.Len() > 0 {
			return errors.New(stderr.String())
		}

		return err
	}, nil
}
