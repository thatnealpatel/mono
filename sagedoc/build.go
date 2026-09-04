package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"patel.codes/indexing"
)

func buildIndex(ctx context.Context, configuredPython, path string, diagnostics io.Writer, verify func() error) (err error) {
	builder, err := indexing.Create(path, SageTokenizer{})
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		if abortErr := builder.Abort(); abortErr != nil {
			err = errors.Join(err, fmt.Errorf("abort index build: %w", abortErr))
		}
	}()

	if _, err := runExtractor(ctx, configuredPython, builder, diagnostics); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := builder.Close(); err != nil {
		return fmt.Errorf("publish index: %w", err)
	}
	complete = true
	return nil
}
