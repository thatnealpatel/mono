# cgt package extraction plan

Regenerated against the current go.yaml (post B2 renames, D0b GtWord
routing, drop tables, H11 collapse, manual constructors) and the flat
`package cgt` source as of 2026-06-10.

Re-derive any counts with `grep '^- package:' _api/go.yaml` and
`ls *.go | grep -v '_test\|_gen'`.


## 1. Planned packages (go.yaml)

```
generator   — gen_leech2/gen_xi/gen_rng/ufind primitives, orbit machinery
leech       — XLeech2 type, Leech lattice mod-2/mod-3 ops, leech3matrix
mat24       — M24 permutations, Golay code, Parker loop, AutPL, Cocode, PLoop
mm          — MMVector, MM (monster element), BiMM, P3/AutP3, axis/order machinery
mmindex     — mm_index.c index-conversion layer (extern/intern/sparse/leech2 maps)
qstate12    — QState (quadratic state), qs12 internals, bm64 bit-matrix ops
reduce      — GtWord / gtSubword, word compression/expansion, reduction engine
swar        — bm64* bit-matrix primitives (exported), BitWeight/BitParity helpers
xsp2co1     — Xsp2Co1 (G_{x0} element), involution analysis, traces
```


## 2. Source file to package mapping

### mat24
- `mat24.go` — M24 permutation, Golay code, cocode, syndrome, octad, Parker loop
  algebra, AutPL
- `ploop.go` — Parity, GCode, Cocode, PLoop, Octad, AutPL types and arithmetic
- `mat24_gen.go` — precomputed tables (mat24EncTable, mat24DecTable,
  mat24SyndromeTable, mat24ThetaTable, mat24RecipBasis, mat24AutTable,
  mat24OctDecTable, etc.)

### generator
- `gen_xi.go` — XiGGray, XiGCocode, XiOpXi, XiLeechToShort, XiShortToLeech,
  Leech2Mul, Leech2Subtype, Leech2Type, Leech2Type2
- `rng.go` — Rng (xoshiro256**), NewRng, NewRngSeed, BytesModP, ModP
- `misc.go` (partial) — UFindInit, UFindUnion, UFindFind, UFindFindAllMin,
  UFindPartition, UFindMakeMap, BitParity, BitWeight, HadamardSign

### leech
- `leech.go` — XLeech2, NewXLeech2, NewXLeech2FromInt, XLeech2FromInt,
  NewXLeech2Copy, NewXLeech2FromPLoop, NewXLeech2Random, Leech2Scalprod,
  Leech2To3Short, Leech3To2Short, Short3Reduce, Leech2Pow, Leech2OpAtom,
  Leech2OpWord, GenLeech2OpWordMany, GenLeech2OpWordLeech2Many, VectToGcodeRaw,
  getLeech2Basis, Leech2MatrixBasis, Leech2MatrixRadical, Leech2MatrixOrthogonal,
  Leech3OpPi, Leech3OpY, Leech3OpXi, Leech3OpVectorWord
- `leech.go` (bm64 subset) — bm64RestoreCapH, bm64CapH (leech-specific composites
  built on the swar bm64 primitives)
- `xsp2_reduce.go` — GenLeech2StartType24, GenLeech2StartType4,
  GenLeech2ReduceType2, GenLeech2ReduceType4, GenLeech2ReduceType2Ortho,
  reduceType2Ortho, reduceType4, applyPerm, findOctadPermutation
  (all gen_leech_reduce.c provenance; routed here, not generator, because
  the reducers call the leech-level Leech2OpAtom)
- `leech_orbits.go` — Leech2OpWordMatrix24, Leech2OrbitsRaw,
  Leech2OrbitsResult, Leech2OrbitGen

The mm-coupled XLeech2 constructors (NewXLeech2RandomType, RandXleech2Type,
NewXLeech2FromShort, NewXLeech2FromMM, MMToQX0, NewXLeech2FromBasisVector,
NewXLeech2FromName) do NOT live here: they take/yield an *MM, parse a named
q-element, or index the mm-rep short-vector table, so they sit in flat cgt
(mm) in `xleech2_mm.go` and construct through the exported leech surface.

### swar
- `qstate.go` (bm64 block, lines ~348-580) — bm64RotBits, bm64XchBits,
  bm64ReverseBits, bm64T, bm64Mul, bm64MaskRows, bm64AddDiag, bm64EchelonH,
  bm64EchelonL, bm64Inv, bm64FindLowBit
- `misc.go` (partial) — BitParity, BitWeight (also claimed by generator;
  resolve by putting in swar with a re-export)

### qstate12
- `qstate.go` (bulk) — QState, qs12, all qs* functions, gate operations,
  qsComplex, qsToSigns, qsCompareSigns, scanAffine, fillAffine, qsPrepMul,
  qsProduct, qsMatmul, NewQState, UnitMatrix, and the full exported QState API

