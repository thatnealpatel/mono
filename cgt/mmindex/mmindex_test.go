package mmindex

import (
	"fmt"
	"testing"
)

func TestIndexExternSparseRoundtrip(t *testing.T) {
	t.Parallel()
	for _, idx := range []int{0, 23, 300, 852, 49428, 196883} {
		wantSp := oracleUint(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_extern_to_sparse(%d))", idx))
		if got := uint64(IndexExternToSparse(idx)); got != wantSp {
			t.Fatalf("IndexExternToSparse(%d) = %#x, want %#x", idx, got, wantSp)
		}
		wantExt := oracleInt(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_sparse_to_extern(%d))", wantSp))
		if got := int64(IndexSparseToExtern(uint32(wantSp))); got != wantExt {
			t.Fatalf("IndexSparseToExtern(%#x) = %d, want %d", wantSp, got, wantExt)
		}
	}
}

func TestIndexExternToIntern(t *testing.T) {
	t.Parallel()
	for _, idx := range []int{0, 24, 300, 853, 49500, 100000} {
		want := oracleInt(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_extern_to_intern(%d))", idx))
		if got := int64(IndexExternToIntern(idx)); got != want {
			t.Fatalf("IndexExternToIntern(%d) = %d, want %d", idx, got, want)
		}
	}
}

func TestIndexSparseLeech2(t *testing.T) {
	t.Parallel()
	for _, idx := range []int{0, 300, 852, 49428, 196883} {
		sp := IndexExternToSparse(idx)
		intern := IndexSparseToIntern(sp)
		wantIntern := oracleInt(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_sparse_to_intern(%d))", sp))
		if int64(intern) != wantIntern {
			t.Fatalf("IndexSparseToIntern(%#x) = %d, want %d", sp, intern, wantIntern)
		}
		backSp := IndexInternToSparse(intern)
		wantBackSp := oracleUint(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_intern_to_sparse(%d))", intern))
		if uint64(backSp) != wantBackSp {
			t.Fatalf("IndexInternToSparse(%d) = %#x, want %#x", intern, backSp, wantBackSp)
		}
	}
}

func TestIndexCheckIntern(t *testing.T) {
	t.Parallel()
	for _, ext := range []int{0, 24, 300, 853, 49500, 100000} {
		intern := IndexExternToIntern(ext)
		got := IndexCheckIntern(intern)
		want := int(oracleInt(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_check_intern(%d))", intern)))
		if got != want {
			t.Fatalf("IndexCheckIntern(%d)=%d want %d", intern, got, want)
		}
	}
}

func TestIndexSparseToLeech2(t *testing.T) {
	t.Parallel()
	for _, ext := range []int{300, 852, 49428, 196883} {
		sp := IndexExternToSparse(ext)
		got := IndexSparseToLeech2(sp)
		want := oracleUint(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_sparse_to_leech2(%d))", sp))
		if uint64(got) != want {
			t.Fatalf("IndexSparseToLeech2(%#x)=%#x want %#x", sp, got, want)
		}
	}
}

func TestIndexLeech2ToSparse(t *testing.T) {
	t.Parallel()
	for _, ext := range []int{300, 852, 49428, 196883} {
		sp := IndexExternToSparse(ext)
		x := IndexSparseToLeech2(sp)
		got := IndexLeech2ToSparse(x)
		want := oracleUint(t, fmt.Sprintf("int(mmgroup.mm_op.mm_aux_index_leech2_to_sparse(%d))", x))
		if uint64(got) != want {
			t.Fatalf("IndexLeech2ToSparse(%#x)=%#x want %#x", x, got, want)
		}
	}
}
