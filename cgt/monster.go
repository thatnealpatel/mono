package cgt

type MM struct {
	data []uint32
}

func NewMM(word string) (*MM, error) { panic("not implemented") }

func MMIdentity() *MM { panic("not implemented") }

func MMGen(tag string, i int) *MM { panic("not implemented") }

type Subgroup string

const (
	SubM   Subgroup = "M"
	SubGx0 Subgroup = "G_x0"
	SubN0  Subgroup = "N_0"
	SubNx0 Subgroup = "N_x0"
	SubB   Subgroup = "B"
	SubQx0 Subgroup = "Q_x0"
)

func MMRand(rounds int) *MM { panic("not implemented") }

func MMRandIn(sub Subgroup) *MM { panic("not implemented") }

func MMFromInt(n uint64) *MM { panic("not implemented") }

func (g *MM) String() string { panic("not implemented") }

func (g *MM) Mmdata() []uint32 { panic("not implemented") }

func (g *MM) Mul(h *MM) *MM { panic("not implemented") }

func (g *MM) Inv() *MM { panic("not implemented") }

func (g *MM) Pow(e int) *MM { panic("not implemented") }

func (g *MM) Reduce() *MM { panic("not implemented") }

// Equal reduces both elements and
// compares the resulting []uint32.
// 
// This is likely to be slower, but
// easier to reason about in a first
// pass; if performance is bad, then
// consider  mm_group_words_equ in
// mmgroup (N_0 fast path) + ORDER_VECTOR.
func (g *MM) Equal(h *MM) bool { panic("not implemented") }

func (g *MM) Order() int { panic("not implemented") }

func (g *MM) HalfOrder() (int, *MM) { panic("not implemented") }

func (g *MM) InGx0() bool { panic("not implemented") }

func (g *MM) AsInt() uint64 { panic("not implemented") }

func (g *MM) InNx0() bool {
	panic("not implemented")
}

func (g *MM) InQx0() bool {
	panic("not implemented")
}

func (g *MM) ChiGx0() [4]int {
	panic("not implemented")
}

func (g *MM) IsReduced() bool {
	panic("not implemented")
}

func (g *MM) Simplify(ntrials int) *MM {
	panic("not implemented")
}

// ChiMap maps divisors e of the order to χ_M(g^e).
// Absent keys were not computed; use the ok idiom.
type ChiMap map[int]int

func (g *MM) ChiPowers(maxE, ntrials int) (int, ChiMap, *MM) {
	panic("not implemented")
}

type Axis struct {
	g *MM
	v *MMVector
}

func AxisFor(g *MM) *Axis { panic("not implemented") }

func (a *Axis) Type() string { panic("not implemented") }

func (a *Axis) Vector() *MMVector { panic("not implemented") }

func (a *Axis) Mul(g *MM) *Axis { panic("not implemented") }

// ReduceGx0 returns a G_x0 subgroup element, not an Axis. Python reduce_G_x0 returns G_x0.
func (a *Axis) ReduceGx0() *MM { panic("not implemented") }

func (a *Axis) Equal(b *Axis) bool { panic("not implemented") }
