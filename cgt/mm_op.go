// Package cgt: MMVector, the representation
// vectors of the Monster group modulo p, ported
// from the mmgroup C library (mm_aux.c,
// mm_tables.c, mm_tables_xi.c, and the per-modulus
// mm*_op_*.c files). The mm_index.c index-conversion
// layer lives in the mmindex subpackage.
package cgt

import (
	"patel.codes/cgt/mat24"
	"patel.codes/cgt/mmindex"
)

// Tag values for basis vectors. The numeric codes
// match the sparse encoding tag field (bits 27..25
// of a sparse index, here as the bare 1..7 value).
type Tag uint8

const (
	TagA Tag = 1
	TagB Tag = 2
	TagC Tag = 3
	TagT Tag = 4
	TagX Tag = 5
	TagZ Tag = 6
	TagY Tag = 7
)

// Tuple is a scaled basis vector (factor, tag, i0,
// i1) as accepted by NewVector.
type Tuple struct {
	Factor int
	Tag    Tag
	I0     int
	I1     int
}

// MMVector is a vector of the 196884-dimensional
// representation of the Monster modulo p, stored in
// internal representation. The data slice has
// length MMVSize(p)+1; the final guard entry holds
// protectOverflow and is never touched by the
// operations.
type MMVector struct {
	p    int
	data []uint64
}

// Tag-offset and tag-code aliases for the constants
// the mm_index.c layer now owns (package mmindex).
// They are kept unexported here so the
// constants-only consumers in flat cgt (mm_op_eval.go,
// mm_op_t.go, mm_op_group.go, monster_gx0.go,
// mm_op_aux.go, mm_op_vector.go, monster_order.go,
// and the generated mm_op_p_gen.go) need no edits and
// regeneration stays byte-identical.
const (
	// Internal-representation tag offsets. C enum
	// MM_AUX_OFS.
	mmAuxOfsA = mmindex.OfsA
	mmAuxOfsB = mmindex.OfsB
	mmAuxOfsC = mmindex.OfsC
	mmAuxOfsT = mmindex.OfsT
	mmAuxOfsX = mmindex.OfsX
	mmAuxOfsZ = mmindex.OfsZ
	mmAuxOfsY = mmindex.OfsY
	mmAuxLenV = mmindex.LenV

	// External-representation tag offsets. C enum
	// MM_AUX_XOFS.
	mmAuxXofsD = mmindex.XofsD
	mmAuxXofsA = mmindex.XofsA
	mmAuxXofsB = mmindex.XofsB
	mmAuxXofsC = mmindex.XofsC
	mmAuxXofsT = mmindex.XofsT
	mmAuxXofsX = mmindex.XofsX
	mmAuxXofsZ = mmindex.XofsZ
	mmAuxXofsY = mmindex.XofsY
	mmAuxXlenV = mmindex.XlenV

	// Sparse-representation tag codes (bits 27..25
	// pre-shifted). C enum MM_SPACE_TAG.
	mmSpaceTagA = mmindex.SpaceTagA
	mmSpaceTagB = mmindex.SpaceTagB
	mmSpaceTagC = mmindex.SpaceTagC
	mmSpaceTagT = mmindex.SpaceTagT
	mmSpaceTagX = mmindex.SpaceTagX
	mmSpaceTagZ = mmindex.SpaceTagZ
	mmSpaceTagY = mmindex.SpaceTagY
)

// protectOverflow is the guard value stored in the
// last data entry. C: low 64 bits of (17 << 64) / 19.
const protectOverflow = 0xe50d79435e50d794

// mmvConstTable holds, for each modulus, a packed
// constant word. Indexed by ((p+1)*232 >> 8) & 7. C
// table MMV_CONST_TABLE.
var mmvConstTable = [8]uint32{
	0x00044643, 0x00000000, 0x00034643, 0x00011305,
	0x0003c643, 0x0002c643, 0x00022484, 0x0001a484,
}

// mmAuxTblReduce supplies the masks used by the
// reduce step. C table MM_AUX_TBL_REDUCE.
var mmAuxTblReduce = [14]uint64{
	0x5555555555555555, 0x5555555555555555,
	0x1111111111111111, 0x7777777777777777,
	0x1111111111111111, 0x3333333333333333,
	0x0101010101010101, 0x1f1f1f1f1f1f1f1f,
	0x0101010101010101, 0x3f3f3f3f3f3f3f3f,
	0x0101010101010101, 0x7f7f7f7f7f7f7f7f,
	0x0101010101010101, 0x0f0f0f0f0f0f0f0f,
}

// mmvConst returns the packed constant word for
// modulus p. C macro MMV_LOAD_CONST.
func mmvConst(p int) uint32 {
	return mmvConstTable[(((p)+1)*232>>8)&7]
}

// mmAuxBadP reports whether p is an illegal
// modulus. C macro mm_aux_bad_p.
func mmAuxBadP(p int) bool {
	u := uint32(p)
	return (u&(u+1))|((u-3)&uint32(0xffffff00)) != 0
}

// logIntFields returns LOG_INT_FIELDS for p (number
// of entries per uint64 is 1<<logIntFields).
func logIntFields(p int) uint { return uint(mmvConst(p) & 7) }

// logFieldBits returns LOG_FIELD_BITS for p (each
// field is 1<<logFieldBits bits wide).
func logFieldBits(p int) uint { return uint((mmvConst(p) >> 9) & 3) }

// fieldBits returns FIELD_BITS for p.
func fieldBits(p int) uint { return uint((mmvConst(p) >> 11) & 15) }

// intFields returns INT_FIELDS for p.
func intFields(p int) uint { return uint((mmvConst(p) >> 3) & 63) }

// pBits returns P_BITS for p.
func pBits(p int) uint { return uint((mmvConst(p) >> 15) & 15) }

// Characteristics returns the supported moduli p. C
// table MM_OP_P_TABLE.
func Characteristics() []int {
	return []int{3, 7, 15, 31, 127, 255}
}

// MMVSize returns the number of uint64 entries
// required to store a vector modulo p in internal
// representation (excluding the guard). C function
// mm_aux_mmv_size.
//
// MMVSize panics if p is not a supported modulus.
func MMVSize(p int) int {
	checkP(p)
	return 247488 >> (mmvConst(p) & 7)
}

// vectToCocode returns the cocode element of vector
// v. C mat24_vect_to_cocode. It is the flat-cgt twin
// of mmindex's unexported helper, kept here so the
// generated mm_op_p_gen.go (genScalprodDISign) needs
// no edit and regeneration stays byte-identical.
func vectToCocode(v uint32) uint32 { return mat24.Vintern(v) & 0xfff }
