package reduce

import (
	"fmt"

	"patel.codes/cgt/generator"
	"patel.codes/cgt/leech"
	"patel.codes/cgt/mat24"
	"patel.codes/cgt/mmindex"
	"patel.codes/cgt/n0"
	"patel.codes/cgt/xsp2co1"
)

// Package reduce ports the integer encoding of a
// reduced monster word from the C modules
// mm_compress.c and mm_shorten.c (the GtWord
// reduction engine). It depends only on the lower
// algebra packages (xsp2co1, n0, leech, mmindex and
// below) and never on the flat cgt package.
//
// An element of the monster is encoded as a 255-bit
// integer held as four uint64 digits (lowest digit
// first). The public Go surface in monster.go
// exposes this as a single uint64; for the elements
// it is applied to the value fits in 64 bits and the
// upper three digits are zero. CompressAsInt
// returns the lowest digit and ExpandInt reads it
// back from the lowest digit.

// invertWord inverts the word w in place.
func invertWord(w []uint32) {
	for i := range w {
		w[i] ^= 0x80000000
	}
	for i, j := 0, len(w)-1; i < j; i, j = i+1, j-1 {
		w[i], w[j] = w[j], w[i]
	}
}

//////////////////////////////////////////////////
// Bit-field helpers on a 256-bit little-endian
// integer held as [4]uint64 (mm_compress.c).
//////////////////////////////////////////////////

// insertInt256 stores the low nbits of value into pN
// starting at bit position pos. It mirrors C
// insert_int256.
func insertInt256(pN *[4]uint64, value uint64, pos, nbits uint32) {
	wpos := pos >> 6
	var mask uint64
	if wpos >= 4 {
		return
	}
	pos &= 0x3f
	if nbits < 64 {
		mask = (uint64(1) << nbits) - 1
		value &= mask
	}
	pN[wpos] &^= mask << pos
	pN[wpos] |= value << pos
	if wpos >= 3 || pos == 0 {
		return
	}
	pN[wpos+1] &^= mask >> (64 - pos)
	pN[wpos+1] |= value >> (64 - pos)
}

// extractInt256 returns the nbits-wide field of pN
// at bit position pos. It mirrors C extract_int256.
func extractInt256(pN *[4]uint64, nbits, pos uint32) uint32 {
	wpos := pos >> 6
	if wpos >= 4 {
		return 0
	}
	pos &= 0x3f
	result := pN[wpos] >> pos
	if wpos < 3 && pos != 0 {
		result += pN[wpos+1] << (64 - pos)
	}
	if nbits < 64 {
		result &= (uint64(1) << nbits) - 1
	}
	return uint32(result)
}

//////////////////////////////////////////////////
// Compress a type-4 Leech vector from 24 to 23 bits
// (mm_compress.c).
//////////////////////////////////////////////////

const (
	cocode0   = 0x800 // internal rep of cocode word [0]
	cocode01  = 0x600 // internal rep of cocode word [0,1]
	cocodeStd = 0x200 // internal rep of cocode word [0,2]
)

// compress24To23 compresses an even-type Leech mod 2
// vector from 24 to 23 bits. At least one of bits
// 11..22 of i must be set. Mirrors C compress_24_23.
func compress24To23(i uint32) uint32 {
	i &= 0xffffff
	// Exchange bits 11 and 23 in i.
	j := (i ^ (i >> 12)) & 0x800
	i ^= j + (j << 12)
	// Delete the lowest set bit of i>>12 from i and
	// shift higher bits down by one.
	b := i >> 12
	b = b & (0 - b)
	i = (i & (b - 1)) | ((i >> 1) & (0 - b))
	return i
}

// expand23To24 reverses compress24To23. It returns
// (value, true), or (0, false) on error. Mirrors C
// expand_23_24.
func expand23To24(i uint32) (uint32, bool) {
	i &= 0x7fffff
	if i&0x7ff800 == 0 {
		return 0, false
	}
	// Insert a zero bit at the position of the lowest
	// set bit of i>>11, shifting higher bits up by one.
	b := i >> 11
	b = b & (0 - b)
	i = (i & (b - 1)) | ((i & (0 - b)) << 1)
	// Adjust the inserted bit so parity(i & (i>>12))
	// is even.
	j := i & (i >> 12)
	j ^= j >> 6
	j ^= j >> 3
	j = (0x96 >> (j & 7)) & 1
	i ^= (0 - j) & b
	// Exchange bits 11 and 23.
	j = (i ^ (i >> 12)) & 0x800
	i ^= j ^ (j << 12)
	return i, true
}