### mmindex
- `mmindex/mmindex.go` — the mm_index.c index-conversion layer (C
  mm_basics/mm_index.c): the OfsA../XofsA../SpaceTagA.. tag constants, the
  mmAuxTblABC table, the eight Index* converters (IndexExternToSparse,
  IndexSparseToExtern, IndexExternToIntern, IndexSparseToIntern,
  IndexInternToSparse, IndexCheckIntern, IndexSparseToLeech2,
  IndexLeech2ToSparse), the two intern<->leech2 converters
  (IndexLeech2ToInternFast, IndexInternToLeech2), and the mat24 inline
  wrappers. Imports only mat24; sits below both mm and reduce so the legal
  C downward edge (mm_reduce/mm_compress.c -> mm_index.c) no longer reads as
  mm-coupling inside reduce.

### mm
- `mm_op.go` — Tag, Tuple, MMVector layout, the mmvConst helpers, and the
  mm_aux.c vector surface (Characteristics, MMVSize, the reduce tables).
  The index-conversion slice moved to package mmindex (mm_op.go keeps an
  unexported mmAuxOfs/mmAuxXofs/mmSpaceTag alias block over mmindex's
  exported constants, plus a flat vectToCocode the generated
  mm_op_p_gen.go depends on)
- `mm_op_aux.go` — getMMV, putMMV, addMMV, readMMV24/32, writeMMV24/32,
  mmvToBytes, bytesToMMV, zeroMMV, reduceMMV, checkMMV, mmvToSparse,
  mmvExtractSparse, Hash, MmAuxExtractSparseSigns, MmAuxMulSparse
- `mm_op_axis.go` — MulStdAxis, OpStoreAxis
- `mm_op_eval.go` — OpNormA, CountShort, OpLoadLeech3Matrix, OpEvalARankMod3,
  EvalA, AxisType
- `mm_op_group.go` — opVectorAdd, opScalarMul, OpCheckzero, Scalprod,
  ScalprodInd, xi perm/sign tables, subPrepPi64, subPrepXY, OpOmega, OpPi,
  OpXY, OpWord, OpWordTagA, OpWordABC, PrepareOpABC
- `mm_op_t.go` — OpTA, evalA15, leech3matrixRank, leech3Echelon
- `mm_op_vector.go` — ZeroVector, MMV, NewVector, BasisVector, FromBytes,
  FromSparse, RandVector, Copy, Add, Sub, MulScalar, Hash, Equal, AsBytes,
  AsSparse, AsTuples, Mul, MulExp, Projection, ParseVector
- `mm_op_p_gen.go` — genSwar table, all genOp* field-generic SWAR operations
- `mm_op_xi_gen.go` — xi permutation/sign tables (generated)
- `gen_leech2.go` — genLeech2PrefixGx0, genLeech2MapStdSubframe, genExtractBC,
  genSubframeValue, genOpDeltaTagABC (routed here, not generator: genExtractBC,
  genSubframeValue and genOpDeltaTagABC take genSwar/[]uint64 mm-rep arguments
  and are consumed by mm_op_p_gen.go)
- `xleech2_mm.go` — the mm-coupled XLeech2 constructors split from leech.go:
  NewXLeech2RandomType, RandXleech2Type, NewXLeech2FromShort, NewXLeech2FromMM,
  MMToQX0, NewXLeech2FromBasisVector, NewXLeech2FromName (build via
  leech.NewXLeech2FromInt)
- `monster.go` — MM type, N0Elem, atom encoding, N_0 word algebra (nMul,
  nMulDeltaPi, nMulWordScan, nReduceElement, nToWord, etc.), parseMMWord,
  NewMM, MMIdentity, MMGen, MMRand, MMRandIn, MMFromInt, Mul/Inv/Pow/String
- `monster_compress.go` — GtWord compression/expansion primitives (insertInt256,
  extractInt256, mmCompressType4, mmCompress struct, mmCompressAsInt,
  mmExpandInt), gtWord/gtSubword reduction engine (reduceWord, ruleJoin,
  ruleTxiT, etc.)
- `monster_gx0.go` — G_{x0} membership via order vector (findInGx0, findInQx0,
  orderCheckInGx0, opWatermarkA)
- `monster_order.go` — order-vector reduction engine (loadOrderVector,
  genLeech3To2, reduce2AAxisType, analyzeAxis, reduceVAxis, mmReduceVectorVP,
  mmReduceVectorVm, mmReduceVectorV1, rebaseAxis), precomputed tables
- `monster_order_gen.go` — orderVectorTable, orderTagTable (generated)
- `monster_random.go` — mmRand, mmRandIn, subgroup flags, iterRandMM,
  appendTagsYXDP, randType4Vector
