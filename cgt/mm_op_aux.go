package cgt

import "patel.codes/cgt/mmindex"

// This file ports the field-access primitives and
// representation conversions from mm_aux.c.

// getMMV returns entry i (internal index) of vector
// mv modulo p, reduced. C mm_aux_get_mmv.
func getMMV(p int, mv []uint64, i uint32) uint8 {
	if mmAuxBadP(p) {
		return 0
	}
	c := mmvConst(p)
	j := uint(c & 7) // LOG_INT_FIELDS
	if i >= mmAuxLenV {
		return 0
	}
	sh := (i & ((1 << j) - 1)) << (6 - j)
	res := uint64((mv[i>>j] >> sh)) & uint64(p)
	pb := uint((c >> 15) & 15) // P_BITS
	return uint8((res + ((res + 1) >> pb)) & uint64(p))
}

// putMMV sets entry i (internal index) of vector mv
// to value. Writes both twin locations if any. C
// mm_aux_put_mmv.
func putMMV(p int, value uint8, mv []uint64, i uint32) {
	if mmAuxBadP(p) {
		return
	}
	j := uint(mmvConst(p) & 7)
	v1 := uint64(value) & uint64(p)
	mask := uint64(p)
	iTwin := mmindex.IndexCheckIntern(int(i))
	if iTwin < 0 {
		return
	}
	sh := (i & ((1 << j) - 1)) << (6 - j)
	mv[i>>j] = (mv[i>>j] &^ (mask << sh)) | (v1 << sh)
	if iTwin == 0 {
		return
	}
	it := uint32(iTwin)
	sh = (it & ((1 << j) - 1)) << (6 - j)
	mv[it>>j] = (mv[it>>j] &^ (mask << sh)) | (v1 << sh)
}

// addMMV adds value to entry i (internal index) of
// mv modulo p, updating both twin locations. C
// mm_aux_add_mmv.
func addMMV(p int, value uint8, mv []uint64, i uint32) {
	if mmAuxBadP(p) {
		return
	}
	c := mmvConst(p)
	j := uint(c & 7)
	pb := uint((c >> 15) & 15)
	v1 := uint64(value) & uint64(p)
	mask := uint64(p)
	iTwin := mmindex.IndexCheckIntern(int(i))
	if iTwin < 0 {
		return
	}
	sh := (i & ((1 << j) - 1)) << (6 - j)
	old := (mv[i>>j] >> sh) & uint64(p)
	v1 += old
	v1 = (v1 + ((v1 + 1) >> pb)) & uint64(p)
	mv[i>>j] = (mv[i>>j] &^ (mask << sh)) | (v1 << sh)
	if iTwin == 0 {
		return
	}
	it := uint32(iTwin)
	sh = (it & ((1 << j) - 1)) << (6 - j)
	mv[it>>j] = (mv[it>>j] &^ (mask << sh)) | (v1 << sh)
}

// readMMV32 reads len rows of 32 entries from mv
// (starting at row i) into b, reduced modulo p. C
// mm_aux_read_mmv32.
func readMMV32(p int, mv []uint64, i uint32, b []uint8, length uint32) {
	c := mmvConst(p)
	sh := uint((c >> 15) & 15)    // P_BITS
	logF := uint((c >> 9) & 3)    // LOG_FIELD_BITS
	fb := uint(1 << logF)         // bits per field
	nFields := uint32(64) >> logF // fields per uint64
	off := (i << 5) >> (6 - logF)
	n := (length << 5) >> (6 - logF) // number of uint64 words
	mvi := off
	bi := 0
	for ; n > 0; n-- {
		src := mv[mvi]
		mvi++
		for f := uint32(0); f < nFields; f++ {
			tmp := (src >> (uint(f) * fb)) & uint64(p)
			b[bi] = uint8((tmp + ((tmp + 1) >> sh)) & uint64(p))
			bi++
		}
	}
}

