package cgt

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

type Tuple struct {
	Factor int
	Tag    Tag
	I0     int
	I1     int
}

type MMVector struct {
	p    int
	data []uint64
}

func Characteristics() []int {
	panic("not implemented")
}

func MMVSize(p int) int {
	panic("not implemented")
}

func MMV(p int) func(tag Tag, i0, i1 int) *MMVector {
	panic("not implemented")
}

func ZeroVector(p int) *MMVector {
	panic("not implemented")
}

func RandVector(p int) *MMVector {
	panic("not implemented")
}

func BasisVector(p int, tag Tag, i0, i1 int) *MMVector {
	panic("not implemented")
}

func NewVectorA(p, i0, i1 int) *MMVector {
	panic("not implemented")
}

func NewVectorJ(p, i0, i1 int) *MMVector {
	panic("not implemented")
}

func NewVector(p int, tuples []Tuple) *MMVector {
	panic("not implemented")
}

func FromBytes(p int, b []uint8) *MMVector {
	panic("not implemented")
}

func FromSparse(p int, sparse []uint32) *MMVector {
	panic("not implemented")
}

func ParseVector(p int, s string) (*MMVector, error) {
	panic("not implemented")
}

func Scalprod(v1, v2 *MMVector) int {
	panic("not implemented")
}

func ScalprodInd(p int, v1, v2 []uint64, ind []uint16) int {
	panic("not implemented")
}

func OpPi(p int, src []uint64, delta, pi int, dst []uint64) {
	panic("not implemented")
}

func OpXY(p int, src []uint64, f, e, eps int, dst []uint64) {
	panic("not implemented")
}

func OpOmega(p int, v []uint64, x int) {
	panic("not implemented")
}

func OpWord(p int, v []uint64, g []uint32, length, e int, work []uint64) error {
	panic("not implemented")
}

func OpWordTagA(p int, v []uint64, g []uint32, length, e int) error {
	panic("not implemented")
}

func OpWordABC(p int, src []uint64, g []uint32, length int, dst []uint64) error {
	panic("not implemented")
}

func OpTA(p int, src []uint64, e int, dst []uint64) {
	panic("not implemented")
}

func MulStdAxis(p int, v []uint64) {
	panic("not implemented")
}

func PrepareOpABC(g []uint32, length int, out []uint32) int {
	panic("not implemented")
}

func IndexExternToSparse(i int) uint32 {
	panic("not implemented")
}

func IndexSparseToExtern(sp uint32) int {
	panic("not implemented")
}

func IndexExternToIntern(i int) int {
	panic("not implemented")
}

func IndexSparseToIntern(sp uint32) int {
	panic("not implemented")
}

func IndexInternToSparse(i int) uint32 {
	panic("not implemented")
}

func IndexCheckIntern(i int) int {
	panic("not implemented")
}

func IndexSparseToLeech2(sp uint32) uint32 {
	panic("not implemented")
}

func IndexLeech2ToSparse(x uint32) uint32 {
	panic("not implemented")
}

func GetTableXi(stage, e, j, col int) uint32 {
	panic("not implemented")
}

func GetOffsetTableXi(stage, e, dir int) uint32 {
	panic("not implemented")
}

func SubTestPrepPi64(delta, pi int, out []uint32) {
	panic("not implemented")
}

func SubTestPrepXY(f, e, eps, mode int, out []uint32) {
	panic("not implemented")
}

func OpStoreAxis(p int, x uint32, dst []uint64) {
	panic("not implemented")
}

func OpNormA(p int, v []uint64) int {
	panic("not implemented")
}

func OpCheckzero(p int, v []uint64) bool {
	panic("not implemented")
}

func OpLoadLeech3Matrix(p int, v []uint64, a []uint64) {
	panic("not implemented")
}

func OpEvalARankMod3(p int, v []uint64) int {
	panic("not implemented")
}

func MmAuxMulSparse(p1 int, a []uint32, scalar, p int, dst []uint32) int {
	panic("not implemented")
}

func MmAuxExtractSparseSigns(p int, v []uint64, a []uint32) []uint32 {
	panic("not implemented")
}

func Hash(p int, data []uint64, skip int) uint64 {
	panic("not implemented")
}

func (v *MMVector) P() int {
	panic("not implemented")
}

func (v *MMVector) Data() []uint64 {
	panic("not implemented")
}

func (v *MMVector) Copy() *MMVector {
	panic("not implemented")
}

func (v *MMVector) Check() error {
	panic("not implemented")
}

func (v *MMVector) Add(other *MMVector) *MMVector {
	panic("not implemented")
}

func (v *MMVector) Sub(other *MMVector) *MMVector {
	panic("not implemented")
}

func (v *MMVector) MulScalar(a int) *MMVector {
	panic("not implemented")
}

func (v *MMVector) Mul(g []uint32) *MMVector {
	panic("not implemented")
}

func (v *MMVector) MulExp(g []uint32, e int, breakG bool) *MMVector {
	panic("not implemented")
}

func (v *MMVector) Equal(other *MMVector) bool {
	panic("not implemented")
}

func (v *MMVector) Hash() uint64 {
	panic("not implemented")
}

func (v *MMVector) AsBytes() []uint8 {
	panic("not implemented")
}

func (v *MMVector) AsSparse() []uint32 {
	panic("not implemented")
}

func (v *MMVector) AsTuples() []Tuple {
	panic("not implemented")
}

func (v *MMVector) At(tag Tag, i0, i1 int) int {
	panic("not implemented")
}

func (v *MMVector) Set(tag Tag, i0, i1, value int) {
	panic("not implemented")
}

func (v *MMVector) Entry(i int) int {
	panic("not implemented")
}

func (v *MMVector) Projection(tuples []Tuple) *MMVector {
	panic("not implemented")
}

func (v *MMVector) GetSparse(sparse []uint32) []uint32 {
	panic("not implemented")
}

func (v *MMVector) EvalA(v2 uint64, e int) int {
	panic("not implemented")
}

func (v *MMVector) CountShort() []int {
	panic("not implemented")
}

func (v *MMVector) AxisType(e int) string {
	panic("not implemented")
}
