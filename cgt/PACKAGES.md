# Package Migration Plan

Migration of the flat `package cgt` (`github.com/nealpatel/mono/cgt`) into the
8 idiomatic packages crystallized in `cgt/_api/go.yaml` (`gen`, `gt`, `leech`,
`mat24`, `mm`, `qstate12`, `swar`, `xsp2co1`).

## Status of the inputs

- **`_api/go.yaml` is a translation _plan_, not a description of the current
  code.** It was auto-generated from `cython.yaml` + `python.yaml`
  (`go.yaml:1`) and enumerates **1711** function entries (917 of them methods
  via `recv:`) plus 5 constants. Many entries are **planned but unimplemented**
  (see [Open Questions](#open-questions)).
- The flat package today is **30 non-test, non-`_gen` `.go` files** plus 4
  generated files (`mat24_gen.go`, `mm_op_xi_gen.go`, `mm_op_p_gen.go`,
  `monster_order_gen.go`) and test files (ignored here per task).
- **The plan's parameter/return types use placeholder C-struct names** that do
  **not** match the Go type names already in the code. This is the single
  biggest source of confusion when reading `go.yaml` against the source:

  | `go.yaml` type string | Actual Go type (file)                    | Exported? |
  |-----------------------|------------------------------------------|-----------|
  | `*qstate12Type`       | `qs12` (`qstate.go:68`)                   | **no**    |
  | `*qstate12SupportType`| `qsSupport` (`qstate.go:1078`)           | **no**    |
  | `*gtWordType`         | `gtWord` (`monster_compress.go:521`)     | **no**    |
  | `*gtSubwordType`      | `gtSubword` (`monster_compress.go:496`)  | **no**    |
  | `*mmCompressType`     | `mmCompress` (`monster_compress.go:185`) | **no**    |

  Every cross-package type referenced by the plan is currently an **unexported
  struct**. Unexported identifiers are package-scoped in Go, so each of these
  becomes a migration hazard the moment it must cross a package boundary.

---

## Target Layout

`go.yaml` package section line numbers: `gen` (15), `gt` (1414), `leech`
(1588), `mat24` (1996), `mm` (3542), `qstate12` (9294), `swar` (10585),
`xsp2co1` (11133), constants (12197).

| Package    | Dir (proposed)   | Primary exported types (planned)                                                                 | Flat source files that move there |
|------------|------------------|--------------------------------------------------------------------------------------------------|-----------------------------------|
| `gen`      | `cgt/gen/`       | `OrbitElem2`, `OrbitLin2`, `RandomSubgroup` (all **unimplemented**); plus the `Rng` machinery     | `gen_xi.go`†, `rng.go`†, parts of `misc.go` (UFind*) |
| `gt`       | `cgt/gt/`        | `GtWord`, `GtSubWord` (**unimplemented**); wraps `gtWord`/`gtSubword`/`mmCompress`                | `monster_compress.go`‡ |
| `leech`    | `cgt/leech/`     | `XLeech2` (`leech.go:8`)                                                                          | `leech.go`, `gen_leech2.go`‡ |
| `mat24`    | `cgt/mat24/`     | `GCode`,`Cocode`,`PLoop`,`PLoopIntersection`,`Parity`,`AutPL`,`AutPlGroup`,`GcVector`,`Octad`     | `mat24.go`, `ploop.go`, `mat24_gen.go` |
| `mm`       | `cgt/mm/`        | `MM`,`MMVector`,`Axis`,`BiMM`,`AutP3`,`P3Node`,`Tag`,`Tuple`,`Subgroup`,`N0Elem`, +many planned   | `monster*.go`, `mm_op*.go`, `bimm.go`, `misc.go`†, `mm_op_xi_gen.go`, `mm_op_p_gen.go`, `monster_order_gen.go` |
| `qstate12` | `cgt/qstate12/`  | `QState12` + `QStateMatrix` (plan) — **code has one type `QState`** (`qstate.go:42`) over `qs12`   | `qstate.go` |
| `swar`     | `cgt/swar/`      | none (free functions only): `bitmatrix64_*`, `bitvector*`, `uint64_*` utils                       | **no single file** — code is the unexported `bm64*` helpers, currently split across `leech.go` + `qstate.go` |
| `xsp2co1`  | `cgt/xsp2co1/`   | `Xsp2Co1` (`xsp2.go:35`), `Xsp2Co1Group` (**unimplemented**)                                       | `xsp2.go`, `xsp2_involution.go`, `xsp2_reduce.go` |

† **File splits across packages** — these files do not map 1:1 to a package:
- `misc.go` holds `P3Node`/`AutP3` (→ `mm`) **and** the `UFind*` family
  (`misc.go:917-1193`, → `gen`) **and** bit/Hadamard helpers. Must be split.
- `gen_xi.go` holds `Xi*` (plan → `gen`) **and** `Leech2Mul`/`Leech2Type*`
  (`gen_xi.go:257-447`, plan → `gen` too, but semantically Leech). It is
  internally consistent with `gen` but the file name is misleading.
- `rng.go` defines `Rng` used by `mm_op_vector.go` (→ `mm`); see cross-deps.

‡ **Misleadingly named files:**
- `gen_leech2.go` declares `genSubframeValue(s *genSwar, …)`,
  `genExtractBC`, `genOpDeltaTagABC` (`gen_leech2.go:131-196`) — these are
  **`mm` modular-arithmetic** routines, not `gen`/`leech`. Routes to `mm`.
- `monster_compress.go` is the home of the `gt`-package types
  (`gtWord`/`gtSubword`/`mmCompress`), so it routes to `gt`, **not** `mm`,
  despite the `monster_` prefix.

### `_gen.go` routing

| Generated file          | Target pkg | Evidence | Matches plan? |
|-------------------------|------------|----------|---------------|
| `mat24_gen.go`          | `mat24`    | `mat24*Table` data vars (`mat24_gen.go`)                 | yes |
| `mm_op_xi_gen.go`       | `mm`       | `xiSign*`/`xiPerm*` ξ-operation tables                   | yes |
| `mm_op_p_gen.go`        | `mm`       | `genSwar` type + `genOp*`/`genScalprod*`; calls `logIntFields` (`mm_op.go:121`) | yes — **but** note `genSwar` is **not** the planned `swar` package (see clash F) |
| `monster_order_gen.go`  | `mm`       | `orderVectorTable`/`orderTagTable`                       | yes |

All generated files are regenerated via `//go:generate` directives in
`generate.go` (`generate.go:16-22`); per `CLAUDE.md` they must never be
hand-edited, and the generators (`_gen`, `_api`) must emit into the new package
directories after the split.

---

## Dependency Graph

Directional edges (`A → B` = "A imports B"), derived from actual type/function
references in the flat code (not just the plan):

```
            ┌────────────────────────── swar (leaf: bm64*/bitvector/uint64 utils)
            │                              ▲   ▲   ▲
            │                              │   │   │
   gen ◄────┤                         leech│   │qstate12
    ▲       │                              │   │   │
    │       │                              │   │   │
  mat24 ◄───┴── leech ◄── xsp2co1 ──► qstate12
    ▲             ▲           ▲ │
    │             │           │ │
    └──── mm ─────┴───────────┘ │
          ▲ │                   │
          │ └───────────────────┘   (mm ⇄ xsp2co1 — SEE CYCLE)
          │
       gt (→ mm for mmCompress, OR mm → gt — SEE CYCLE)
```

Edges with evidence:

- **`swar` is a leaf** (no outgoing edges). Everything that does bit-matrix
  work depends on it: `leech`, `qstate12`, `xsp2co1` all call `bm64*`.
- `mat24 → swar?` — `mat24.go` does not currently call `bm64*`; `mat24` is
  effectively a second near-leaf (depends on `mat24_gen.go` data only). It is
  referenced by almost everyone.
- `leech → mat24` (unexported helpers `gcodeToVectInternal`, `parity12`,
  `cocodeSyndrome`, `synFromTable`, `vintern` — see clash D), `leech → swar`
  (`bm64EchelonH`/`bm64EchelonL`), `leech → gen` (`gen_leech2.go`).
- `qstate12 → swar` (11 of the `bm64*` defs **and** uses live in `qstate.go`).
- `xsp2co1 → qstate12` (7 functions take `*qs12`: `ChainShort3`, `ElemToQs`,
  `ElemToQsI`, `MapShort3`, `MulQsV3Word`, `QsToElemI`, `RepMod3FromQs`, plus
  the `qs*` helper family in `xsp2.go`), `xsp2co1 → swar`, `xsp2co1 → mat24`,
  `xsp2co1 → leech`.
- `mm → leech` (uses `XLeech2`, 10×, e.g. `bimm.go:285`), `mm → xsp2co1`
  (uses `Xsp2Co1` / `NewXsp2Co1`, e.g. `monster.go:1299`), `mm → mat24`,
  `mm → gen` (`Rng`, UFind).
- `gt → mm` **or** `mm → gt` via `mmCompress` (clash A).

### Cycles (hard blockers)

1. **`mm ⇄ xsp2co1`** (on unexported symbols). See clash B. **Confirmed.**
2. **`gt ⇄ mm`** (on `mmCompress`). See clash A. **Risk, direction-dependent.**

---

## Clashes

### A. `mmCompress` / `mmCompressType` straddles `gt` and `mm`

- The plan references `*mmCompressType` in package **`gt`** (1×) **and** package
  **`mm`** (3×).
  - `gt`: `WordToMmCompress(pGt *gtWordType, pC *mmCompressType)`
    (`go.yaml:1579-1586`).
  - `mm`: `CompressPcAddNx`, `CompressPcAddType2`, `CompressPcInit`
    (`go.yaml:3978-4011`) all take `*mmCompressType`.
- Actual type `mmCompress` is declared at `monster_compress.go:185`, in the
  same file as the `gt` types `gtWord`/`gtSubword`. A single Go type can live
  in **one** package. If `mmCompress` → `gt`, then `mm` imports `gt`; if
  `mmCompress` → `mm`, then `gt` imports `mm`. Either way the type is currently
  **unexported**, so it cannot cross the boundary without being exported.
- **Hazard:** combined with the `GtWord` receiver-home conflict (clash C) and
  `mm → gt`/`gt → mm` references, this risks a `gt ⇄ mm` cycle.

### B. `mm ⇄ xsp2co1` import cycle on the N_0 word algebra (unexported)

- `xsp2_involution.go` (→ `xsp2co1`) directly uses the N_0 machinery declared
  in `monster.go` (→ `mm`):
  - casts `(*N0Elem)(g)` (`xsp2_involution.go:25,38`) — `N0Elem` is
    `monster.go:59`.
  - calls unexported `nMulWordScan` (`xsp2_involution.go:27`) and
    `nReduceElement` (`xsp2_involution.go:38`) — both `monster.go`.
  - reuses the `iT..iPi` index constants — `monster.go`.
  - The file's own header documents this reuse explicitly:
    `xsp2_involution.go:9-13`.
- `monster.go` (→ `mm`) calls `NewXsp2Co1` (→ `xsp2co1`) at
  `monster.go:1299, 1338, 1435, 1476`.
- **Result:** a hard bidirectional dependency on **unexported** functions
  (`nMulWordScan`, `nReduceElement`) and an exported-named-but-internal type
  (`N0Elem`). The plan papers over this by typing `ElemFromN0`/`ElemToN0` as
  `g []uint32` (`go.yaml:11265, 11332`), hiding `N0Elem` at the API surface,
  but the **implementation** still needs the `nMul*` family on both sides.
- **This is the primary structural blocker for the migration.**

### C. Receiver-home conflict: `GtWord` / `GtSubWord` methods in `mm`, free functions in `gt`

- The plan assigns the `gtWord` **C-style free functions** to package **`gt`**:
  `WordAlloc`, `WordReduce`, `WordCompress`, … (`go.yaml:1421-1586`), all
  taking `*gtWordType`.
- The plan assigns the `gtWord` **methods** (from Python `GtWord.*`) to package
  **`mm`**: `Append`, `Reduce`, `Mmdata`, `Subwords`, … (`go.yaml:7566-7645`,
  `recv: GtWord`), and `GtSubWord` methods (`go.yaml:7550-7565`, `recv:
  GtSubWord`).
- In Go, methods must be declared in the same package as their receiver type.
  `GtWord` cannot have methods in `mm` while being declared in `gt`. **One
  package must own `GtWord`/`GtSubWord`** and hold both the free functions and
  the methods.
- Same shape applies to `EvalNodeVisitor`, `TaggedAtom`, `AtomDict`, `Vsparse`,
  and the `Abstract*` group/space hierarchy — all placed in `mm` with `recv:`,
  which is internally consistent *within* `mm`, so those are fine; only
  `GtWord`/`GtSubWord` cross the `gt`/`mm` line.

### D. Unexported `mat24` helpers called across boundaries

`mat24`'s lowercase helpers (declared in `mat24.go`) are called from files that
route to **other** packages. Each must be **exported** (most have an exported
twin already, e.g. `cocodeSyndrome`→`CocodeSyndrome`), or the callers must be
rewritten to call the exported form:

| Unexported `mat24` helper | Called from (→ pkg)                                  | Count |
|---------------------------|------------------------------------------------------|-------|
| `parity12`                | `monster*.go`/`mm_op*.go` (→ mm); `leech.go`,`xsp2*.go` | 9 + 11 |
| `vintern`                 | `monster*.go`/`mm_op*.go` (→ mm); `leech.go`         | 9 + 1 |
| `cocodeSyndrome`          | `mm`; `leech.go`,`xsp2*.go`                          | 8 + 2 |
| `synFromTable`            | `mm`; `leech.go`,`xsp2*.go`                          | 2 + 2 |
| `gcodeToVectInternal`     | `leech.go`,`xsp2*.go` (→ leech/xsp2co1)              | 7 |
| `cocodeToSuboctad`        | `mm`                                                 | 2 |
| `suboctadWeight`          | `mm`                                                 | 2 |
| `permCheck`               | `mm`                                                 | 1 |

### E. `swar` primitives (`bm64*`) are unexported **and** split across two files

- The planned `swar` package corresponds to the C `bitmatrix64_*` family,
  implemented in the flat code as unexported `bm64*` functions.
- **Defined in two files that route to different packages:**
  - `leech.go`: `bm64RestoreCapH` (`leech.go:177`), `bm64CapH`
    (`leech.go:207`).
  - `qstate.go`: 11 defs — `bm64RotBits`, `bm64XchBits`, `bm64ReverseBits`,
    `bm64T`, `bm64Mul`, `bm64MaskRows`, `bm64AddDiag`, `bm64EchelonH`,
    `bm64EchelonL`, `bm64Inv`, `bm64FindLowBit` (`qstate.go:350-549`).
- **Used in 4 files spanning 3 prospective packages:** `leech.go` (21×, leech),
  `qstate.go` (43×, qstate12), `xsp2.go` (14×) + `xsp2_involution.go` (16×,
  xsp2co1).
- **Hazard:** these must be consolidated into the new `swar` package and
  **exported**, then `leech`/`qstate12`/`xsp2co1` rewritten to call
  `swar.EchelonH` etc. Until consolidation, the defs live in packages that the
  callers cannot reach.

### F. Two distinct "swar" concepts — name overload

- The **planned `swar` package** = bit utilities (`bitmatrix64_*`,
  `bitvector*`, `uint64_*`; `go.yaml:10585+`).
- The flat code **also** has an internal `genSwar` type +
  `genSwarFor`/`genSwarTable`/`logIntFields` in `mm_op_p_gen.go` — this is the
  **modular-reduction SWAR** for the `mm` representation (depends on
  `logIntFields`/`mmvConst` from `mm_op.go:121`), and is **unrelated** to the
  planned `swar` package.
- **Hazard:** do not route `mm_op_p_gen.go` / `genSwar` into `swar`. They stay
  in `mm`. The name collision is cosmetic but easy to get wrong.

### G. `qstate12` free-function name collisions (plan vs. existing code)

The plan emits **low-level C wrappers** under bare names while the existing flat
code already uses those bare names for **high-level constructors**, in the same
target package `qstate12`. Two different signatures under one Go name in one
package **will not compile**:

| Go name (pkg `qstate12`) | Plan (C wrapper, mutate-in-place)                         | Existing flat code (constructor)              |
|--------------------------|-----------------------------------------------------------|-----------------------------------------------|
| `UnitMatrix`             | `UnitMatrix(*qstate12Type, nqb) int32` (`go.yaml:9934`)   | `UnitMatrix(nqb) *QState` (`qstate.go:2189`)  |
| `PauliMatrix`            | `PauliMatrix(*qstate12Type, …) int32` (`go.yaml:9685`)    | `PauliMatrix(nqb, v) *QState` (`qstate.go:2222`) |
| `FromSigns`              | `FromSigns([]uint64, n) int32` (`go.yaml:9446`)           | `FromSigns(bmap, n) *QState` (`qstate.go:2286`) |
| `ColumnMonomialMatrix`   | py `qstate12_column_monomial_matrix` (`go.yaml:10003`)    | `ColumnMonomialMatrix(data) *QState` (`qstate.go:2262`) |
| `RowMonomialMatrix`      | py `qstate12_row_monomial_matrix` (`go.yaml:10121`)       | `RowMonomialMatrix(data) *QState` (`qstate.go:2272`) |

- `PauliVectorMul`/`PauliVectorExp`/`FlatProduct` also appear in both, but with
  **matching** signatures (`go.yaml:9706-9727`, `qstate.go:2300-2313`) — not
  collisions, just duplicate listings.
- The plan provides `Qs`-prefixed alternates (`QsUnitMatrix`, `QsFromSigns`,
  `QsPauliMatrix`, `QsRowMonomialMatrix`, `QsColumnMonomialMatrix`;
  `go.yaml:10035-10120`) — evidence the plan's intended convention is "bare name
  = C wrapper, `Qs`-prefix = high-level". The existing code chose the opposite.
  **This is a naming-convention decision a human must make.**

### H. `QState12` vs `QStateMatrix` — two planned receivers, one implemented type

- The plan declares **two** receiver types in `qstate12`: `QState12` (62
  methods, the low-level state) and `QStateMatrix` (the high-level matrix
  wrapper). Both having identically-named methods (`Abs`, `Copy`, `Reduce`,
  `ToSigns`, `Transpose`, …) is **legal** in Go — methods are namespaced by
  receiver, so this is **not** a collision.
- The flat code implements this split with different names: `qs12` (unexported,
  mutable, "mirrors `qstate12_type`") and `QState` (exported wrapper), per the
  doc comment at `qstate.go:5-9` / `qstate.go:42`.
- **Not a clash, but a divergence:** the plan wants the low-level type
  **exported** as `QState12`; the code keeps it unexported as `qs12`. If
  `xsp2co1` is to consume `qs12` (clash B/cross-deps), `qs12`/`QState12`
  **must be exported**. See Open Questions.

### I. Name drift between plan and code (low severity, pervasive)

The plan was generated from Python/C names; the code already hand-adjusted many.
Examples (representative, not exhaustive):

- `XiOpXiNosign` (plan, `go.yaml:942`) vs `XiOpXiNoSign` (`gen_xi.go:95`).
- `Leech2To3Short` (`leech.go:69`) vs plan `Leech2to3Short` (`go.yaml:348`).
- `M24numRandAdjustXY` (`mat24.go:2345`) vs plan `M24numRandAdjustXy`
  (`go.yaml:2202`).

These are not compile blockers (each lands in one package) but mean the
generator output will **not** match existing exported names — a reconciliation
pass is required so the public API does not churn.

---

## Migration Order

Extraction DAG (leaves first; a package may be extracted once all its
dependencies are already extracted **and** every identifier it needs from them
is exported):

```
Wave 0 (leaves, zero cross-deps once helpers are gathered):
   swar     ← consolidate bm64* from leech.go+qstate.go, export them (clash E)
   mat24    ← export the lowercase helpers used elsewhere (clash D); mat24_gen.go

Wave 1 (depend only on Wave 0):
   gen      ← gen_xi.go, rng.go, UFind* from misc.go; depends on mat24
   leech    ← leech.go (+gen_leech2.go's leech parts); depends on swar,mat24,gen

Wave 2 (depend on Wave 0–1):
   qstate12 ← qstate.go; depends on swar. Resolve clash G (naming) + H (export qs12) first.

Wave 3 (the tangled core — cannot be cleanly ordered as-is):
   mm  ⇄  xsp2co1     ← BLOCKED by cycle (clash B)
   gt  ?  mm          ← BLOCKED by mmCompress direction (clash A)
```

- **Waves 0–2 are mechanically extractable** once the export/consolidation
  prerequisites (clashes D, E, G, H) are done. They form a clean DAG:
  `swar` and `mat24` first, then `gen`, then `leech`, then `qstate12`.
- **Wave 3 cannot be split without first breaking cycles B and A.** Options:
  1. **Break B** by extracting the N_0 word algebra (`N0Elem`, `nMul*`,
     `iT..iPi`) into a small shared lower package (e.g. `cgt/mmn0` or fold into
     `gen`) that both `mm` and `xsp2co1` import. Then `mm → xsp2co1` only
     (one direction), removing the cycle.
  2. **Break A** by choosing `mmCompress`'s owner: put it in `mm` and let `gt`
     import `mm` (natural, since `gt` words *contain* `mm` compress data), or
     vice-versa. Export `mmCompress` either way.
  3. If neither cut is acceptable, `mm` + `xsp2co1` (and possibly `gt`) ship as
     **one package**, contradicting the 8-package plan for that cluster.

---

## Open Questions

These require a human decision before (or during) migration:

1. **Cycle B resolution (highest priority).** Where does the N_0 word algebra
   live? Extract `N0Elem`/`nMul*`/`iT..iPi` to a shared sub-package, or accept
   `mm`+`xsp2co1` as one package? The plan's `[]uint32` API typing
   (`ElemFromN0`) only hides the boundary at the surface; the implementation
   coupling is real.

2. **Cycle/edge A direction.** Does `mmCompress` belong to `gt` or `mm`? This
   decides whether `mm → gt` or `gt → mm`, and whether `mmCompress` must be
   exported.

3. **`GtWord`/`GtSubWord` home (clash C).** Confirm both the C free functions
   (plan → `gt`) and the `GtWord.*` methods (plan → `mm`) consolidate into a
   single package. Recommend `gt` (it owns `gtWord`/`gtSubword`/`mmCompress`),
   with `mm` importing `gt`.

4. **`qstate12` naming convention (clash G).** Bare name = C wrapper or = the
   existing high-level constructor? The existing code and the plan disagree on
   `UnitMatrix`/`PauliMatrix`/`FromSigns`/`ColumnMonomialMatrix`/
   `RowMonomialMatrix`. Pick one; rename the other (the plan already offers
   `Qs*` alternates).

5. **Export `qs12` as `QState12` (clash H)?** `xsp2co1` consumes `*qs12`. Either
   export it (matching the plan's `QState12`), or keep `qstate12` and `xsp2co1`
   able to share it some other way. The 7 `xsp2co1` functions affected:
   `ChainShort3`, `ElemToQs`, `ElemToQsI`, `MapShort3`, `MulQsV3Word`,
   `QsToElemI`, `RepMod3FromQs`.

6. **Export surface for `mat24` helpers (clash D).** Confirm the lowercase
   helpers (`parity12`, `vintern`, `cocodeSyndrome`, `synFromTable`,
   `gcodeToVectInternal`, …) should be exported, or that callers should be
   rewritten to use existing exported twins. Some (`parity12`,
   `gcodeToVectInternal`) have **no** exported twin and would need one.

7. **`Rng` placement.** The plan puts RNG (`RngSeed`, `RngUniform`, …) in `gen`,
   but `Rng` (`rng.go:30`) is consumed by `mm_op_vector.go` (→ `mm`). Confirm
   `Rng` → `gen` with `mm` importing it (creates a clean `mm → gen` edge), vs.
   leaving it in `mm`.

8. **Scope of the unimplemented surface.** A large fraction of `go.yaml` is
   **planned but not yet in the Go code** — e.g. the entire `gt` `GtWord`/
   `GtSubWord` method set, `OrbitElem2`/`OrbitLin2`/`RandomSubgroup` (`gen`),
   `Xsp2Co1Group` (`xsp2co1`), `QStateMatrix` (`qstate12`), and the `mm`
   `Abstract*`/`Axis`/`BabyAxis`/`BiMM`-group hierarchy with Python-derived
   (empty-typed) parameters. Decide whether migration extracts only the
   **implemented** subset now and grows each package toward the plan, or blocks
   on completing the translation first. The implemented exported types are:
   `XLeech2`, `GCode`,`Cocode`,`PLoop`,`PLoopIntersection`,`Parity`,`AutPL`,
   `Octad`, `MM`,`Axis`,`BiMM`,`AutP3`,`P3Node`,`Tag`,`Tuple`,`Subgroup`,
   `ChiMap`,`N0Elem`,`Xsp2Co1`,`QState`,`Rng` (plus unexported `qs12`/
   `qsSupport`/`gtWord`/`gtSubword`/`mmCompress`/`genSwar`). `GcVector` is
   listed with 22 methods in `go.yaml` but is **not yet implemented**;
   likewise `QStateMatrix`/`QState12` (`qstate12`), `GtWord`/`GtSubWord`
   (`gt`), `Xsp2Co1Group` (`xsp2co1`), `GcVector`/`AutPlGroup` (`mat24`), and
   `OrbitElem2`/`OrbitLin2`/`RandomSubgroup` (`gen`).
