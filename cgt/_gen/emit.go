package main

import (
	"bytes"
	"fmt"
)

// Shared rendering infrastructure used by every
// table generator (mat24.go, mm_op_xi.go).
// The xi and mat24 generators differ only in their
// per-table headers (mat24 emits a doc comment and a
// [...] length, xi an explicit [N] length); the
// value-row loop is identical and lives here in
// writeHexTable.

// cType describes a table element type and how it
// maps onto Go.
type cType struct {
	goType string // Go element type, e.g. "uint32".
	width  int    // hex digits to zero-pad each value.
}

// cTable is one computed table ready to render.
type cTable struct {
	goName string   // Go var name, e.g. mat24EncTable0.
	typ    cType    // resolved Go element type.
	values []uint64 // element values.
}

// valsPerLine is the number of array elements
// emitted per source line, matching the golden
// table layout shared by every generated file.
const valsPerLine = 8

// writeHeader writes the generated-file header,
// including the package clause, in the house style
// shared with the xi table generator. The mat24 tables
// live in the cgt/mat24 package.
func writeHeader(buf *bytes.Buffer) {
	buf.WriteString("// Code generated from the Golay code basis. DO NOT EDIT.\n")
	buf.WriteString("//\n")
	buf.WriteString("// Precomputed mat24 tables (derived from the Golay code).\n\n")
	buf.WriteString("package mat24\n")
}

// writeHexTable renders one table as a Go var
// declaration: "var name = [lenSpec]typ{" followed
// by the element values, perLine zero-padded
// (to width hex digits) values per line, then "}".
//
// lenSpec is the array-length text between the
// brackets, e.g. "..." for an ellipsis array or a
// decimal count. The caller is responsible for any
// preceding doc comment.
//
// The output is not yet gofmt-clean; the caller runs
// go/format over the whole file.
func writeHexTable(buf *bytes.Buffer, name, lenSpec, typ string, width, perLine int, vals []uint64) {
	fmt.Fprintf(buf, "\nvar %s = [%s]%s{\n", name, lenSpec, typ)
	for i, v := range vals {
		if i%perLine == 0 {
			buf.WriteByte('\t')
		}
		fmt.Fprintf(buf, "0x%0*x,", width, v)
		if i%perLine == perLine-1 || i == len(vals)-1 {
			buf.WriteByte('\n')
		} else {
			buf.WriteByte(' ')
		}
	}
	buf.WriteString("}\n")
}
