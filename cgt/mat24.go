package cgt

const Mat24Order = 244823040

func GcodeToVect(v uint32) uint32 {
	panic("not implemented")
}

func VectToGcode(v uint32) uint32 {
	panic("not implemented")
}

func Bw24(v uint32) uint32 {
	panic("not implemented")
}

func GcodeWeight(v uint32) uint32 {
	panic("not implemented")
}

func VectToBitList(v uint32) (int, []byte) {
	panic("not implemented")
}

func GcodeToBitList(v uint32) []byte {
	panic("not implemented")
}

func Lsbit24(v uint32) uint32 {
	panic("not implemented")
}

func ExtractB24(v, mask uint32) uint32 {
	panic("not implemented")
}

func SpreadB24(v, mask uint32) uint32 {
	panic("not implemented")
}

func VectToVintern(v uint32) uint32 {
	panic("not implemented")
}

func VinternToVect(v uint32) uint32 {
	panic("not implemented")
}

func VectToCocode(v uint32) uint32 {
	panic("not implemented")
}

func CocodeToVect(c uint32) uint32 {
	panic("not implemented")
}

func Syndrome(v, tetrad uint32) uint32 {
	panic("not implemented")
}

func CocodeSyndrome(c, tetrad uint32) uint32 {
	panic("not implemented")
}

func CocodeToBitList(c, tetrad uint32) []byte {
	panic("not implemented")
}

func CocodeToSextet(c uint32) []byte {
	panic("not implemented")
}

func AllSyndromes(c uint32) []uint32 {
	panic("not implemented")
}

func CocodeAllSyndromes(c uint32) []uint32 {
	panic("not implemented")
}

func CocodeWeight(c uint32) uint32 {
	panic("not implemented")
}

func VectType(v uint32) uint32 {
	panic("not implemented")
}

func GcodeToOctad(v uint32, strict uint8) uint32 {
	panic("not implemented")
}

func VectToOctad(v uint32, strict uint8) uint32 {
	panic("not implemented")
}

func OctadToGcode(octad uint32) uint32 {
	panic("not implemented")
}

func OctadToVect(octad uint32) uint32 {
	panic("not implemented")
}

func CocodeToSuboctad(c, v, strict uint32) uint32 {
	panic("not implemented")
}

func SuboctadToCocode(sub, octad uint32) uint32 {
	panic("not implemented")
}

func SuboctadWeight(sub uint32) uint32 {
	panic("not implemented")
}

func SuboctadScalarProd(sub1, sub2 uint32) uint32 {
	panic("not implemented")
}

func ScalarProd(v, c uint32) uint32 {
	panic("not implemented")
}

func IntersectOctadTetrad(v1, v2 uint32) uint32 {
	panic("not implemented")
}

func CocodeAsSubdodecad(c, v, single uint32) uint32 {
	panic("not implemented")
}

func PloopTheta(v uint32) uint32 {
	panic("not implemented")
}

func PloopCocycle(v1, v2 uint32) uint32 {
	panic("not implemented")
}

func MulPloop(v1, v2 uint32) uint32 {
	panic("not implemented")
}

func PowPloop(v, exp uint32) uint32 {
	panic("not implemented")
}

func PloopComm(v1, v2 uint32) uint32 {
	panic("not implemented")
}

func PloopCap(v1, v2 uint32) uint32 {
	panic("not implemented")
}

func PloopAssoc(v1, v2, v3 uint32) uint32 {
	panic("not implemented")
}

func PloopSolve(a []uint32) uint32 {
	panic("not implemented")
}

func M24numToPerm(num uint32) []byte {
	panic("not implemented")
}

func PermToM24num(p []byte) uint32 {
	panic("not implemented")
}

func PermCheck(p []byte) error {
	panic("not implemented")
}

func PermCompleteHeptad(p []byte) ([]byte, error) {
	panic("not implemented")
}

func PermCompleteOctad(p []byte) ([]byte, error) {
	panic("not implemented")
}

func PermFromHeptads(h1, h2 []byte) []byte {
	panic("not implemented")
}

func PermFromDodecads(d1, d2 []byte) []byte {
	panic("not implemented")
}

// PermFromMap returns the completion type (1=short, 2=heptad, 3=full) and the completed permutation.
func PermFromMap(h1, h2 []byte) (int, []byte, error) {
	panic("not implemented")
}

func PermToMatrix(p []byte) []uint32 {
	panic("not implemented")
}

func MatrixToPerm(m []uint32) []byte {
	panic("not implemented")
}

func OpVectPerm(v uint32, p []byte) uint32 {
	panic("not implemented")
}

func OpGcodeMatrix(v uint32, m []uint32) uint32 {
	panic("not implemented")
}

func OpGcodePerm(v uint32, p []byte) uint32 {
	panic("not implemented")
}

func OpCocodePerm(c uint32, p []byte) uint32 {
	panic("not implemented")
}

func MulPerm(p1, p2 []byte) []byte {
	panic("not implemented")
}

func InvPerm(p []byte) []byte {
	panic("not implemented")
}

func PermToAutpl(c uint32, p []byte) []uint32 {
	panic("not implemented")
}

func CocodeToAutpl(c uint32) []uint32 {
	panic("not implemented")
}

func AutplToPerm(m []uint32) []byte {
	panic("not implemented")
}

func AutplToCocode(m []uint32) uint32 {
	panic("not implemented")
}

func OpPloopAutpl(v uint32, m []uint32) uint32 {
	panic("not implemented")
}

func MulAutpl(m1, m2 []uint32) []uint32 {
	panic("not implemented")
}

func InvAutpl(m []uint32) []uint32 {
	panic("not implemented")
}

func PermToIautpl(c uint32, p []byte) ([]byte, []uint32) {
	panic("not implemented")
}

func OpAllAutpl(m []uint32) []uint16 {
	panic("not implemented")
}

func OpAllCocode(c uint32) []byte {
	panic("not implemented")
}

func CompleteRandMode(mode uint32) uint32 {
	panic("not implemented")
}

func PermInLocal(p []byte) uint32 {
	panic("not implemented")
}

func PermRandLocal(mode, rand uint32) []byte {
	panic("not implemented")
}

func M24numRandLocal(mode, rand uint32) int32 {
	panic("not implemented")
}

func VectToList(v uint32, maxLen int) []byte {
	panic("not implemented")
}

func OctadEntries(octad uint32) [8]uint8 {
	panic("not implemented")
}

func PermToNet(p []byte) [9]uint32 {
	panic("not implemented")
}

func MatrixFromModOmega(m []uint32) {
	panic("not implemented")
}

func M24numRandAdjustXY(mode, v uint32) uint32 {
	panic("not implemented")
}
