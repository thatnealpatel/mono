package cgt

type XspAtom struct {
	Tag string
	I   int
}

const xsp2Co1Words = 26

type Xsp2Co1 struct {
	data [xsp2Co1Words]uint64
}

func NewXsp2Co1(atoms ...XspAtom) *Xsp2Co1 {
	panic("not implemented")
}

func Xsp2Co1Identity() *Xsp2Co1 {
	panic("not implemented")
}

func Xsp2FromXsp(x uint32) *Xsp2Co1 {
	panic("not implemented")
}

func (g *Xsp2Co1) AsXsp() uint32 {
	panic("not implemented")
}

func (g *Xsp2Co1) Order() int {
	panic("not implemented")
}

func (g *Xsp2Co1) Mul(h *Xsp2Co1) *Xsp2Co1 {
	panic("not implemented")
}

func (g *Xsp2Co1) Inv() *Xsp2Co1 {
	panic("not implemented")
}

func (g *Xsp2Co1) Pow(e int) *Xsp2Co1 {
	panic("not implemented")
}

func (g *Xsp2Co1) Equal(h *Xsp2Co1) bool {
	panic("not implemented")
}

func (g *Xsp2Co1) XspConjugate(v []uint32) []uint32 {
	panic("not implemented")
}

func (g *Xsp2Co1) Mmdata() []uint32 {
	panic("not implemented")
}

// Subtype returns 16*type + subtype as a packed value. Python .subtype unpacks to (type, subtype).
func (g *Xsp2Co1) Subtype() uint32 {
	panic("not implemented")
}

func (g *Xsp2Co1) TypeQx0() uint32 {
	panic("not implemented")
}

func (g *Xsp2Co1) HalfOrder() (int, *Xsp2Co1) {
	panic("not implemented")
}

func (g *Xsp2Co1) InGx0() bool {
	panic("not implemented")
}

func (g *Xsp2Co1) ChiGx0() [4]int {
	panic("not implemented")
}

func (g *Xsp2Co1) ConjugateInvolution() (int, *Xsp2Co1) {
	panic("not implemented")
}

func (g *Xsp2Co1) AsCo1Bitmatrix() []uint64 {
	panic("not implemented")
}

func Leech2OpWord(x uint32, g []uint32) uint32 {
	panic("not implemented")
}