// mmCompressType4 compresses a type-4 vector i in
// the Leech lattice mod 2 to a 23-bit integer. It
// returns -1 if i is not of type 4. Mirrors C
// mm_compress_type4.
func mmCompressType4(i uint32) int {
	i &= 0xffffff
	if generator.Leech2Type(i) != 4 {
		return -1
	}
	if i&0x7ff800 == 0 {
		i0 := i & 0x7ff
		j := (i0 << 12) | (uint32(mat24.ThetaTable(i0)) & 0x7ff) | cocode0
		if i&0x800000 != 0 {
			j ^= cocode01
		}
		// Make the type of Leech vector j even.
		b := j & (j >> 12)
		b = mat24.Parity12(b)
		j ^= b << 23
		i = j
	}
	return int(compress24To23(i))
}

// mmCompressExpandType4 reverses mmCompressType4. It
// returns a negative value on error. Mirrors C
// mm_compress_expand_type4.
func mmCompressExpandType4(i uint32) int {
	i, ok := expand23To24(i)
	if !ok || i&0xff000000 != 0 {
		return -11
	}
	switch generator.Leech2Type(i) {
	case 2:
		// Result vector is of subtype 00, 20, 40, or 48.
		j := (i >> 12) & 0x7ff
		// Reject if j has weight 2.
		if uint32(mat24.SyndromeTable(j))>>15 != 0 {
			return -12
		}
		coc := (i ^ uint32(mat24.ThetaTable(j)) ^ cocode0) & 0xfff
		if coc != 0 {
			if coc != cocode01 {
				return -13
			}
			j ^= 0x800000
		} else if j == 0 {
			return -14
		}
		return int(j)
	case 4:
		return int(i)
	default:
		return -15
	}
}

//////////////////////////////////////////////////
// The compressed-element structure (mm_compress.c).
//
// nx encodes y_f x_d x_delta pi; w holds the type-2,
// type-4 and t entries; cur is the index of the last
// entry written.
//////////////////////////////////////////////////

const mmCompressNEntries = 19

type mmCompress struct {
	nx  uint64
	w   [mmCompressNEntries]uint32
	cur uint32
}

// mmCompressPCInit sets pc to the empty word.
// Mirrors C mm_compress_pc_init.
func mmCompressPCInit(pc *mmCompress) {
	pc.nx = 0
	pc.cur = 0
	for i := range pc.w {
		pc.w[i] = 0
	}
}

// nReduceElementY reduces g in N_0 modulo the kernel
// K_0, always folding y into the x part. It returns 0
// iff g is neutral. Mirrors C
// mm_group_n_reduce_element_y.
func nReduceElementY(g []uint32) uint32 {
	g[0] %= 3
	g[1] &= 0x1fff
	g[2] &= 0x1fff
	g[3] &= 0xfff
	g[2] ^= uint32(n0.KerTableYx[g[1]>>11])
	g[1] &= 0x7ff
	return g[0] | g[1] | g[2] | g[3] | g[4]
}

// mmCompressPCAddNx adds the N_0 prefix of word m to
// pc, returning the number of atoms read or a
// negative value on failure. Only tags d, p, x, y are
// allowed. It may be applied only to an empty
// structure. Mirrors C mm_compress_pc_add_nx.
func mmCompressPCAddNx(pc *mmCompress, m []uint32) int {
	var g n0.N0Elem
	i := uint32(0)
	for ; i < uint32(len(m)); i++ {
		if (m[i]>>28)&7 > 4 {
			break
		}
	}
	if n0.MulWordScan(&g, m[:i]) != i {
		return -0x1001
	}
	if nReduceElementY(g[:]) == 0 {
		return int(i)
	}
	if pc.nx|uint64(pc.w[0]&0x2000000) != 0 {
		return -0x1002
	}
	if pc.w[pc.cur] != 0 {
		return -0x1003
	}
	pc.nx = uint64(g[4]) + (uint64(g[1]) << 28) +
		(uint64(g[2]) << 39) + (uint64(g[3]) << 52)
	return int(i)
}

// mmCompressPCAddType2 adds a deprecated type-2 entry
// to pc. Mirrors C mm_compress_pc_add_type2.
func mmCompressPCAddType2(pc *mmCompress, c uint32) int {
	c &= 0xffffff
	if c&^cocodeStd == 0 {
		return 0
	}
	if pc.nx|uint64(pc.w[pc.cur]&0x6000000) != 0 {
		return -2001
	}
	if pc.cur|pc.w[pc.cur] != 0 {
		return -2003
	}
	pc.w[pc.cur] = c | 0x2000000
	return 0
}

