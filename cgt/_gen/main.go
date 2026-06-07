// Command gen derives the precomputed cgt tables
// from first principles (the Golay code basis and
// the per-modulus SWAR layout) and writes them as
// package cgt source. Each generator self-verifies
// its output against the checked-in golden values
// before emitting.
//
// Usage:
//
//	go run -C _gen . -out ../mat24_gen.go
//	go run -C _gen . -out ../mm_op_xi_gen.go
//	go run -C _gen . -out ../mm_op_p_gen.go
//	go run -C _gen . -out ../monster_order_gen.go
//
// -out names both the generator to run (selected by
// the file's basename) and the path to write. The
// golden file each generator verifies against lives
// in the same directory as the output, so that
// directory is taken from -out. The //go:generate
// directives in mat24.go, mm_op.go and mm_op_group.go
// invoke this command from _gen with -out pointing
// one level up into the cgt package directory.
package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
)

// generators maps an -out basename to the function
// that derives that file. cgtDir is passed through
// so generators that verify against golden files can
// locate them; mm_op_p_gen.go ignores it.
var generators = map[string]func(w io.Writer, cgtDir string) error{
	"mat24_gen.go":         genMat24Tables,
	"mm_op_xi_gen.go":      genXiTables,
	"mm_op_p_gen.go":       genMMOpP,
	"monster_order_gen.go": genOrderVector,
}

func main() {
	log.SetPrefix("gen: ")
	log.SetFlags(0)

	out := flag.String("out", "", "output file to generate "+
		"(mat24_gen.go, mm_op_xi_gen.go, mm_op_p_gen.go or "+
		"monster_order_gen.go)")
	flag.Parse()

	if *out == "" {
		log.Fatal("-out is required")
	}
	// The generator is selected by the output's
	// basename; the golden file it verifies against
	// lives in the same directory as the output.
	gen, ok := generators[filepath.Base(*out)]
	if !ok {
		log.Fatalf("unknown -out %q (want one of mat24_gen.go, "+
			"mm_op_xi_gen.go, mm_op_p_gen.go, monster_order_gen.go)", *out)
	}

	var buf bytes.Buffer
	if err := gen(&buf, filepath.Dir(*out)); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