// writeMMV32 writes len rows of 32 entries from b
// into mv starting at row i. C mm_aux_write_mmv32.
func writeMMV32(p int, b []uint8, mv []uint64, i uint32, length uint32) {
	c := mmvConst(p)
	logF := uint((c >> 9) & 3)
	fb := uint(1 << logF)
	nFields := uint32(64) >> logF
	off := (i << 5) >> (6 - logF)
	n := (length << 5) >> (6 - logF)
	mvi := off
	bi := 0
	for ; n > 0; n-- {
		var dest uint64
		for f := uint32(0); f < nFields; f++ {
			dest += uint64(b[bi]) << (uint(f) * fb)
			bi++
		}
		mv[mvi] = dest
		mvi++
	}
}

// readMMV24 reads len rows of 24 entries from mv
// (each padded to a 32-entry row, starting at row i)
// into b, reduced modulo p. C mm_aux_read_mmv24.
func readMMV24(p int, mv []uint64, i uint32, b []uint8, length uint32) {
	c := mmvConst(p)
	sh := uint((c >> 15) & 15)
	logF := uint((c >> 9) & 3)
	fb := uint(1 << logF)
	v24 := uint32(32) >> uint(c&7) // uint64 words per 24-entry row
	nFields := uint32(64) >> logF
	mvi := (i << 5) >> (6 - logF)
	bi := 0
	for ; length > 0; length-- {
		rowStart := mvi
		got := uint32(0)
		for w := uint32(0); w < v24 && got < 24; w++ {
			src := mv[mvi]
			mvi++
			for f := uint32(0); f < nFields && got < 24; f++ {
				tmp := (src >> (uint(f) * fb)) & uint64(p)
				b[bi] = uint8((tmp + ((tmp + 1) >> sh)) & uint64(p))
				bi++
				got++
			}
		}
		// Advance by the full v24 stride to skip the
		// padding word the 24-entry read may leave
		// untouched (8-bit fields, LOG_FIELD_BITS=3).
		// Matches writeMMV24 and the mv += (8<<LOG_F)/
		// INT_BITS adjustment in C mm_aux_read_mmv24.
		mvi = rowStart + v24
	}
}

// writeMMV24 writes len rows of 24 entries from b
// into mv (each padded to a 32-entry row, starting
// at row i), zeroing the slack. C
// mm_aux_write_mmv24.
func writeMMV24(p int, b []uint8, mv []uint64, i uint32, length uint32) {
	c := mmvConst(p)
	logF := uint((c >> 9) & 3)
	fb := uint(1 << logF)
	v24 := uint32(32) >> uint(c&7)
	nFields := uint32(64) >> logF
	mvi := (i << 5) >> (6 - logF)
	bi := 0
	for ; length > 0; length-- {
		got := uint32(0)
		for w := uint32(0); w < v24; w++ {
			var dest uint64
			for f := uint32(0); f < nFields; f++ {
				idxInRow := w*nFields + f
				if idxInRow < 24 {
					dest += (uint64(b[bi]) & uint64(p)) << (uint(f) * fb)
					bi++
					got++
				}
			}
			mv[mvi] = dest
			mvi++
		}
		_ = got
	}
}

// small24Expand maps the 852 tag-A/B/C entries of
// b_src to the 3*24*24 entries of b_dest (three
// symmetric matrices). C mm_aux_small24_expand.
func small24Expand(bSrc, bDest []uint8) {
	si := 0
	for j0 := 0; j0 < 24*25; j0 += 25 {
		bDest[j0] = bSrc[si]
		si++
		bDest[j0+1152] = 0
		bDest[j0+576] = 0
	}
	ti := 0 // b_transpose base (== bDest)
	di := 0 // b_dest cursor
	for j0 := 0; j0 < 24; j0++ {
		j1e := 24 * j0
		for j1t := 0; j1t < j1e; j1t += 24 {
			bDest[ti+j1t] = bSrc[si]
			bDest[di] = bSrc[si]
			bDest[ti+j1t+576] = bSrc[si+276]
			bDest[di+576] = bSrc[si+276]
			bDest[ti+j1t+1152] = bSrc[si+552]
			bDest[di+1152] = bSrc[si+552]
			di++
			si++
		}
		di += 24 - j0
		ti++
	}
}