// mmCompressPCAddType4 adds an entry with tag 'c'
// (a type-4 vector) to pc. Mirrors C
// mm_compress_pc_add_type4.
func mmCompressPCAddType4(pc *mmCompress, c uint32) int {
	c &= 0xffffff
	if c&0x7fffff == 0 {
		return 0
	}
	if pc.w[pc.cur]&0x6000000 != 0 {
		return -3001
	}
	if pc.w[pc.cur] != 0 {
		pc.cur++
	}
	if pc.cur >= mmCompressNEntries {
		return -3003
	}
	pc.w[pc.cur] = c | 0x4000000
	return 0
}

// mmCompressPCAddT adds an entry with tag 't' to pc.
// Mirrors C mm_compress_pc_add_t.
func mmCompressPCAddT(pc *mmCompress, t uint32) int {
	t %= 3
	if t == 0 {
		return 0
	}
	t |= 0x1000000
	if pc.w[pc.cur]&0x1000000 != 0 {
		return -4001
	}
	if pc.w[pc.cur] != 0 {
		pc.cur++
	}
	if pc.cur >= mmCompressNEntries {
		return -4003
	}
	pc.w[pc.cur] = t
	return 0
}

// mmCompressPC compresses the word in pc into the
// 256-bit integer pN (lowest digit first). It returns
// 0 on success and a negative value on failure.
// Mirrors C mm_compress_pc.
func mmCompressPC(pc *mmCompress, pN *[4]uint64) int {
	pN[0], pN[1], pN[2], pN[3] = 0, 0, 0, 0
	var posN uint32
	if pc.nx == 0 {
		pN[0] = mat24.Mat24Order
		posN = 28
	} else {
		pN[0] = pc.nx
		posN = 64
	}

	last := uint32(0)
	for i := 0; i < mmCompressNEntries; i++ {
		tag := pc.w[i] >> 24
		c := pc.w[i] & 0xffffff
		switch tag {
		case 1:
			c %= 3
			if last&1 != 0 || c == 0 {
				return -20001
			}
			if last == 0 && posN == 28 {
				pN[0] += 1
			} else if last == 0 && posN == 64 {
				k := uint32(mmCompressType4(0x800000))
				insertInt256(pN, uint64(k), posN, 23)
				posN += 23
			}
			insertInt256(pN, uint64(c+1), posN, 2)
			posN += 1
		case 2:
			if last == 0 && posN == 28 {
				pN[0] += 2
			} else {
				return -20002
			}
			ki := mmindex.IndexLeech2ToSparse(c)
			if ki == 0 {
				return -20003
			}
			ke := mmindex.IndexSparseToExtern(ki)
			if ke < 300 || ke >= 300+98280 {
				return -20004
			}
			insertInt256(pN, uint64(ke), posN, 17)
			posN += 17
		case 4:
			if last&6 != 0 {
				return -20005
			}
			kc := mmCompressType4(c)
			if kc < 0 {
				return kc
			}
			insertInt256(pN, uint64(uint32(kc)), posN, 23)
			posN += 23
		case 0:
			continue
		default:
			return -20006
		}
		last = tag
		if posN > 255 {
			return -20007
		}
	}
	return 0
}

//////////////////////////////////////////////////
// Expand a compressed word (mm_compress.c).
//////////////////////////////////////////////////

