//go:build generate

package cgt

// Code generators for cgt. Run all of them with:
//
//	go generate ./...    # from the cgt/ directory
//
// Each generator is a package main program under an underscore-
// prefixed directory (_gen, _api) so it stays out of the normal
// package graph (go build/test/list ./...). go generate reaches
// these directives because this file lives in the cgt package
// directory, then steps into the hidden dirs via `go run -C`.

// Derived tables (verified against checked-in goldens by _gen):
//go:generate go run -C _gen . -out ../mat24/mat24_gen.go
//go:generate go run -C _gen . -out ../mm_op_xi_gen.go
//go:generate go run -C _gen . -out ../mm_op_p_gen.go
//go:generate go run -C _gen . -out ../monster_order_gen.go

// API-surface manifests (cython.yaml, python.yaml, go.yaml in _api):
//go:generate go run -C _api .
