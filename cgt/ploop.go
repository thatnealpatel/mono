package cgt

type Parity int

func NewParity(v int) Parity { return Parity(v & 1) }

func (p Parity) Int() int { return int(p) & 1 }

func (p Parity) Sign() int { return 1 - 2*p.Int() }

func (p Parity) Equal(other Parity) bool { return p.Int() == other.Int() }

func (p Parity) Add(other Parity) Parity { return p ^ other }

type GCode struct {
	value uint16
}

func NewGCode(obj any) GCode { panic("not implemented") }

func (g GCode) Ord() uint16 { panic("not implemented") }

func (g GCode) Len() int { panic("not implemented") }

func (g GCode) Vector() uint32 { panic("not implemented") }

func (g GCode) BitList() []int { panic("not implemented") }

func (g GCode) Octad() int { panic("not implemented") }

func (g GCode) Add(other GCode) GCode { panic("not implemented") }

func (g GCode) And(other GCode) Cocode { panic("not implemented") }

func (g GCode) ScalarProd(c Cocode) Parity { panic("not implemented") }

func (g GCode) Theta() Cocode { panic("not implemented") }

func (g GCode) ThetaWith(other GCode) Parity { panic("not implemented") }

func (g GCode) Invert() GCode { panic("not implemented") }

func (g GCode) Apply(a *AutPL) GCode { panic("not implemented") }

type Cocode struct {
	value uint16
}

func NewCocode(obj any) Cocode { panic("not implemented") }

func (c Cocode) Ord() uint16 { panic("not implemented") }

func (c Cocode) Len() int { panic("not implemented") }

func (c Cocode) Parity() Parity { panic("not implemented") }

func (c Cocode) Add(other Cocode) Cocode { panic("not implemented") }

func (c Cocode) Syndrome(i int) uint32 { panic("not implemented") }

func (c Cocode) SyndromeList(i int) []int { panic("not implemented") }

func (c Cocode) AllSyndromes() []uint32 { panic("not implemented") }

func (c Cocode) Apply(a *AutPL) Cocode { panic("not implemented") }

type PLoop struct {
	value uint16
}

func NewPLoop(obj any) PLoop { panic("not implemented") }

func PLoopZ(e1, eo int) PLoop { panic("not implemented") }

func (p PLoop) Ord() uint16 { panic("not implemented") }

func (p PLoop) Sign() int { panic("not implemented") }

func (p PLoop) Len() int { panic("not implemented") }

func (p PLoop) GCode() GCode { panic("not implemented") }

func (p PLoop) Theta() Cocode { panic("not implemented") }

func (p PLoop) Mul(other PLoop) PLoop { panic("not implemented") }

func (p PLoop) Pow(e int) PLoop { panic("not implemented") }

func (p PLoop) Neg() PLoop { panic("not implemented") }

func (p PLoop) Invert() PLoop { panic("not implemented") }

func (p PLoop) Abs() PLoop { panic("not implemented") }

func (p PLoop) Cap(other PLoop) Cocode { panic("not implemented") }

func (p PLoop) Comm(other PLoop) int { panic("not implemented") }

func (p PLoop) Assoc(b, c PLoop) int { panic("not implemented") }

func (p PLoop) Split() (int, int, PLoop) { panic("not implemented") }

func (p PLoop) SplitOctad() (int, int, PLoop) { panic("not implemented") }

func (p PLoop) Apply(a *AutPL) PLoop { panic("not implemented") }

type Octad struct {
	value uint16
}

func NewOctad(o int) Octad { panic("not implemented") }

func (o Octad) Octad() int { panic("not implemented") }

func (o Octad) GCode() uint16 { panic("not implemented") }

type AutPL struct {
	cocode  uint16
	permNum uint32
	perm    []int
	rep     []uint32
}

func NewAutPL(d, p any) *AutPL { panic("not implemented") }

func (a *AutPL) Cocode() uint16 { panic("not implemented") }

func (a *AutPL) PermNum() uint32 { panic("not implemented") }

func (a *AutPL) Perm() []int { panic("not implemented") }

func (a *AutPL) Parity() Parity { panic("not implemented") }

func (a *AutPL) Mul(other *AutPL) *AutPL { panic("not implemented") }

func (a *AutPL) Inv() *AutPL { panic("not implemented") }

func (a *AutPL) Pow(e int) *AutPL { panic("not implemented") }

func (a *AutPL) Equal(other *AutPL) bool { panic("not implemented") }

func (a *AutPL) Check() *AutPL { panic("not implemented") }