// small24Compress reverses small24Expand: it maps
// the 3*24*24 entries of b_src to the 852 tag-A/B/C
// entries of b_dest. C mm_aux_small24_compress.
func small24Compress(bSrc, bDest []uint8) {
	di := 0
	for j0 := 0; j0 < 24*25; j0 += 25 {
		bDest[di] = bSrc[j0]
		di++
	}
	si := 0
	for j0 := 0; j0 < 24; j0++ {
		for j1 := j0; j1 > 0; j1-- {
			bDest[di] = bSrc[si]
			bDest[di+276] = bSrc[si+576]
			bDest[di+552] = bSrc[si+1152]
			di++
			si++
		}
		si += 24 - j0
	}
}

// mmvToBytes converts vector mv (internal) to the
// external byte representation b (length 196884),
// reduced modulo p. C mm_aux_mmv_to_bytes.
func mmvToBytes(p int, mv []uint64, b []uint8) {
	if mmAuxBadP(p) {
		return
	}
	var b1 [3 * 576]uint8
	readMMV24(p, mv, 0, b1[:], 72)
	small24Compress(b1[:], b)
	readMMV32(p, mv, mmAuxOfsT/32, b[mmAuxXofsT:], 2*759)
	readMMV24(p, mv, mmAuxOfsX/32, b[mmAuxXofsX:], 6144)
}

// bytesToMMV converts the external byte
// representation b (length 196884) to vector mv
// (internal). Each entry must satisfy 0 <= x <= p. C
// mm_aux_bytes_to_mmv.
func bytesToMMV(p int, b []uint8, mv []uint64) {
	if mmAuxBadP(p) {
		return
	}
	var b1 [3 * 576]uint8
	small24Expand(b, b1[:])
	writeMMV24(p, b1[:], mv, 0, 72)
	writeMMV32(p, b[mmAuxXofsT:], mv, mmAuxOfsT/32, 759*2)
	writeMMV24(p, b[mmAuxXofsX:], mv, mmAuxOfsX/32, 6144)
}

// zeroMMV zeros the first MMVSize(p) entries of mv. C
// mm_aux_zero_mmv.
func zeroMMV(p int, mv []uint64) {
	if mmAuxBadP(p) {
		return
	}
	n := MMVSize(p)
	for i := 0; i < n; i++ {
		mv[i] = 0
	}
}

// reduceMMVFields reduces the first nfields entries
// of mv to a canonical form (mapping 1..1 to 0..0).
// C mm_aux_reduce_mmv_fields. Returns 0 on success,
// -1 for bad p, -2 for a stray bit.
func reduceMMVFields(p int, mv []uint64, nfields uint32) int {
	if mmAuxBadP(p) {
		return -1
	}
	c := mmvConst(p)
	sh := uint((c >> 15) & 15) // P_BITS
	lif := uint(c & 7)         // LOG_INT_FIELDS
	n := nfields >> lif        // number of uint64 words
	mask1 := mmAuxTblReduce[2*sh-4]
	maskP := mmAuxTblReduce[2*sh-3]
	if sh&(sh-1) != 0 {
		// P_BITS not a power of two
		var acc uint64
		mvi := uint32(0)
		for i := n; i > 0; i-- {
			data := mv[mvi]
			acc |= data
			data &= maskP
			cy := (data + mask1) &^ maskP
			data += (cy >> sh) - cy
			mv[mvi] = data
			mvi++
		}
		if acc&^maskP != 0 {
			return -2
		}
	} else {
		// P_BITS a power of two
		sh >>= 1
		mvi := uint32(0)
		for i := n; i > 0; i-- {
			data := mv[mvi]
			acc := data & (data >> sh) & maskP
			cy := (acc + mask1) &^ maskP
			data += (cy >> sh) - (cy << sh)
			mv[mvi] = data
			mvi++
		}
	}
	return 0
}

// reduceMMV reduces all entries of mv. C
// mm_aux_reduce_mmv.
func reduceMMV(p int, mv []uint64) int {
	return reduceMMVFields(p, mv, mmAuxLenV)
}

