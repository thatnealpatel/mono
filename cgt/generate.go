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
//go:generate go run -C _gen . -out ../mm_op_p_gen.go
//go:generate go run -C _gen . -out ../monster_order_gen.go

// The xi operation tables are no longer generated: package
// cgt builds all twenty (xiPerm00..xiPerm41 and
// xiSign00..xiSign41) at init from generator.XiOpXiShort
// (see mm_op_xi.go). The independent first-principles
// derivation that formerly verified the emitted file now
// runs as a regression cross-check in
// mm_op_xi_regress_test.go on every `go test`.

// API-surface manifests (cython.yaml, python.yaml, go.yaml in _api):
//go:generate go run -C _api .