// mmCompressPCExpandInt expands the 255-bit integer
// pN to a word of monster generators, returning the
// word or an error. Mirrors C
// mm_compress_pc_expand_int.
func mmCompressPCExpandInt(pN *[4]uint64) ([]uint32, error) {
	posN := uint32(28)
	withT := uint32(0)
	var m []uint32

	if pN[0] == 0 || pN[3]>>63 != 0 {
		return nil, compressError(-2)
	}
	p := uint32(pN[0] & 0xfffffff)
	if p < mat24.Mat24Order {
		var g n0.N0Elem
		g[0] = 0
		g[1] = uint32((pN[0] >> 28) & 0x7ff)
		g[2] = uint32((pN[0] >> 39) & 0x1fff)
		g[3] = uint32((pN[0] >> 52) & 0xfff)
		g[4] = p
		if n0.ReduceElement(&g) != 0 {
			var w [5]uint32
			n := n0.ToWord(&g, w[:])
			m = append(m, w[:n]...)
		}
		posN = 64
	} else {
		switch p {
		case mat24.Mat24Order:
			// Do nothing.
		case mat24.Mat24Order + 1:
			c := extractInt256(pN, 1, posN)
			posN++
			m = append(m, 0x50000001+c)
		case mat24.Mat24Order + 2:
			c := extractInt256(pN, 17, posN)
			posN += 17
			sp := mmindex.IndexExternToSparse(int(c))
			if sp == 0 {
				return nil, compressError(-3)
			}
			// TODO(nealpatel): re-evaluate after porting;
			// IndexSparseToLeech2 returns 0 on failure.
			// Unlike the other two call sites, this input
			// derives from untrusted decompressed data, so
			// the zero check is required. The sentinel is
			// unambiguous: 0 is the zero vector (type 0),
			// never a valid short/type-2 result.
			v := mmindex.IndexSparseToLeech2(sp)
			if v == 0 {
				return nil, compressError(-4)
			}
			var sub [6]uint32
			status := leech.GenLeech2ReduceType2(v, sub[:])
			if status < 0 {
				return nil, compressError(int32(status))
			}
			if status > 6 {
				return nil, compressError(-5)
			}
			invertWord(sub[:status])
			m = append(m, sub[:status]...)
			withT = 1
		default:
			return nil, compressError(-6)
		}
	}

	for {
		c := extractInt256(pN, 23+withT, posN)
		posN += 23 + withT
		if withT != 0 && c >= 2 {
			m = append(m, 0x50000001+(c&1))
		}
		c >>= withT
		withT = 1
		if c < 2 {
			return m, nil
		}
		ec := mmCompressExpandType4(c)
		if ec < 0 {
			return nil, compressError(int32(ec))
		}
		var sub [6]uint32
		status := leech.GenLeech2ReduceType4(uint32(ec), sub[:])
		if status < 0 {
			return nil, compressError(int32(status))
		}
		if status > 6 {
			return nil, compressError(-6)
		}
		invertWord(sub[:status])
		m = append(m, sub[:status]...)
	}
}

//////////////////////////////////////////////////
// GtWord reduction engine (mm_shorten.c).
//
// A monster word is held as a list of subwords. Each
// subword is a reduced word g of generators of G_x0
// followed by tau^t_exp. img_Omega is the image of
// the standard frame Omega under g. The list is kept
// in a slice with explicit prev/next links so the C
// circular doubly-linked-list logic ports directly.
//////////////////////////////////////////////////

const (
	maxGtWordData        = 24
	omegaFrame           = 0x800000
	tltConversion uint32 = 0x6345127
)

// gtSubword stores a subword g tau^tExp of the
// monster, with g a word of G_x0 generators.
type gtSubword struct {
	eof      bool
	imgOmega uint32
	tExp     uint32
	reduced  bool
	prev     int
	next     int
	data     []uint32
}

// clear resets s to the empty (non-eof) subword.
// Mirrors C gt_subword_clear.
func (s *gtSubword) clear() {
	s.data = s.data[:0]
	s.tExp = 0
	s.imgOmega = omegaFrame
	s.reduced = true
	s.eof = false
}

// GtWord is the reduction buffer holding a monster
// word as a circular doubly-linked list of subwords.
// node is the slice of nodes; end is the EOF index;
// cur is the current node index; free is the head of
// the recycled-node free list (-1 if empty).
type GtWord struct {
	node       []gtSubword
	end        int
	cur        int
	free       int
	reduceMode int
}

// NewGtWord returns an empty reduction buffer in the
// given reduce mode. Mirrors C gt_word_alloc with the
// list initialized by set_eof_word.
func NewGtWord(mode int) *GtWord {
	if mode > 2 {
		mode = 1
	}
	g := &GtWord{free: -1, reduceMode: mode}
	end := g.newNode()
	g.end = end
	g.cur = end
	g.node[end].prev = end
	g.node[end].next = end
	g.node[end].clear()
	g.node[end].eof = true
	return g
}

// newNode allocates a fresh node index, reusing the
// free list when possible. Mirrors the allocation in
// _gt_word_new_subword / gt_word_insert.
func (g *GtWord) newNode() int {
	if g.free >= 0 {
		idx := g.free
		g.free = g.node[idx].next
		return idx
	}
	g.node = append(g.node, gtSubword{})
	return len(g.node) - 1
}

// insert adds an empty subword after the current node
// and makes it current. Mirrors C gt_word_insert.
func (g *GtWord) insert() {
	idx := g.newNode()
	g.node[idx].clear()
	cur := g.cur
	next := g.node[cur].next
	g.node[idx].next = next
	g.node[idx].prev = cur
	g.node[cur].next = idx
	g.node[next].prev = idx
	g.cur = idx
}