// check24 checks that no out-of-range entries (>= 24
// in a 24-entry row) are nonzero in length rows. C
// helper check24.
func check24(p int, mv []uint64, length uint32) int {
	c := mmvConst(p)
	d := 5 - uint(c&7) // 5 - LOG_INT_FIELDS
	var acc, mask uint64
	mvi := 0
	switch d {
	case 0:
		mask = 0xffff000000000000
		for ; length > 0; length-- {
			acc |= mv[mvi] & mask
			mvi++
		}
	case 1:
		mask = 0xffffffff00000000
		for ; length > 0; length-- {
			acc |= mv[mvi+1] & mask
			mvi += 2
		}
	case 2:
		for ; length > 0; length-- {
			acc |= mv[mvi+3]
			mvi += 4
		}
	}
	if acc != 0 {
		return -3
	}
	return 0
}

// checkSym verifies the symmetric A/B/C part is
// actually symmetric and has zero diagonal for B/C.
// buffer must hold at least 72*32 bytes; on return
// it holds the A/B/C rows. C helper check_sym.
func checkSym(p int, mv []uint64, buffer []uint8) int {
	readMMV32(p, mv, 0, buffer, 72)
	var acc uint32
	for i := 768; i < 1536; i += 33 {
		acc |= uint32(buffer[i]) | uint32(buffer[i+768])
	}
	if acc != 0 {
		return -4
	}
	acc = 0
	pRow := 0
	for pCol := 0; pCol < 24; pCol++ {
		for i := 0; i < 24; i++ {
			acc |= uint32(buffer[pRow+i]^buffer[pCol+(i<<5)]) |
				uint32(buffer[pRow+i+768]^buffer[pCol+(i<<5)+768]) |
				uint32(buffer[pRow+i+1536]^buffer[pCol+(i<<5)+1536])
		}
		pRow += 32
	}
	if acc != 0 {
		return -5
	}
	return 0
}

// checkMMVBuffer reduces mv and validates it. buffer
// must hold 72*32 bytes. C helper check_mmv_buffer.
func checkMMVBuffer(p int, mv []uint64, buffer []uint8) int {
	if i := reduceMMV(p, mv); i != 0 {
		return i
	}
	if i := check24(p, mv, 72); i != 0 {
		return i
	}
	lif := uint(mmvConst(p) & 7)
	if i := check24(p, mv[mmAuxOfsX>>lif:], 6144); i != 0 {
		return i - 100
	}
	return checkSym(p, mv, buffer)
}

// checkMMV checks vector mv for errors, reducing it
// as a side effect. C mm_aux_check_mmv.
func checkMMV(p int, mv []uint64) int {
	var buffer [72 * 32]uint8
	return checkMMVBuffer(p, mv, buffer[:])
}

// indexMMV fills a with the sorted internal indices
// of nonzero entries of mv, terminated by 0xffff. C
// mm_aux_index_mmv. Returns the array length or a
// negative error.
func indexMMV(p int, mv []uint64, a []uint16, l uint32) int {
	status := reduceMMV(p, mv)
	if status < 0 {
		return status
	}
	if l == 0 {
		return -3
	}
	nMax := uint32(MMVSize(p))
	i := uint32(0)
	for n := uint32(0); n < nMax; n++ {
		if mv[n] != 0 {
			a[i] = uint16(n)
			i++
			if i >= l {
				return -3
			}
		}
	}
	a[i] = 0xffff
	i++
	return int(i)
}