- `misc.go` (P3/AutP3 block) — P3Node, AutP3, NewP3Node, NewAutP3,
  P3Incidence, P3IsCollinear, P3PointSetType, p3 helpers
- `bimm.go` — BiMM, xsp2co1Co1GetMapping, xsp2co1Co1MatrixToWord,
  xsp2co1ElemFromMapping, P3 precomputation, autP3MM, Norton generators,
  NewBiMM, P3BiMM, AutP3BiMM

### reduce
- `monster_compress.go` (GtWord block) — gtSubword, gtWord, newGtWord,
  appendSubPart, reduceSub, ruleJoin, ruleTxiT, reduceInput, appendWord,
  reduce, toMmCompress, gtWordStore, compressError, mmCompressAsInt, mmExpandInt

  Note: monster_compress.go straddles mm and reduce. The low-level mmCompress*
  primitives (bit-field packing, PC init/add/expand) stay in mm. The gtWord
  reduction engine (linked-list, rule application, word store) moves to reduce.
  This is the resolved form of Clash A (see below).

### xsp2co1
- `xsp2.go` — Xsp2Co1 type, XspAtom, constructors (NewXsp2Co1, Xsp2Co1Identity,
  Xsp2FromXsp), G_{x0} element algebra (xsp2co1MulElem, xsp2co1InvElem,
  xsp2co1PowerElem, xsp2co1ReduceElem), Leech2OpWord, genLeech2OpWordMany,
  qs-to-elem conversions, symplectic row, Pauli vector, xi multiplication
- `xsp2_involution.go` — involution invariants, conjugation to standard form,
  involution class, traces (xsp2co1InvolutionInvariants,
  xsp2co1ElemConjugateInvolution, xsp2co1ElemInvolutionClass, trace98280Fast)
- `bimm.go` (xsp2co1 helpers) — xsp2co1Co1GetMapping, xsp2co1Co1MatrixToWord,
  chi244096, xsp2co1ElemFromMapping. These live in bimm.go but their C
  provenance is xsp2co1_map.c; they belong in xsp2co1 and bimm.go calls them.


## 3. Dependency graph

Edges are "A imports B". Only inter-package edges shown.

```
mat24       → (none — leaf)
swar        → (none — leaf)
generator   → mat24, swar
leech       → mat24, generator, swar
qstate12    → swar
xsp2co1     → mat24, generator, leech, qstate12, swar
mm          → mat24, generator, leech, qstate12, swar, xsp2co1
reduce      → mm, xsp2co1 (for mmCompress primitives and N0Elem)
```

Key dependency evidence:
- generator → mat24: gen_xi.go uses mat24ThetaTable, mat24SyndromeTable,
  mat24RecipBasis, mat24OctDecTable (all from mat24_gen.go)
- generator → swar: BitParity, BitWeight (re-export or direct use)
- leech → generator: Leech2Mul (gen_xi.go), Leech2OpAtom calls XiOpXi
- leech → swar: bm64EchelonH, bm64RotBits, bm64XchBits, bm64T, bm64EchelonL,
  bm64Mul, bm64ReverseBits in Leech2MatrixBasis/Leech2MatrixRadical
- qstate12 → swar: bm64FindLowBit, bm64EchelonH, bm64RotBits, bm64XchBits,
  bm64Mul, bm64T, bm64EchelonL, bm64Inv, bm64MaskRows, bm64AddDiag,
  bm64ReverseBits
- xsp2co1 → qstate12: qs12 type, qsPauliVector, qsReduce, qsEchelonize,
  qsMulAv, bm64 functions (via qstate.go definitions)
- xsp2co1 → leech: Leech2To3Short, Leech3To2Short, Short3Reduce,
  Leech3OpPi/Y/Xi, Leech2OpAtom, Leech2OpWord, GenLeech2ReduceType2/4,
  GenLeech2ReduceType2Ortho, Leech2MatrixOrthogonal, GenLeech2OpWordMany
  (GenLeech2Reduce* live in xsp2_reduce.go, now routed to leech, since the
  reducers call the leech-level Leech2OpAtom)
- xsp2co1 → generator: Leech2Subtype, Leech2Type2
- mm → xsp2co1: xsp2co1SetElemWordScan, xsp2co1ElemSubtype, xsp2co1ElemToWord,
  xsp2co1MulElemWord, xsp2co1ElemToN0 (in mm_op_group.go PrepareOpABC)
- mm → leech: Leech2MatrixBasis, Leech2MatrixRadical (monster_order.go)
- reduce → mm: N0Elem, nMulWordScan, nToWord, nReduceElement, mmCompress*
  primitives, IndexExternToSparse, IndexLeech2ToSparse
- reduce → xsp2co1: xsp2co1ElemToWord, xsp2co1SetElemWordScan, reduceWord
  calls xsp2co1 elem ops


