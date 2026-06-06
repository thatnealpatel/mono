package cgt

type XLeech2 struct {
	ord uint32
}

func NewXLeech2(value uint32) XLeech2 {
	panic("not implemented")
}

func (x XLeech2) Ord() uint32 {
	panic("not implemented")
}

func (x XLeech2) Type() uint32 {
	panic("not implemented")
}

// Subtype returns the packed subtype (same as gen_leech2_subtype). Python .subtype unpacks to a tuple.
func (x XLeech2) Subtype() uint32 {
	panic("not implemented")
}

func (x XLeech2) Bitvector() []byte {
	panic("not implemented")
}

func Leech2Scalprod(a, b uint32) uint32 {
	panic("not implemented")
}

func Leech2To3Short(x uint32) uint64 {
	panic("not implemented")
}

func Leech3To2Short(x uint64) uint32 {
	panic("not implemented")
}

func Leech2MatrixBasis(v2 []uint32) []uint64 {
	panic("not implemented")
}

func Leech2MatrixRadical(v2 []uint32) []uint64 {
	panic("not implemented")
}

func Leech3OpVectorWord(v3 uint64, g []uint32) uint64 {
	panic("not implemented")
}

func Leech2Pow(x uint32, e uint8) uint32 {
	panic("not implemented")
}

func Leech2OpAtom(x, g uint32) uint32 {
	panic("not implemented")
}