// mmvToSparse converts vector mv (internal) to
// sparse representation in sp, returning the length
// or a negative error. sp must have length 196884. C
// mm_aux_mmv_to_sparse.
func mmvToSparse(p int, mv []uint64, sp []uint32) int {
	var b [72 * 32]uint8
	if status := checkMMVBuffer(p, mv, b[:]); status != 0 {
		return status
	}
	c := mmvConst(p)
	fbits := uint((c >> 11) & 15) // FIELD_BITS
	lif := uint(c & 7)            // LOG_INT_FIELDS
	sh := uint(8 - 6 + lif)       // 8 - LOG_FIELD_BITS
	isp := 0

	// tags A, B, C
	pRow := 0
	for row := 0; row < 3; row++ {
		for i := 0; i < 24; i++ {
			for j := 0; j <= i; j++ {
				if value := uint32(b[pRow+j]); value != 0 {
					sp[isp] = 0x2000000 + (uint32(row) << 25) +
						(uint32(i) << 14) + (uint32(j) << 8) + value
					isp++
				}
			}
			pRow += 32
		}
	}

	mvi := mmAuxOfsT >> lif
	rowEnd := (mmAuxOfsX - mmAuxOfsT) >> lif
	for row := 0; row < rowEnd; row++ {
		source := mv[mvi]
		mvi++
		if source == 0 {
			continue
		}
		ofs := 0x8000000 + (uint32(row) << (8 + lif))
		for j := uint(0); j < 64; j += fbits {
			if value := uint32((source >> j)) & uint32(p); value != 0 {
				sp[isp] = ofs + (uint32(j) << sh) + value
				isp++
			}
		}
	}

	rowEnd = (mmAuxLenV - mmAuxOfsX) >> lif
	for row := 0; row < rowEnd; row++ {
		source := mv[mvi]
		mvi++
		if source == 0 {
			continue
		}
		ofs := 0x5000000 + (uint32(row) << (8 + lif))
		ofs += ofs & 0xfffe000
		for j := uint(0); j < 64; j += fbits {
			if value := uint32((source >> j)) & uint32(p); value != 0 {
				sp[isp] = ofs + (uint32(j) << sh) + value
				isp++
			}
		}
	}
	return isp
}

// mmvExtractSparse updates each entry of sp with the
// corresponding coordinate of mv. C
// mm_aux_mmv_extract_sparse.
func mmvExtractSparse(p int, mv []uint64, sp []uint32, length int) {
	if mmAuxBadP(p) {
		return
	}
	for i := 0; i < length; i++ {
		if sp[i]&mmSpaceTagY == 0 {
			continue
		}
		iIntern := mmindex.IndexSparseToIntern(sp[i])
		k := (uint32(getMMV(p, mv, uint32(iIntern))) ^ sp[i]) & uint32(p)
		sp[i] = (sp[i] & 0xffffff00) + k
	}
}

// mmvGetSparse extracts a single sparse entry. C
// mm_aux_mmv_get_sparse.
func mmvGetSparse(p int, mv []uint64, sp uint32) uint32 {
	a := [1]uint32{sp}
	mmvExtractSparse(p, mv, a[:], 1)
	return a[0]
}

// mmvAddSparse adds the sparse vector sp to mv. C
// mm_aux_mmv_add_sparse.
func mmvAddSparse(p int, sp []uint32, length int, mv []uint64) {
	if mmAuxBadP(p) {
		return
	}
	for i := 0; i < length; i++ {
		iIntern := mmindex.IndexSparseToIntern(sp[i])
		addMMV(p, uint8(sp[i]), mv, uint32(iIntern))
	}
}

// mmvSetSparse sets entries of mv from sparse vector
// sp. C mm_aux_mmv_set_sparse.
func mmvSetSparse(p int, mv []uint64, sp []uint32, length int) {
	if mmAuxBadP(p) {
		return
	}
	for i := 0; i < length; i++ {
		iIntern := mmindex.IndexSparseToIntern(sp[i])
		putMMV(p, uint8(sp[i]), mv, uint32(iIntern))
	}
}

// hashSections holds the section boundaries used by
// Hash. C array HASH_SECTIONS.
var hashSections = [8]uint32{
	mmAuxOfsA, mmAuxOfsB, mmAuxOfsC, mmAuxOfsT,
	mmAuxOfsX, mmAuxOfsZ, mmAuxOfsY, mmAuxLenV,
}

const hashCH = 0x9e3779b97f4a7c15 // close to 2**63*(sqrt(5)-1)