## 4. Surviving clashes

### Clash A: mmCompress straddles reduce/mm
**Status: RESOLVED (Q-h).** The mmCompress* bit-field helpers (insertInt256,
extractInt256, mmCompressType4, mmCompressPCInit, mmCompressPCAddNx, etc.) stay
in mm because they operate on MMVector internals. The gtWord reduction engine
(gtSubword, gtWord linked-list, rule application) moves to reduce. The file
monster_compress.go must be split at the boundary; the mmCompress struct and its
methods stay in mm, and the gtWord struct and its methods go to reduce.

### Clash B: mm <-> xsp2co1 cycle on N0Elem
**Status: RESOLVED (Q-g).** Extract N0Elem and the N_0 word algebra (nMul,
nMulDeltaPi, nMulWordScan, nReduceElement, nToWord, nMulElement, etc.) into a
new `cgt/n0` package. Both mm and xsp2co1 import n0; the cycle breaks.

Note: n0 is not in go.yaml yet. It needs to be added. It is a small package
(the N0Elem type + ~20 functions from monster.go lines 59-480).

### Clash C: GtWord home
**Status: DISSOLVED (D0b).** GtWord routing is settled: the type and its
reduction engine live in reduce. No remaining ambiguity.

### Clash D: unexported mat24 helpers
**Status: RESOLVED (Q-j).** Export the helpers (lsbit24, synFromTable, vintern,
gcodeToVectInternal, oddSyn, gcodeToOctad, parity12). They are used by gen_xi.go
and mm_op_t.go, which will be in different packages. Exporting is the simplest
fix; the names already follow Go conventions once capitalized.

### Clash E: bm64* split across qstate.go, leech.go, xsp2.go, xsp2_involution.go
**Status: NEEDS CONSOLIDATION.** The bm64 primitives are defined in qstate.go
(lines ~348-580) and consumed by leech.go, xsp2.go, and xsp2_involution.go.
All primitive bm64 functions move to swar. The two composite helpers in leech.go
(bm64RestoreCapH, bm64CapH) stay in leech since they are leech-specific
compositions. Callers in xsp2co1 and qstate12 import swar.

### Clash F: genSwar vs swar
**Status: NO CLASH.** genSwar is the per-modulus SWAR table for MMVector
field-generic operations (mm_op_p_gen.go). It stays in mm. The swar package is
the bit-matrix primitive library (bm64*). The names do not collide: genSwar is
unexported and internal to mm.

### Clash G: qstate12 bare-name collisions
**Status: RESOLVED (Q-i).** The qs12 internal type and its ~50 free functions
(qsReduce, qsPivot, qsEchelonize, etc.) have bare names that would collide in a
flat namespace. Receiver disambiguation resolves this: qs12 becomes the
unexported working type inside qstate12, and the free functions keep their qs*
prefix (they are already unexported).

### Clash H: qs12 export
**Status: RESOLVED (Q-i).** The public API is QState (already exported). The
qs12 type stays unexported inside qstate12. xsp2co1 accesses qs12 through
package-internal helpers that qstate12 exports (e.g., a ToQS12/FromQS12 pair or
direct field access). This requires either:
  (a) xsp2co1 uses only the QState public API (preferred if feasible), or
  (b) qstate12 exports a thin internal-access API for xsp2co1.
Currently xsp2.go creates qs12 literals directly and calls ~15 unexported qs*
functions. Option (b) is needed; the thin API is the set of qs* functions that
xsp2.go calls.

### Clash I: name drift
**Status: DEFERRED (Q-e).** Renaming identifiers to match the go.yaml
conventions (e.g., camelCase C names to Go PascalCase) is a mechanical pass
done after extraction.


## 5. Extraction order

Based on the dependency DAG, extract leaf-first:

```
Phase 1 (leaves, parallel):
  mat24    — no inter-package deps
  swar     — no inter-package deps

Phase 2 (depends only on Phase 1):
  generator  → mat24, swar
  qstate12   → swar

Phase 3 (depends on Phase 1+2):
  leech      → mat24, generator, swar

Phase 4 (depends on Phase 1-3):
  n0         → mat24 (new package for N0Elem; extract from monster.go)
  xsp2co1    → mat24, generator, leech, qstate12, swar

Phase 5 (depends on everything):
  mm         → mat24, generator, leech, qstate12, swar, xsp2co1, n0
  reduce     → mm, xsp2co1, n0
```

Within each phase the packages are independent and can be extracted in
parallel. Phase 4 requires n0 to exist before xsp2co1 can be cleanly
extracted (Clash B resolution).

### Pre-extraction prerequisite

Before any phase: resolve Clash D (export mat24 helpers) and Clash E
(consolidate bm64 into swar). These are mechanical changes that unblock
all subsequent extractions.
