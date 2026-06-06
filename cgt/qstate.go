package cgt

type QState struct {
	rows   int
	cols   int
	factor [2]int
	data   []uint64
}

func NewQState(rows, cols int, data []uint64, mode int) *QState {
	panic("not implemented")
}

func UnitMatrix(nqb int) *QState {
	panic("not implemented")
}

func RandMatrix(rows, cols, dataRows int) *QState {
	panic("not implemented")
}

func RandRealMatrix(rows, cols, dataRows int) *QState {
	panic("not implemented")
}

func PauliMatrix(nqb int, v uint64) *QState {
	panic("not implemented")
}

func CtrlNotMatrix(nqb int, vc, v uint64) *QState {
	panic("not implemented")
}

func PhiMatrix(nqb int, v uint64, phi int) *QState {
	panic("not implemented")
}

func CtrlPhiMatrix(nqb int, v1, v2 uint64) *QState {
	panic("not implemented")
}

func HadamardMatrix(nqb int, v uint64) *QState {
	panic("not implemented")
}

func ColumnMonomialMatrix(data []uint64) *QState {
	panic("not implemented")
}

func RowMonomialMatrix(data []uint64) *QState {
	panic("not implemented")
}

func FromSigns(bmap []uint64, n int) *QState {
	panic("not implemented")
}

func PauliVectorMul(nqb int, v1, v2 uint64) uint64 {
	panic("not implemented")
}

func PauliVectorExp(nqb int, v uint64, e int) uint64 {
	panic("not implemented")
}

func FlatProduct(a, b *QState, nqb, nc int) *QState {
	panic("not implemented")
}

func (q *QState) Copy() *QState {
	panic("not implemented")
}

func (q *QState) Shape() (int, int) {
	panic("not implemented")
}

func (q *QState) Factor() (int, int) {
	panic("not implemented")
}

func (q *QState) Data() []uint64 {
	panic("not implemented")
}

func (q *QState) NRows() int {
	panic("not implemented")
}

func (q *QState) NCols() int {
	panic("not implemented")
}

func (q *QState) Matrix() []complex128 {
	panic("not implemented")
}

func (q *QState) MulScalar(e, phi int) *QState {
	panic("not implemented")
}

func (q *QState) Reduce() *QState {
	panic("not implemented")
}

func (q *QState) Echelon() *QState {
	panic("not implemented")
}

func (q *QState) ReduceMatrix() []int {
	panic("not implemented")
}

func (q *QState) Reshape(rows, cols int) *QState {
	panic("not implemented")
}

func (q *QState) Transpose() *QState {
	panic("not implemented")
}

func (q *QState) Conjugate() *QState {
	panic("not implemented")
}

func (q *QState) T() *QState {
	panic("not implemented")
}

func (q *QState) H() *QState {
	panic("not implemented")
}

func (q *QState) RotBits(rot, nrot, start int) *QState {
	panic("not implemented")
}

func (q *QState) XchBits(sh int, mask uint64) *QState {
	panic("not implemented")
}

func (q *QState) Extend(j, nqb int) *QState {
	panic("not implemented")
}

func (q *QState) ExtendZero(j, nqb int) *QState {
	panic("not implemented")
}

func (q *QState) Restrict(j, nqb int) *QState {
	panic("not implemented")
}

func (q *QState) RestrictZero(j, nqb int) *QState {
	panic("not implemented")
}

func (q *QState) Sumup(j, nqb int) *QState {
	panic("not implemented")
}

func (q *QState) GateNot(v uint64) *QState {
	panic("not implemented")
}

func (q *QState) GateCtrlNot(vc, v uint64) *QState {
	panic("not implemented")
}

func (q *QState) GatePhi(v uint64, phi int) *QState {
	panic("not implemented")
}

func (q *QState) GateCtrlPhi(v1, v2 uint64) *QState {
	panic("not implemented")
}

func (q *QState) GateH(v uint64) *QState {
	panic("not implemented")
}

func (q *QState) ToSigns() []uint64 {
	panic("not implemented")
}

func (q *QState) CompareSigns(bmap []uint64) bool {
	panic("not implemented")
}

func (q *QState) LbRank() int {
	panic("not implemented")
}

func (q *QState) LbNorm2() int {
	panic("not implemented")
}

func (q *QState) Trace() complex128 {
	panic("not implemented")
}

func (q *QState) Inv() *QState {
	panic("not implemented")
}

func (q *QState) Power(e int) *QState {
	panic("not implemented")
}

func (q *QState) Order(maxOrder int) int {
	panic("not implemented")
}

func (q *QState) MatMul(other *QState) *QState {
	panic("not implemented")
}

func (q *QState) Mul(other *QState) *QState {
	panic("not implemented")
}

func (q *QState) Equal(other *QState) bool {
	panic("not implemented")
}

func (q *QState) PauliVector() uint64 {
	panic("not implemented")
}

func (q *QState) PauliConjugate(v []uint64, arg bool) []uint64 {
	panic("not implemented")
}

func (q *QState) ToSymplectic() []uint64 {
	panic("not implemented")
}