// delete removes the current node and makes its
// predecessor current. Mirrors C gt_word_delete. It
// returns a negative value on an attempt to delete the
// EOF mark.
func (g *GtWord) delete() int {
	cur := g.cur
	if g.node[cur].eof {
		return gtErr(1, 2)
	}
	next := g.node[cur].next
	prev := g.node[cur].prev
	g.cur = prev
	g.node[next].prev = prev
	g.node[prev].next = next
	g.node[cur].next = g.free
	g.free = cur
	return 0
}

// ReduceWord reduces a G_x0 word a to canonical form,
// returning the reduced word. It is the Go equivalent
// of C xsp2co1_reduce_word.
func ReduceWord(a []uint32) ([]uint32, int) {
	// 26 = the G_x0 element word count (xsp2co1's
	// internal representation: 1 Leech-mod-3 word + 25
	// qstate words).
	var elem [26]uint64
	if err := xsp2co1.SetElemWord(elem[:], a); err != nil {
		return nil, -1
	}
	out := make([]uint32, 10)
	n := xsp2co1.ElemToWord(elem[:], out)
	return out[:n], n
}

// appendSubPart appends the maximal monster prefix of
// a to the current subword, returning the number of
// atoms consumed or a negative value on failure.
// Mirrors C gt_word_append_sub_part.
func (g *GtWord) appendSubPart(a []uint32) int {
	cur := g.cur
	if g.node[cur].eof {
		return 0
	}
	n := uint32(len(a))
	reduced := g.node[cur].reduced
	w := append([]uint32(nil), g.node[cur].data...)
	imgOmegaLen := uint32(len(w))
	e := g.node[cur].tExp
	i := uint32(0)
	var gn n0.N0Elem

	for {
		gn = n0.N0Elem{}
		gn[0] = e
		i += n0.MulWordScan(&gn, a[i:])
		e = n0.RightCosetNx0(gn[:])
		var wbuf [5]uint32
		j := n0.ToWord(&gn, wbuf[:])
		w = append(w, wbuf[:j]...)
		reduced = reduced && j == 0
		if e != 0 || i >= n {
			break
		}
		v := a[i]
		i++
		if v&0x70000000 != 0x60000000 {
			return gtErr(2, 1)
		}
		v ^= 0 - ((v >> 31) & 1)
		v = (v & 0xfffffff) % 3
		if v != 0 {
			w = append(w, v+0x60000000)
			reduced = false
		}
		if uint32(len(w)) > maxGtWordData+40-6 {
			rw, res := ReduceWord(w)
			if res < 0 {
				return gtVarErr(1, res)
			}
			w = append([]uint32(nil), rw...)
			imgOmegaLen = 0
			g.node[cur].imgOmega = omegaFrame
			reduced = true
		}
	}
	g.node[cur].tExp = e
	if uint32(len(w)) > maxGtWordData-1 {
		rw, res := ReduceWord(w)
		if res < 0 {
			return gtVarErr(2, res)
		}
		w = append([]uint32(nil), rw...)
		imgOmegaLen = 0
		g.node[cur].imgOmega = omegaFrame
		reduced = true
	}
	g.node[cur].imgOmega = leech2OpWordLeech2(g.node[cur].imgOmega,
		w[imgOmegaLen:])
	g.node[cur].data = w
	g.node[cur].reduced = reduced
	return int(i)
}

// leech2OpWordLeech2 returns the image of the Leech
// vector v under the G_x0 word a (forward action). It
// is the single-vector form of C
// gen_leech2_op_word_leech2 with back = 0.
func leech2OpWordLeech2(v uint32, a []uint32) uint32 {
	buf := [1]uint32{v}
	leech.GenLeech2OpWordLeech2Many(buf[:], a, false)
	return buf[0]
}

// complexity returns the minimum number of tag-l
// atoms needed to represent the G_x0 element with the
// given image of Omega. Mirrors C complexity.
func complexity(imgOmega uint32) uint32 {
	if imgOmega&0x800 != 0 {
		return 3
	}
	if imgOmega&0x7ff800 == 0 {
		if imgOmega&0x7fffff != 0 {
			return 1
		}
		return 0
	}
	weight := (uint32(mat24.ThetaTable((imgOmega>>12)&0x7ff)) >> 12) & 1
	return 2 + weight
}

// overComplex reports whether the G_x0 word a (of an
// element with the given l-weight) is complex enough
// that lazy reduction is worthwhile. Mirrors C
// over_complex.
func overComplex(a []uint32, lWeight uint32) bool {
	if len(a) > 12 {
		return true
	}
	var nAtoms [8]uint32
	for _, v := range a {
		nAtoms[(v>>28)&7]++
	}
	if nAtoms[6] > lWeight {
		return true
	}
	if nAtoms[2] > lWeight+1 {
		return true
	}
	if nAtoms[3] > 1 {
		return true
	}
	if nAtoms[4] > 1 {
		return true
	}
	if nAtoms[1] > 2 {
		return true
	}
	return false
}

