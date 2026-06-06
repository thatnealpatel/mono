package cgt

type P3Node struct {
	ord int
}

func NewP3Node(obj any) P3Node {
	panic("not implemented")
}

func (p P3Node) Ord() int {
	panic("not implemented")
}

func (p P3Node) Name() string {
	panic("not implemented")
}

func (p P3Node) Equal(other P3Node) bool {
	panic("not implemented")
}

func (p P3Node) Apply(g *AutP3) P3Node {
	panic("not implemented")
}

func P3Incidence(nodes ...any) P3Node {
	panic("not implemented")
}

func P3Incidences(nodes ...any) []P3Node {
	panic("not implemented")
}

func P3IsCollinear(points []int) bool {
	panic("not implemented")
}

func P3RemainingNodes(x1, x2 int) []P3Node {
	panic("not implemented")
}

func P3PointSetType(s []int) int {
	panic("not implemented")
}

type AutP3 struct {
	perm []int
}

func NewAutP3(mapping any) *AutP3 {
	panic("not implemented")
}

func NewAutP3Rand() *AutP3 {
	panic("not implemented")
}

func (g *AutP3) Order() int {
	panic("not implemented")
}

func (g *AutP3) Mul(other *AutP3) *AutP3 {
	panic("not implemented")
}

func (g *AutP3) Perm() []int {
	panic("not implemented")
}

func (g *AutP3) Inv() *AutP3 {
	panic("not implemented")
}

func (g *AutP3) Pow(e int) *AutP3 {
	panic("not implemented")
}

func (g *AutP3) Equal(other *AutP3) bool {
	panic("not implemented")
}

func (g *AutP3) PointMap() []int {
	panic("not implemented")
}

func (g *AutP3) LineMap() []int {
	panic("not implemented")
}

type BiMM struct {
	m1    *MM
	m2    *MM
	alpha int
}

func NewBiMM(m1, m2 *MM, e int) *BiMM {
	panic("not implemented")
}

func BiMMIdentity() *BiMM {
	panic("not implemented")
}

func (b *BiMM) Mul(other *BiMM) *BiMM {
	panic("not implemented")
}

func (b *BiMM) Pow(e int) *BiMM {
	panic("not implemented")
}

func (b *BiMM) Inv() *BiMM {
	panic("not implemented")
}

func (b *BiMM) Order() int {
	panic("not implemented")
}

func (b *BiMM) Orders() (int, int, int) {
	panic("not implemented")
}

func (b *BiMM) Equal(other *BiMM) bool {
	panic("not implemented")
}

func P3BiMM(word []int) *BiMM {
	panic("not implemented")
}

func AutP3BiMM(g *AutP3) *BiMM {
	panic("not implemented")
}

func BiMMCoxeterExp(x1, x2 int) int {
	panic("not implemented")
}

func (b *BiMM) Decompose() (*MM, *MM, int) {
	panic("not implemented")
}

func UFindInit(t []uint32) int {
	panic("not implemented")
}

func UFindUnion(t []uint32, i, j uint32) int {
	panic("not implemented")
}

func UFindFind(t []uint32, i uint32) uint32 {
	panic("not implemented")
}

func UFindFindAllMin(t []uint32) int {
	panic("not implemented")
}

func UFindPartition(t []uint32, data, ind []uint32) int {
	panic("not implemented")
}

func UFindMakeMap(t []uint32) []uint32 {
	panic("not implemented")
}

func BitParity(x uint64) int {
	panic("not implemented")
}

func BitWeight(x uint64) int {
	panic("not implemented")
}

func HadamardSign(i, j int) int {
	panic("not implemented")
}

func ParityHadamardSign(i, j int) int {
	panic("not implemented")
}

func HadamardTransform(p int, v []int) []int {
	panic("not implemented")
}

func XchParity(v []int) []int {
	panic("not implemented")
}

func ConjugateInvolutionType(g *MM) (int, *MM) {
	panic("not implemented")
}
