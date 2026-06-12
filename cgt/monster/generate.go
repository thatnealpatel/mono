//go:build generate

package monster

// Code generators for the monster package. Run all of them with:
//
//	go generate ./...    # from the cgt/ directory
//
// Each generator is a package main program under an underscore-
// prefixed directory (cgt/_gen, cgt/_api) so it stays out of the
// normal package graph (go build/test/list ./...). go generate
// reaches these directives because this file lives in the monster
// package directory. `go run -C <dir>` runs the generator with <dir>
// as its working directory, so the hidden generator dirs are reached
// via `-C ../_gen` / `-C ../_api` and every -out path is written
// relative to that generator dir (the cgt/_gen directory), not this
// one: tables that belong in the monster package land at
// ../monster/<file>.

// Derived tables (verified against checked-in goldens by _gen):
//go:generate go run -C ../_gen . -out ../mat24/mat24_gen.go
//go:generate go run -C ../_gen . -out ../monster/mm_op_p_gen.go
//go:generate go run -C ../_gen . -out ../monster/monster_order_gen.go

// The xi operation tables are no longer generated: the monster
// package builds all twenty (xiPerm00..xiPerm41 and
// xiSign00..xiSign41) at init from generator.XiOpXiShort
// (see mm_op_xi.go). The independent first-principles
// derivation that formerly verified the emitted file now
// runs as a regression cross-check in
// mm_op_xi_regress_test.go on every `go test`.

// API-surface manifests (cython.yaml, python.yaml, go.yaml in _api):
//go:generate go run -C ../_api .