// reduceSub reduces the current subword. Mode bit 0
// forces reduction; bit 1 also moves an N_x0 prefix to
// the previous subword. Mirrors C gt_word_reduce_sub.
func (g *GtWord) reduceSub(subMode uint32) int {
	cur := g.cur
	if g.node[cur].eof || len(g.node[cur].data) == 0 {
		g.node[cur].reduced = true
		return 0
	}
	if !g.node[cur].reduced {
		reduce := subMode & 1
		if reduce == 0 {
			lWeight := complexity(g.node[cur].imgOmega)
			if overComplex(g.node[cur].data, lWeight) {
				reduce = 1
			}
		}
		if reduce != 0 {
			rw, res := ReduceWord(g.node[cur].data)
			if res < 0 {
				return gtVarErr(3, res)
			}
			g.node[cur].data = append([]uint32(nil), rw...)
			g.node[cur].reduced = true
		}
	}
	if subMode < 2 || g.node[g.node[cur].prev].eof {
		return 0
	}
	w := g.node[cur].data
	i := 0
	for ; i < len(w); i++ {
		if w[i]&0x70000000 == 0x60000000 {
			break
		}
	}
	if i != 0 {
		prefix := append([]uint32(nil), w[:i]...)
		g.cur = g.node[cur].prev
		res := g.appendSubPart(prefix)
		g.cur = cur
		if res < 0 {
			return res
		}
		g.node[cur].data = append(g.node[cur].data[:0], w[i:]...)
		g.node[cur].reduced = g.node[cur].reduced && res == i
	}
	return 0
}

// ruleJoin tries to merge the current subword into its
// predecessor. It returns 1 if a rule applied (and
// repositions cur to the first changed node), 0
// otherwise, or a negative value on error. Mirrors C
// gt_word_rule_join.
func (g *GtWord) ruleJoin() int {
	cur := g.cur
	if g.node[cur].eof {
		return 0
	}
	prev := g.node[cur].prev
	if g.node[prev].eof {
		if g.node[cur].imgOmega == omegaFrame && g.node[cur].tExp == 0 {
			rw, res := ReduceWord(g.node[cur].data)
			if res < 0 {
				return gtVarErr(4, res)
			}
			if res == 0 {
				g.delete()
				g.cur = g.node[g.cur].next
				return 1
			}
			g.node[cur].data = append([]uint32(nil), rw...)
			g.node[cur].reduced = true
		}
		return 0
	}

	if g.node[prev].tExp == 0 {
		g.delete()
		if g.cur != prev {
			return gtErr(3, 1)
		}
		joined := append([]uint32(nil), g.node[cur].data...)
		joined = append(joined, 0x50000000+g.node[cur].tExp)
		res := g.appendSubPart(joined)
		if res != len(joined) {
			if res < 0 {
				return res
			}
			return gtVarErr(5, res)
		}
		return 1
	} else if g.node[cur].imgOmega == omegaFrame {
		g.delete()
		if g.cur != prev {
			return gtErr(3, 2)
		}
		rw, res := ReduceWord(g.node[cur].data)
		if res < 0 {
			return gtVarErr(6, res)
		}
		joined := append([]uint32(nil), rw...)
		joined = append(joined, 0x50000000+g.node[cur].tExp)
		res2 := g.appendSubPart(joined)
		if res2 != len(joined) {
			if res2 < 0 {
				return res2
			}
			return gtVarErr(7, res2)
		}
		return 1
	}
	return 0
}