// doHash accumulates the hash over n*4 entries of
// mv into h. C inline do_hash.
func doHash(mv []uint64, n uint32, p int, mask1 uint64, h *[4]uint64) {
	maskP := uint64(p) * mask1
	maskPh := (maskP &^ mask1) >> 1
	var hh [4]uint64
	hh = *h
	mvi := 0
	for ; n > 0; n-- {
		for j := 0; j < 4; j++ {
			v := mv[mvi+j] & maskP
			v ^= (v >> 1) & maskPh
			w := (v & maskPh) + maskPh
			v &= w | maskPh
			hh[j] = (hh[j] >> 19) | (hh[j] << (64 - 19))
			hh[j] += (hh[j] << 11) + (hh[j] << 1) + v
		}
		mvi += 4
	}
	*h = hh
}

// hash returns a hash value of vector data modulo p.
// If bit i of skip is set, entries with the i-th tag
// in "ABCTXZY" are ignored. C mm_aux_hash.
//
// hash panics if p is not a supported modulus.
func hash(p int, data []uint64, skip int) uint64 {
	checkP(p)
	var h [4]uint64
	h[0] = hashCH + (uint64(p) << 4) + ((uint64(skip) & 0x7f) << 12)
	h[1] = h[0] * hashCH
	h[2] = h[1] * hashCH
	h[3] = h[2] * hashCH
	c := mmvConst(p)
	pb := uint((c >> 15) & 15)
	lif := uint(c & 7)
	mask1 := mmAuxTblReduce[2*pb-4]
	for i := 0; i < 7; i++ {
		if (skip>>i)&1 != 0 {
			continue
		}
		doHash(
			data[hashSections[i]>>lif:],
			(hashSections[i+1]-hashSections[i])>>(lif+2),
			p, mask1, &h,
		)
		if i == 3 && p == 3 {
			var buf [4]uint64
			buf[0] = data[(hashSections[4]>>5)-2]
			buf[1] = data[(hashSections[4]>>5)-1]
			buf[2] = 0
			buf[3] = 0
			doHash(buf[:], 1, 3, mask1, &h)
		}
	}
	return h[0] + h[1] + h[2] + h[3]
}

// mmAuxMulSparse multiplies the sparse vector a (mod
// p1) by scalar and reduces modulo p, storing the
// result in dst. It returns the result length or -1
// on failure. C mm_aux_mul_sparse (parameter order:
// p1=source modulus, scalar=f, p=target modulus).
func mmAuxMulSparse(p1 int, a []uint32, scalar, p int, dst []uint32) int {
	length := len(a)
	var aTbl, bad [256]uint8
	mask := uint32(4)
	isBad := uint8(0)
	if p1 < 3 || p1 > 255 || p1&1 == 0 ||
		p < 3 || p > 255 || p&1 == 0 {
		return -1
	}
	f := int64(scalar) % int64(p)
	for f < 0 {
		f += int64(p)
	}
	difficult := (f*int64(p1))%int64(p) != 0
	if f == 0 {
		const m = uint32(0xffffff00)
		for i := 0; i < length; i++ {
			dst[i] = a[i] & m
		}
		return 0
	}
	for mask < uint32(p1) {
		mask += mask
	}
	if mask > 256 {
		return -1
	}
	mask--
	if p1 == p && f == 1 {
		m := mask | 0xffffff00
		for i := 0; i < length; i++ {
			dst[i] = a[i] & m
		}
		return length
	}
	for i := uint32(0); i <= mask; i++ {
		aTbl[i] = uint8((int64(i) * f) % int64(p))
	}
	if !difficult {
		for i := 0; i < length; i++ {
			dst[i] = (a[i] & 0xffffff00) | uint32(aTbl[a[i]&mask])
		}
	} else {
		for i := uint32(0); i <= mask; i++ {
			bad[i] = uint8((int64(i) * f * int64(p1)) % int64(p))
		}
		for i := 0; i < length; i++ {
			dst[i] = (a[i] & 0xffffff00) | uint32(aTbl[a[i]&mask])
			isBad |= bad[a[i]&mask]
		}
	}
	if isBad != 0 {
		return -1
	}
	return length
}
