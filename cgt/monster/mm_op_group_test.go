package monster

import "testing"

// Test for the standalone monomial OpPi operation
// (mm_op_group.go), grounded in the canonical mmgroup
// tests
//   mmgroup/tests/test_mm_op/test_group_op.py
//   mmgroup/tests/test_mm_op/test_prep_pi_64.py
// which exercise the per-modulus mm_op_pi operation
// (x_delta * x_pi) on representation vectors.
//
// OpPi(p, src, delta, pi, dst) is checked directly here:
// the existing TestOpPiDeltaPerm exercises the same code
// path through MMVector.Mul, but the exported OpPi entry
// point itself was uncovered. Expected hashes were taken
// from mm_op.mm_op_pi over a seeded random vector via
// `goof mmgroup.py`; RandVectorSeed reproduces that input
// bit-for-bit.

// TestOpPi checks OpPi against the C reference for every
// supported modulus, over the identity (delta=pi=0), a
// pure diagonal (delta only), a pure permutation (pi
// only), and combined delta*pi atoms.
func TestOpPi(t *testing.T) {
	t.Parallel()
	type tc struct {
		p, seed   int
		delta, pi int
		want      uint64
	}
	cases := []tc{
		{3, 5, 0, 0, 16663755337054640364},
		{3, 5, 291, 0, 5794077451676875760},
		{3, 5, 0, 9999, 7591826269397346076},
		{3, 5, 2103, 217821225, 8085959444955897610},
		{3, 5, 1110, 12745645, 11898115995691648006},
		{7, 5, 0, 0, 479189915398588243},
		{7, 5, 291, 0, 13020637542263993573},
		{7, 5, 0, 9999, 15788922338695264488},
		{7, 5, 2103, 217821225, 12194915703083094312},
		{7, 5, 1110, 12745645, 12648149238398315962},
		{15, 5, 0, 0, 10707357825830228989},
		{15, 5, 291, 0, 17114433673551429014},
		{15, 5, 0, 9999, 3873671028674393878},
		{15, 5, 2103, 217821225, 15734302918546832276},
		{15, 5, 1110, 12745645, 6856304821277600009},
		{31, 5, 0, 0, 17970534251686109349},
		{31, 5, 291, 0, 12220952075424253527},
		{31, 5, 0, 9999, 9754415809754088833},
		{31, 5, 2103, 217821225, 4467144866623896581},
		{31, 5, 1110, 12745645, 10484690965225986574},
		{127, 5, 0, 0, 17304312899431657946},
		{127, 5, 291, 0, 538863687866169457},
		{127, 5, 0, 9999, 17798779770015815438},
		{127, 5, 2103, 217821225, 10429129829710686055},
		{127, 5, 1110, 12745645, 1319039752854192070},
		{255, 5, 0, 0, 6050511499128019532},
		{255, 5, 291, 0, 6292993926365204887},
		{255, 5, 0, 9999, 1776064988136446968},
		{255, 5, 2103, 217821225, 17766307853126749071},
		{255, 5, 1110, 12745645, 5061366777271198517},
	}
	for _, c := range cases {
		src := RandVectorSeed(c.p, uint64(c.seed))
		dst := ZeroVector(c.p)
		OpPi(c.p, src.Data(), c.delta, c.pi, dst.Data())
		if got := dst.Hash(); got != c.want {
			t.Errorf("OpPi(p=%d, seed=%d, delta=%#x, pi=%d) hash=%d want %d",
				c.p, c.seed, c.delta, c.pi, got, c.want)
		}
	}
}