// ruleTxiT applies the rule t^e1 xi^e2 t^e3 ->
// xi^e4 t^e5 xi^e6 to the current subword and its
// predecessor. It returns 1 if applied, 0 otherwise,
// or a negative value on error. Mirrors C
// gt_word_rule_t_xi_t.
func (g *GtWord) ruleTxiT() int {
	cur := g.cur
	if g.node[cur].eof {
		return 0
	}
	if g.node[cur].tExp == 0 {
		return 0
	}
	prev := g.node[cur].prev
	if g.node[prev].eof {
		return 0
	}
	if g.node[prev].tExp == 0 {
		return 0
	}
	if complexity(g.node[cur].imgOmega) != 1 {
		return 0
	}
	if res := g.reduceSub(3); res < 0 {
		return res
	}
	if g.node[cur].tExp < 1 || g.node[cur].tExp > 2 {
		return -501
	}
	if g.node[prev].tExp < 1 || g.node[prev].tExp > 2 {
		return -502
	}
	var pi uint32
	switch len(g.node[cur].data) {
	case 2:
		pi = g.node[cur].data[1]
		if pi>>28 != 2 {
			return -503
		}
		fallthrough
	case 1:
		e := g.node[cur].data[0]
		if e < 0x60000001 || e > 0x60000002 {
			return -0x504
		}
	default:
		return -505
	}

	// f = F_TLT(e) with e, f packed as in mm_shorten.c.
	e := ((g.node[cur].data[0] - 1) & 1) << 1
	e += g.node[prev].tExp - 1
	e += (g.node[cur].tExp - 1) << 2
	f := (tltConversion >> (e << 2)) & 7

	// Store xi^((f>>2)+1) in the current subword.
	g.node[cur].clear()
	l := []uint32{0x60000001 + ((f >> 2) & 1)}
	if pi != 0 {
		l = append(l, pi)
	}
	if g.appendSubPart(l) != len(l) {
		return -0x508
	}

	// Kill the trailing t^e1 in the predecessor and
	// append xi^((f&1)+1) t^((f>>1)+1) to it.
	g.cur = prev
	g.node[prev].tExp = 0
	l2 := []uint32{0x60000001 + (f & 1), 0x50000001 + ((f >> 1) & 1)}
	g.cur = prev
	if g.appendSubPart(l2) != 2 {
		return -0x507
	}
	return 1
}

// reduceInput applies the join and t-xi-t rules across
// the word as it is built. Mirrors C
// gt_word_reduce_input.
func (g *GtWord) reduceInput() int {
	for !g.node[g.cur].eof {
		res := g.ruleJoin()
		if res < 0 {
			return res
		}
		if res == 0 {
			res = g.ruleTxiT()
			if res < 0 {
				return res
			}
		}
		if res == 0 {
			g.cur = g.node[g.cur].next
		}
	}
	return 0
}

// AppendWord appends the monster word a to g. Mirrors
// C gt_word_append.
func (g *GtWord) AppendWord(a []uint32) int {
	g.cur = g.node[g.end].prev
	i := 0
	for i < len(a) {
		g.insert()
		res := g.appendSubPart(a[i:])
		if res <= 0 {
			if res < 0 {
				return res
			}
			return gtVarErr(8, res)
		}
		i += res
		if g.reduceMode != 0 {
			if res := g.reduceInput(); res < 0 {
				return res
			}
			g.cur = g.node[g.end].prev
		}
	}
	g.cur = g.end
	return 0
}

// Reduce performs the full reduction of the word in g.
// Mirrors C gt_word_reduce. It returns a subgroup code
// (>=4 for G_x0 . N_0) or a negative value on error.
func (g *GtWord) Reduce() int {
	subMode := uint32(3)
	if g.reduceMode == 2 {
		subMode = 2
	}
	old := g.cur

	g.cur = g.node[g.end].prev
	for !g.node[g.cur].eof {
		if res := g.reduceSub(subMode); res < 0 {
			return res
		}
		g.cur = g.node[g.cur].prev
	}
	g.cur = old

	fst := g.node[g.end].next
	if fst == g.end {
		return 3
	}
	if g.node[fst].next != g.end {
		return 0
	}

	res := 4
	if g.node[fst].imgOmega&0x7fffff == 0 {
		res |= 1
	}
	if g.node[fst].tExp == 0 {
		res |= 2
	}
	g.cur = g.node[g.end].prev
	res1 := g.reduceSub(1)
	if res1 < 0 {
		return res
	}
	g.cur = old
	return res
}

// toMmCompress reduces g and stores it in pc. Mirrors
// C gt_word_to_mm_compress.
func (g *GtWord) toMmCompress(pc *mmCompress) int {
	mmCompressPCInit(pc)
	if status := g.Reduce(); status < 0 {
		return status
	}
	fst := g.node[g.end].next
	for cur := fst; !g.node[cur].eof; cur = g.node[cur].next {
		if !g.node[cur].reduced {
			return -100001
		}
		data := g.node[cur].data
		status := 0
		if cur == fst {
			status = mmCompressPCAddNx(pc, data)
			if status < 0 {
				return status
			}
		}
		if status < len(data) && data[status]>>28 != 6 {
			return -1000002
		}
		status = mmCompressPCAddType4(pc, g.node[cur].imgOmega)
		if status < 0 {
			return status
		}
		status = mmCompressPCAddT(pc, g.node[cur].tExp)
		if status < 0 {
			return status
		}
	}
	return 0
}

