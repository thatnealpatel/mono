// Package cgt: MMVector, the representation
// vectors of the Monster group modulo p, ported
// from the mmgroup C library (mm_aux.c,
// mm_index.c, mm_tables.c, mm_tables_xi.c, and the
// per-modulus mm*_op_*.c files).
package cgt

//go:generate go run -C _gen . -out ../mm_op_xi_gen.go

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

// Internal-representation tag offsets (entries from
// the start of the vector). C enum MM_AUX_OFS.
const (
	mmAuxOfsA = 0
	mmAuxOfsB = 768
	mmAuxOfsC = 1536
	mmAuxOfsT = 2304
	mmAuxOfsX = 50880
	mmAuxOfsZ = 116416
	mmAuxOfsY = 181952
	mmAuxLenV = 247488
)

// External-representation tag offsets. C enum
// MM_AUX_XOFS.
const (
	mmAuxXofsD = 0
	mmAuxXofsA = 24
	mmAuxXofsB = 300
	mmAuxXofsC = 576
	mmAuxXofsT = 852
	mmAuxXofsX = 49428
	mmAuxXofsZ = 98580
	mmAuxXofsY = 147732
	mmAuxXlenV = 196884
)

// Sparse-representation tag codes (bits 27..25
// pre-shifted). C enum MM_SPACE_TAG.
const (
	mmSpaceTagA = 0x2000000
	mmSpaceTagB = 0x4000000
	mmSpaceTagC = 0x6000000
	mmSpaceTagT = 0x8000000
	mmSpaceTagX = 0xA000000
	mmSpaceTagZ = 0xC000000
	mmSpaceTagY = 0xE000000
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

// mmAuxV24Ints returns the number of uint64 entries
// needed to hold 24 entries of a vector modulo p. C
// function mm_aux_v24_ints. The caller must pass a
// supported modulus p.
func mmAuxV24Ints(p int) int {
	return 32 >> (mmvConst(p) & 7)
}

/**********************************************************************
 * Index conversion (mm_index.c)
 **********************************************************************/

// mmAuxTblABC expands tag A/B/C entries between the
// external and internal layouts. C table
// MM_AUX_TBL_ABC.
var mmAuxTblABC = [...]uint16{
	0x0018, 0x0038, 0x0058, 0x0078, 0x0098, 0x00b8, 0x00d8, 0x00f8,
	0x0118, 0x0138, 0x0158, 0x0178, 0x0198, 0x01b8, 0x01d8, 0x01f8,
	0x0218, 0x0238, 0x0258, 0x0278, 0x0298, 0x02b8, 0x02d8, 0x02f8,
	0x0820, 0x103f, 0x083f, 0x185d, 0x105d, 0x085d, 0x207a, 0x187a,
	0x107a, 0x087a, 0x2896, 0x2096, 0x1896, 0x1096, 0x0896, 0x30b1,
	0x28b1, 0x20b1, 0x18b1, 0x10b1, 0x08b1, 0x38cb, 0x30cb, 0x28cb,
	0x20cb, 0x18cb, 0x10cb, 0x08cb, 0x40e4, 0x38e4, 0x30e4, 0x28e4,
	0x20e4, 0x18e4, 0x10e4, 0x08e4, 0x48fc, 0x40fc, 0x38fc, 0x30fc,
	0x28fc, 0x20fc, 0x18fc, 0x10fc, 0x08fc, 0x5113, 0x4913, 0x4113,
	0x3913, 0x3113, 0x2913, 0x2113, 0x1913, 0x1113, 0x0913, 0x5929,
	0x5129, 0x4929, 0x4129, 0x3929, 0x3129, 0x2929, 0x2129, 0x1929,
	0x1129, 0x0929, 0x613e, 0x593e, 0x513e, 0x493e, 0x413e, 0x393e,
	0x313e, 0x293e, 0x213e, 0x193e, 0x113e, 0x093e, 0x6952, 0x6152,
	0x5952, 0x5152, 0x4952, 0x4152, 0x3952, 0x3152, 0x2952, 0x2152,
	0x1952, 0x1152, 0x0952, 0x7165, 0x6965, 0x6165, 0x5965, 0x5165,
	0x4965, 0x4165, 0x3965, 0x3165, 0x2965, 0x2165, 0x1965, 0x1165,
	0x0965, 0x7977, 0x7177, 0x6977, 0x6177, 0x5977, 0x5177, 0x4977,
	0x4177, 0x3977, 0x3177, 0x2977, 0x2177, 0x1977, 0x1177, 0x0977,
	0x8188, 0x7988, 0x7188, 0x6988, 0x6188, 0x5988, 0x5188, 0x4988,
	0x4188, 0x3988, 0x3188, 0x2988, 0x2188, 0x1988, 0x1188, 0x0988,
	0x8998, 0x8198, 0x7998, 0x7198, 0x6998, 0x6198, 0x5998, 0x5198,
	0x4998, 0x4198, 0x3998, 0x3198, 0x2998, 0x2198, 0x1998, 0x1198,
	0x0998, 0x91a7, 0x89a7, 0x81a7, 0x79a7, 0x71a7, 0x69a7, 0x61a7,
	0x59a7, 0x51a7, 0x49a7, 0x41a7, 0x39a7, 0x31a7, 0x29a7, 0x21a7,
	0x19a7, 0x11a7, 0x09a7, 0x99b5, 0x91b5, 0x89b5, 0x81b5, 0x79b5,
	0x71b5, 0x69b5, 0x61b5, 0x59b5, 0x51b5, 0x49b5, 0x41b5, 0x39b5,
	0x31b5, 0x29b5, 0x21b5, 0x19b5, 0x11b5, 0x09b5, 0xa1c2, 0x99c2,
	0x91c2, 0x89c2, 0x81c2, 0x79c2, 0x71c2, 0x69c2, 0x61c2, 0x59c2,
	0x51c2, 0x49c2, 0x41c2, 0x39c2, 0x31c2, 0x29c2, 0x21c2, 0x19c2,
	0x11c2, 0x09c2, 0xa9ce, 0xa1ce, 0x99ce, 0x91ce, 0x89ce, 0x81ce,
	0x79ce, 0x71ce, 0x69ce, 0x61ce, 0x59ce, 0x51ce, 0x49ce, 0x41ce,
	0x39ce, 0x31ce, 0x29ce, 0x21ce, 0x19ce, 0x11ce, 0x09ce, 0xb1d9,
	0xa9d9, 0xa1d9, 0x99d9, 0x91d9, 0x89d9, 0x81d9, 0x79d9, 0x71d9,
	0x69d9, 0x61d9, 0x59d9, 0x51d9, 0x49d9, 0x41d9, 0x39d9, 0x31d9,
	0x29d9, 0x21d9, 0x19d9, 0x11d9, 0x09d9, 0xb9e3, 0xb1e3, 0xa9e3,
	0xa1e3, 0x99e3, 0x91e3, 0x89e3, 0x81e3, 0x79e3, 0x71e3, 0x69e3,
	0x61e3, 0x59e3, 0x51e3, 0x49e3, 0x41e3, 0x39e3, 0x31e3, 0x29e3,
	0x21e3, 0x19e3, 0x11e3, 0x09e3, 0x0a0c, 0x122b, 0x0a2b, 0x1a49,
	0x1249, 0x0a49, 0x2266, 0x1a66, 0x1266, 0x0a66, 0x2a82, 0x2282,
	0x1a82, 0x1282, 0x0a82, 0x329d, 0x2a9d, 0x229d, 0x1a9d, 0x129d,
	0x0a9d, 0x3ab7, 0x32b7, 0x2ab7, 0x22b7, 0x1ab7, 0x12b7, 0x0ab7,
	0x42d0, 0x3ad0, 0x32d0, 0x2ad0, 0x22d0, 0x1ad0, 0x12d0, 0x0ad0,
	0x4ae8, 0x42e8, 0x3ae8, 0x32e8, 0x2ae8, 0x22e8, 0x1ae8, 0x12e8,
	0x0ae8, 0x52ff, 0x4aff, 0x42ff, 0x3aff, 0x32ff, 0x2aff, 0x22ff,
	0x1aff, 0x12ff, 0x0aff, 0x5b15, 0x5315, 0x4b15, 0x4315, 0x3b15,
	0x3315, 0x2b15, 0x2315, 0x1b15, 0x1315, 0x0b15, 0x632a, 0x5b2a,
	0x532a, 0x4b2a, 0x432a, 0x3b2a, 0x332a, 0x2b2a, 0x232a, 0x1b2a,
	0x132a, 0x0b2a, 0x6b3e, 0x633e, 0x5b3e, 0x533e, 0x4b3e, 0x433e,
	0x3b3e, 0x333e, 0x2b3e, 0x233e, 0x1b3e, 0x133e, 0x0b3e, 0x7351,
	0x6b51, 0x6351, 0x5b51, 0x5351, 0x4b51, 0x4351, 0x3b51, 0x3351,
	0x2b51, 0x2351, 0x1b51, 0x1351, 0x0b51, 0x7b63, 0x7363, 0x6b63,
	0x6363, 0x5b63, 0x5363, 0x4b63, 0x4363, 0x3b63, 0x3363, 0x2b63,
	0x2363, 0x1b63, 0x1363, 0x0b63, 0x8374, 0x7b74, 0x7374, 0x6b74,
	0x6374, 0x5b74, 0x5374, 0x4b74, 0x4374, 0x3b74, 0x3374, 0x2b74,
	0x2374, 0x1b74, 0x1374, 0x0b74, 0x8b84, 0x8384, 0x7b84, 0x7384,
	0x6b84, 0x6384, 0x5b84, 0x5384, 0x4b84, 0x4384, 0x3b84, 0x3384,
	0x2b84, 0x2384, 0x1b84, 0x1384, 0x0b84, 0x9393, 0x8b93, 0x8393,
	0x7b93, 0x7393, 0x6b93, 0x6393, 0x5b93, 0x5393, 0x4b93, 0x4393,
	0x3b93, 0x3393, 0x2b93, 0x2393, 0x1b93, 0x1393, 0x0b93, 0x9ba1,
	0x93a1, 0x8ba1, 0x83a1, 0x7ba1, 0x73a1, 0x6ba1, 0x63a1, 0x5ba1,
	0x53a1, 0x4ba1, 0x43a1, 0x3ba1, 0x33a1, 0x2ba1, 0x23a1, 0x1ba1,
	0x13a1, 0x0ba1, 0xa3ae, 0x9bae, 0x93ae, 0x8bae, 0x83ae, 0x7bae,
	0x73ae, 0x6bae, 0x63ae, 0x5bae, 0x53ae, 0x4bae, 0x43ae, 0x3bae,
	0x33ae, 0x2bae, 0x23ae, 0x1bae, 0x13ae, 0x0bae, 0xabba, 0xa3ba,
	0x9bba, 0x93ba, 0x8bba, 0x83ba, 0x7bba, 0x73ba, 0x6bba, 0x63ba,
	0x5bba, 0x53ba, 0x4bba, 0x43ba, 0x3bba, 0x33ba, 0x2bba, 0x23ba,
	0x1bba, 0x13ba, 0x0bba, 0xb3c5, 0xabc5, 0xa3c5, 0x9bc5, 0x93c5,
	0x8bc5, 0x83c5, 0x7bc5, 0x73c5, 0x6bc5, 0x63c5, 0x5bc5, 0x53c5,
	0x4bc5, 0x43c5, 0x3bc5, 0x33c5, 0x2bc5, 0x23c5, 0x1bc5, 0x13c5,
	0x0bc5, 0xbbcf, 0xb3cf, 0xabcf, 0xa3cf, 0x9bcf, 0x93cf, 0x8bcf,
	0x83cf, 0x7bcf, 0x73cf, 0x6bcf, 0x63cf, 0x5bcf, 0x53cf, 0x4bcf,
	0x43cf, 0x3bcf, 0x33cf, 0x2bcf, 0x23cf, 0x1bcf, 0x13cf, 0x0bcf,
	0x0bf8, 0x1417, 0x0c17, 0x1c35, 0x1435, 0x0c35, 0x2452, 0x1c52,
	0x1452, 0x0c52, 0x2c6e, 0x246e, 0x1c6e, 0x146e, 0x0c6e, 0x3489,
	0x2c89, 0x2489, 0x1c89, 0x1489, 0x0c89, 0x3ca3, 0x34a3, 0x2ca3,
	0x24a3, 0x1ca3, 0x14a3, 0x0ca3, 0x44bc, 0x3cbc, 0x34bc, 0x2cbc,
	0x24bc, 0x1cbc, 0x14bc, 0x0cbc, 0x4cd4, 0x44d4, 0x3cd4, 0x34d4,
	0x2cd4, 0x24d4, 0x1cd4, 0x14d4, 0x0cd4, 0x54eb, 0x4ceb, 0x44eb,
	0x3ceb, 0x34eb, 0x2ceb, 0x24eb, 0x1ceb, 0x14eb, 0x0ceb, 0x5d01,
	0x5501, 0x4d01, 0x4501, 0x3d01, 0x3501, 0x2d01, 0x2501, 0x1d01,
	0x1501, 0x0d01, 0x6516, 0x5d16, 0x5516, 0x4d16, 0x4516, 0x3d16,
	0x3516, 0x2d16, 0x2516, 0x1d16, 0x1516, 0x0d16, 0x6d2a, 0x652a,
	0x5d2a, 0x552a, 0x4d2a, 0x452a, 0x3d2a, 0x352a, 0x2d2a, 0x252a,
	0x1d2a, 0x152a, 0x0d2a, 0x753d, 0x6d3d, 0x653d, 0x5d3d, 0x553d,
	0x4d3d, 0x453d, 0x3d3d, 0x353d, 0x2d3d, 0x253d, 0x1d3d, 0x153d,
	0x0d3d, 0x7d4f, 0x754f, 0x6d4f, 0x654f, 0x5d4f, 0x554f, 0x4d4f,
	0x454f, 0x3d4f, 0x354f, 0x2d4f, 0x254f, 0x1d4f, 0x154f, 0x0d4f,
	0x8560, 0x7d60, 0x7560, 0x6d60, 0x6560, 0x5d60, 0x5560, 0x4d60,
	0x4560, 0x3d60, 0x3560, 0x2d60, 0x2560, 0x1d60, 0x1560, 0x0d60,
	0x8d70, 0x8570, 0x7d70, 0x7570, 0x6d70, 0x6570, 0x5d70, 0x5570,
	0x4d70, 0x4570, 0x3d70, 0x3570, 0x2d70, 0x2570, 0x1d70, 0x1570,
	0x0d70, 0x957f, 0x8d7f, 0x857f, 0x7d7f, 0x757f, 0x6d7f, 0x657f,
	0x5d7f, 0x557f, 0x4d7f, 0x457f, 0x3d7f, 0x357f, 0x2d7f, 0x257f,
	0x1d7f, 0x157f, 0x0d7f, 0x9d8d, 0x958d, 0x8d8d, 0x858d, 0x7d8d,
	0x758d, 0x6d8d, 0x658d, 0x5d8d, 0x558d, 0x4d8d, 0x458d, 0x3d8d,
	0x358d, 0x2d8d, 0x258d, 0x1d8d, 0x158d, 0x0d8d, 0xa59a, 0x9d9a,
	0x959a, 0x8d9a, 0x859a, 0x7d9a, 0x759a, 0x6d9a, 0x659a, 0x5d9a,
	0x559a, 0x4d9a, 0x459a, 0x3d9a, 0x359a, 0x2d9a, 0x259a, 0x1d9a,
	0x159a, 0x0d9a, 0xada6, 0xa5a6, 0x9da6, 0x95a6, 0x8da6, 0x85a6,
	0x7da6, 0x75a6, 0x6da6, 0x65a6, 0x5da6, 0x55a6, 0x4da6, 0x45a6,
	0x3da6, 0x35a6, 0x2da6, 0x25a6, 0x1da6, 0x15a6, 0x0da6, 0xb5b1,
	0xadb1, 0xa5b1, 0x9db1, 0x95b1, 0x8db1, 0x85b1, 0x7db1, 0x75b1,
	0x6db1, 0x65b1, 0x5db1, 0x55b1, 0x4db1, 0x45b1, 0x3db1, 0x35b1,
	0x2db1, 0x25b1, 0x1db1, 0x15b1, 0x0db1, 0xbdbb, 0xb5bb, 0xadbb,
	0xa5bb, 0x9dbb, 0x95bb, 0x8dbb, 0x85bb, 0x7dbb, 0x75bb, 0x6dbb,
	0x65bb, 0x5dbb, 0x55bb, 0x4dbb, 0x45bb, 0x3dbb, 0x35bb, 0x2dbb,
	0x25bb, 0x1dbb, 0x15bb, 0x0dbb,
}

// IndexExternToSparse converts external index i to a
// sparse index. C mm_aux_index_extern_to_sparse.
// Returns 0 if i >= 196884.
func IndexExternToSparse(i int) uint32 {
	u := uint32(i)
	if u < mmAuxXofsX {
		if u < mmAuxXofsT {
			u = (uint32(mmAuxTblABC[u]) & 0x7ff) + u - 24
			u += (0x2A54000 >> ((u >> 8) << 1)) & 0x300
			return 0x2000000 + ((u & 0xc00) << 15) +
				((u & 0x3e0) << 9) + ((u & 0x1f) << 8)
		}
		u += 0x80000 - mmAuxXofsT
		return u << 8
	} else if u < mmAuxXlenV {
		u -= mmAuxXofsX
		u += (((u >> 3) * 0xaaab) >> 17) << 3
		u += u & 0x3ffe0
		u += 0xA0000
		return u << 8
	}
	return 0
}

// IndexSparseToExtern converts sparse index sp to an
// external index. C mm_aux_index_sparse_to_extern.
// Returns -1 for an illegal index.
func IndexSparseToExtern(sp uint32) int {
	tag := sp >> 25
	j := (sp >> 8) & 0x3f
	i := (sp >> 14) & 0x7ff
	switch tag {
	case 2, 3: // B, C
		if i == j {
			return -1
		}
		fallthrough
	case 1: // A
		if i >= 24 || j >= 24 {
			return -1
		}
		if i == j {
			return int(i)
		}
		return int((tag-1)*276 + mmAuxXofsA + ((i*i - i) >> 1) + j)
	case 4: // T
		if i >= 759 {
			return -1
		}
		return int(uint32(mmAuxXofsT) + (i << 6) + j)
	case 5, 6, 7: // X, Z, Y
		if j >= 24 {
			return -1
		}
		return int((24*((tag<<11)+i) - 0x3c000) + mmAuxXofsX + j)
	default:
		return -1
	}
}

// IndexExternToIntern converts external index i to an
// internal index. C mm_aux_index_extern_to_intern.
// Returns -1 if i >= 196884.
func IndexExternToIntern(i int) int {
	u := uint32(i)
	if u < mmAuxXofsX {
		if u < mmAuxXofsT {
			return int((uint32(mmAuxTblABC[u]) & 0x7ff) + u - 24)
		}
		return int(u + mmAuxOfsT - mmAuxXofsT)
	} else if u < mmAuxXlenV {
		u -= mmAuxXofsX
		u += (((u >> 3) * 0xaaab) >> 17) << 3
		return int(u + mmAuxOfsX)
	}
	return -1
}

// IndexSparseToIntern converts sparse index sp to an
// internal index. C mm_aux_index_sparse_to_intern.
// Returns -1 for an illegal index.
func IndexSparseToIntern(sp uint32) int {
	tag := sp >> 25
	j := (sp >> 8) & 0x3f
	i := (sp >> 14) & 0x7ff
	switch tag {
	case 2, 3: // B, C
		if i == j {
			return -1
		}
		fallthrough
	case 1: // A
		if i >= 24 || j >= 24 {
			return -1
		}
		return int(((tag-1)*24+i)*32 + j)
	case 4: // T
		if i >= 759 {
			return -1
		}
		return int(mmAuxOfsT + (i << 6) + j)
	case 5, 6, 7: // X, Z, Y
		if j >= 24 {
			return -1
		}
		return int(mmAuxOfsX + 32*(((tag-5)<<11)+i) + j)
	default:
		return -1
	}
}

// IndexInternToSparse converts internal index i to a
// sparse index. C mm_aux_index_intern_to_sparse.
// Returns 0 for a bad index.
func IndexInternToSparse(i int) uint32 {
	u := uint32(i)
	if u < mmAuxOfsX {
		if u < mmAuxOfsT {
			t := uint32(0x2A540>>((u>>8)<<1)) & 3
			i0 := u - t*0x300
			i1 := i0 & 31
			i0 >>= 5
			if i0 < i1 {
				i0, i1 = i1, i0
			}
			if i0 >= 24 {
				return 0
			}
			if t != 0 && i0 == i1 {
				return 0
			}
			return ((t + 1) << 25) + (i0 << 14) + (i1 << 8)
		}
		u += 0x80000 - mmAuxOfsT
		return u << 8
	} else if u < mmAuxLenV {
		u -= mmAuxOfsX
		i0 := u >> 5
		i1 := u & 31
		if i1 >= 24 {
			return 0
		}
		return mmSpaceTagX + (i0 << 14) + (i1 << 8)
	}
	return 0
}

// IndexCheckIntern returns the alternate internal
// location of the entry at internal index i (e.g.
// for tags A/B/C off the diagonal), 0 if there is
// exactly one location, or -1 if i is illegal. C
// mm_aux_index_check_intern.
func IndexCheckIntern(i int) int {
	u := uint32(i)
	i2 := u & 31
	if u < mmAuxOfsT {
		if i2 >= 24 {
			return -1
		}
		t := (((u & 0xf00) * 0x55556) >> 28) * 0x300
		i1 := (u - t) >> 5
		if i1 == i2 {
			if t > 0 {
				return -1
			}
			return 0
		}
		return int(t + (i2 << 5) + i1)
	}
	if u < mmAuxOfsX || (u < mmAuxLenV && i2 < 24) {
		return 0
	}
	return -1
}

// IndexSparseToLeech2 converts sparse index sp to a
// short Leech-lattice-mod-2 vector. C
// mm_aux_index_sparse_to_leech2. Returns 0 on
// failure.
func IndexSparseToLeech2(sp uint32) uint32 {
	tag := sp >> 25
	j := (sp >> 8) & 0x3f
	i := (sp >> 14) & 0x7ff
	var res uint32
	switch tag {
	case 3: // C
		res = 0x800000
		fallthrough
	case 2: // B
		if i == j || i >= 24 || j >= 24 {
			return 0
		}
		return res + vectToCocode((1<<i)^(1<<j))
	case 4: // T
		if i >= 759 {
			return 0
		}
		cocode := suboctadToCocodeInline(j, i)
		gcode := uint32(mat24OctDecTable[i]) & 0xfff
		gcode ^= suboctadWeight(j) << 11
		cocode ^= uint32(mat24ThetaTable[gcode&0x7ff]) & 0xfff
		return (gcode << 12) + cocode
	case 5: // X
		if j >= 24 {
			return 0
		}
		cocode := vectToCocode(1 << j)
		theta := uint32(mat24ThetaTable[i&0x7ff])
		w := ((theta >> 12) & 1) ^ (i & cocode)
		w = parity12(w)
		gcode := i ^ (w << 11)
		cocode ^= theta & 0xfff
		return (gcode << 12) + cocode
	default:
		return 0
	}
}

// IndexLeech2ToSparse converts a short
// Leech-lattice-mod-2 vector v2 to a sparse index. C
// mm_aux_index_leech2_to_sparse. Returns 0 if v2 is
// not short.
func IndexLeech2ToSparse(v2 uint32) uint32 {
	var theta, syn, scalar, gc, res uint32

	if v2&0x800 != 0 { // odd cocode words
		theta = uint32(mat24ThetaTable[(v2>>12)&0x7ff])
		syn = uint32(mat24SyndromeTable[(theta^v2)&0x7ff])
		if (syn & 0x3ff) < (24 << 5) {
			return 0
		}
		scalar = (v2 >> 12) & v2
		scalar = parity12(scalar)
		if scalar != 0 {
			return 0
		}
		return 0xA000000 + ((v2 & 0x7ff000) << 2) + ((syn & 0x1f) << 8)
	}
	if v2&0x7ff000 == 0 { // Golay code word 0
		syn = uint32(mat24SyndromeTable[v2&0x7ff])
		if syn&0x8000 == 0 {
			return 0
		}
		syn = uint32(mat24SyndromeTable[(v2^mat24RecipBasis[23])&0x7ff])
		syn &= 0x3ff
		syn -= ((syn + 0x100) & 0x400) >> 5
		return ((syn >> 5) << 14) + ((syn & 0x1f) << 8) + 0x4000000 +
			((0x800000 & v2) << 2)
	}
	// octads (and suboctads)
	gc = (v2 >> 12) & 0xfff
	theta = uint32(mat24ThetaTable[gc&0x7ff]) & 0x7ff
	res = cocodeToSuboctadInline((v2^theta)&0xfff, gc, 1)
	if res == 0xffffffff {
		return 0
	}
	return 0x8000000 + (res << 8)
}

// cocodeToSuboctadInline mirrors the C inline
// mat24_inline_cocode_to_suboctad; it equals the
// internal cocodeToSuboctad in mat24.go.
func cocodeToSuboctadInline(c, v, strict uint32) uint32 {
	return cocodeToSuboctad(c, v, strict)
}

// suboctadToCocodeInline mirrors the C inline
// mat24_inline_suboctad_to_cocode. It returns
// 0xffffffff if octad >= 759 (no panic), matching
// the C inline rather than SuboctadToCocode.
func suboctadToCocodeInline(sub, octad uint32) uint32 {
	if octad >= 759 {
		return 0xffffffff
	}
	pOctad := mat24OctadElementTable[octad<<3:]
	pSub := mat24OctadIndexTable[(sub&0x3f)<<2:]
	c := mat24RecipBasis[pOctad[pSub[0]]] ^
		mat24RecipBasis[pOctad[pSub[1]]] ^
		mat24RecipBasis[pOctad[pSub[2]]] ^
		mat24RecipBasis[pOctad[pSub[3]]]
	return c & 0xfff
}

// vectToCocode returns the cocode element of vector
// v. C mat24_vect_to_cocode.
func vectToCocode(v uint32) uint32 { return vintern(v) & 0xfff }

// mmAuxIndexLeech2ToInternFast converts a short
// Leech-lattice-mod-2 vector v2 to an internal
// index, with fewer checks. C
// mm_aux_index_leech2_to_intern_fast.
func mmAuxIndexLeech2ToInternFast(v2 uint32) uint32 {
	gc := (v2 >> 12) & 0x7ff
	if v2&0x800 != 0 {
		theta := uint32(mat24ThetaTable[gc])
		syn := uint32(mat24SyndromeTable[(theta^v2)&0x7ff])
		return mmAuxOfsX + (gc << 5) + (syn & 0x1f)
	}
	if gc == 0 {
		syn := uint32(mat24SyndromeTable[(v2^mat24RecipBasis[23])&0x7ff])
		syn &= 0x3ff
		syn -= ((syn + 0x100) & 0x400) >> 5
		var res uint32 = mmAuxOfsB
		if v2&0x800000 != 0 {
			res = mmAuxOfsC
		}
		return res + (syn & 0x3ff)
	}
	oct := uint32(mat24OctEncTable[gc]) >> 1
	if oct >= 759 {
		return 0
	}
	pOct := mat24OctadElementTable[oct<<3:]
	theta := uint32(mat24ThetaTable[gc])
	j := uint32(pOct[7])
	c := uint32(mat24SyndromeTable[(theta^v2^mat24RecipBasis[j])&0x7ff])
	syn := synFromTable(c)
	var sub uint32
	if (syn>>j)&1 != 0 {
		sub = 0
	} else {
		sub = 0x3f
	}
	sub ^= ((syn >> pOct[1]) & 1) << 0
	sub ^= ((syn >> pOct[2]) & 1) << 1
	sub ^= ((syn >> pOct[3]) & 1) << 2
	sub ^= ((syn >> pOct[4]) & 1) << 3
	sub ^= ((syn >> pOct[5]) & 1) << 4
	sub ^= ((syn >> pOct[6]) & 1) << 5
	return mmAuxOfsT + (oct << 6) + sub
}

// mmAuxIndexInternToLeech2 converts an internal
// index i to a short Leech-lattice-mod-2 vector. C
// mm_aux_index_intern_to_leech2. Returns 0 on
// failure.
func mmAuxIndexInternToLeech2(i uint32) uint32 {
	if i < mmAuxOfsT { // tags B, C
		t := uint32(0x2A540>>((i>>8)<<1)) & 3
		i0 := i - t*0x300
		i1 := i0 & 31
		i0 >>= 5
		if t == 0 || i0 == i1 || i1 > 24 {
			return 0
		}
		v := mat24RecipBasis[i0] ^ mat24RecipBasis[i1]
		return (v & 0xfff) + ((t - 1) << 23)
	} else if i < mmAuxOfsX { // tag T
		i -= mmAuxOfsT
		i0 := i >> 6
		i1 := i & 0x3f
		cocode := suboctadToCocodeInline(i1, i0)
		gcode := uint32(mat24OctDecTable[i0]) & 0xfff
		gcode ^= suboctadWeight(i1) << 11
		cocode ^= uint32(mat24ThetaTable[gcode&0x7ff]) & 0xfff
		return (gcode << 12) + cocode
	} else if i < mmAuxOfsZ {
		i -= mmAuxOfsX
		i0 := i >> 5
		i1 := i & 0x1f
		if i1 > 24 {
			return 0
		}
		cocode := vectToCocode(1 << i1)
		theta := uint32(mat24ThetaTable[i0&0x7ff])
		w := ((theta >> 12) & 1) ^ (i0 & cocode)
		w = parity12(w)
		gcode := i0 ^ (w << 11)
		cocode ^= theta & 0xfff
		return (gcode << 12) + cocode
	}
	return 0
}