// GtWordStore writes the word held in g to out and
// returns its length, or a negative value if out is
// too small (capacity maxlen). Each subword
// contributes its reduced G_x0 word followed by a tau
// atom 0x50000000 + tExp when tExp is nonzero. Mirrors
// C gt_word_store.
func (g *GtWord) GtWordStore(out []uint32, maxlen int) int {
	n := 0
	for cur := g.node[g.end].next; !g.node[cur].eof; cur = g.node[cur].next {
		data := g.node[cur].data
		e := g.node[cur].tExp
		k := len(data)
		if n+k > maxlen {
			return gtErr(4, 1)
		}
		n += copy(out[n:], data)
		if e != 0 {
			if n+1 > maxlen {
				return gtErr(4, 1)
			}
			out[n] = 0x50000000 + e
			n++
		}
	}
	return n
}

//////////////////////////////////////////////////
// The gt_word shortener wrapper (mm_shorten.c).
//////////////////////////////////////////////////

// GtWordShorten reduces the monster word g and stores
// the reduced word in out (capacity n1max), returning
// its length. mode selects the reduce mode as in
// NewGtWord. It returns a negative value on failure,
// e.g. if the reduced word would exceed n1max. C
// function gt_word_shorten.
func GtWordShorten(g []uint32, out []uint32, n1max int, mode int) int {
	gw := NewGtWord(mode)
	res := gw.AppendWord(g)
	if res < 0 {
		return gtShortenErr(2, res)
	}
	res = gw.Reduce()
	if res < 0 {
		return gtShortenErr(3, res)
	}
	n1 := gtWordLength(gw)
	if n1 > n1max {
		return gtShortenErr(4, -1)
	}
	res = gw.GtWordStore(out, n1)
	if res >= 0 {
		return res
	}
	return gtShortenErr(5, res)
}

// gtShortenErr encodes the (status, res) failure pair
// of GtWordShorten the same way C gt_word_shorten does:
// ((-status << 24) + res) | 0x80000000, returned as a
// signed value.
func gtShortenErr(status, res int) int {
	return int(int32(uint32((-status<<24)+res) | 0x80000000))
}

// gtWordLength returns the length of the word held in
// gw, counting each subword's reduced G_x0 word plus a
// tau atom when tExp is nonzero. C function
// gt_word_length.
func gtWordLength(gw *GtWord) int {
	length := 0
	for cur := gw.node[gw.end].next; !gw.node[cur].eof; cur = gw.node[cur].next {
		length += len(gw.node[cur].data)
		if gw.node[cur].tExp != 0 {
			length++
		}
	}
	return length
}

//////////////////////////////////////////////////
// Error helpers and public entry points.
//////////////////////////////////////////////////

// gtErr packs a (source, error) pair as a negative
// status. Mirrors C _ERROR.
func gtErr(source, err int) int {
	return -(source << 16) - err
}

// gtVarErr packs a (source, res) pair as a negative
// status. Mirrors C _VAR_ERROR.
func gtVarErr(source, res int) int {
	return -((source + 1) << 24) + int(uint32(res)&0xffffff)
}

// compressError builds an error from a negative
// compress/expand status code.
func compressError(status int32) error {
	return fmt.Errorf("cgt: word expansion failed, status %d", status)
}

// CompressAsInt maps the reduced word w to its
// 255-bit integer identifier and returns its lowest
// 64-bit digit. It runs the GtWord reduction engine
// and the compressor. It panics on an internal
// failure, mirroring the contract of GtWord.as_int.
func CompressAsInt(w []uint32) uint64 {
	g := NewGtWord(1)
	if res := g.AppendWord(w); res < 0 {
		panic(fmt.Sprintf("CompressAsInt: gt_word_append failed, status %d", res))
	}
	var pc mmCompress
	if res := g.toMmCompress(&pc); res < 0 {
		panic(fmt.Sprintf("CompressAsInt: gt_word_to_mm_compress failed, status %d", res))
	}
	var pN [4]uint64
	if res := mmCompressPC(&pc, &pN); res != 0 {
		panic(fmt.Sprintf("CompressAsInt: mm_compress_pc failed, status %d", res))
	}
	return pN[0]
}

// ExpandInt reconstructs an atom word from the
// 255-bit integer identifier held in the lowest 64-bit
// digit n.
func ExpandInt(n uint64) ([]uint32, error) {
	pN := [4]uint64{n, 0, 0, 0}
	return mmCompressPCExpandInt(&pN)
}
