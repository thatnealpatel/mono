package main

import (
	"testing"
)

// implUntested is a Record that is implemented but not oracle-tested: the
// queue's eligibility predicate. Tests build matrices from these.
func implUntested(pkg, name, recv string) Record {
	return Record{Package: pkg, Name: name, Recv: recv, Implemented: true, OracleTested: false}
}

func TestBuildGroups_TierAndWeightOrdering(t *testing.T) {
	// Two lead packages (xsp2co1 heavier, leech lighter) plus a heavier and a
	// lighter partial package. Lead tier must precede partials regardless of
	// weight, lead tier follows the canonical C1 order, and partials sort
	// heaviest-first.
	recs := []Record{
		implUntested("xsp2co1", "A", ""),
		implUntested("xsp2co1", "B", ""),
		implUntested("leech", "C", ""),
		implUntested("monster", "D", ""),
		implUntested("monster", "E", ""),
		implUntested("monster", "F", ""),
		implUntested("qstate12", "G", ""),
	}
	groups := buildGroups(recs)

	wantPkgOrder := []string{"leech", "xsp2co1", "monster", "qstate12"}
	if len(groups) != len(wantPkgOrder) {
		t.Fatalf("group count = %d, want %d: %+v", len(groups), len(wantPkgOrder), groups)
	}
	for i, want := range wantPkgOrder {
		if groups[i].Package != want {
			t.Errorf("group[%d].Package = %q, want %q", i, groups[i].Package, want)
		}
	}
	// leech and xsp2co1 are the lead tier; monster and qstate12 are partials.
	if groups[0].Tier != 0 || groups[1].Tier != 0 {
		t.Errorf("lead groups have non-zero tier: %d, %d", groups[0].Tier, groups[1].Tier)
	}
	if groups[2].Tier != 1 || groups[3].Tier != 1 {
		t.Errorf("partial groups have non-one tier: %d, %d", groups[2].Tier, groups[3].Tier)
	}
}

func TestBuildGroups_DropsNameSkewArtifacts(t *testing.T) {
	// An mmindex row that is impl-but-untested only because its test lives
	// under the Aux-stripped name is a naming artifact, not a gap: it must not
	// enter the queue.
	skew := implUntested("mmindex", "AuxIndexInternToLeech2", "")
	skew.NameSkew = true
	recs := []Record{
		skew,
		implUntested("leech", "Real", ""),
	}
	groups := buildGroups(recs)
	for _, g := range groups {
		if g.Package == "mmindex" {
			t.Fatalf("mmindex name_skew artifact leaked into queue: %+v", g)
		}
	}
	if len(groups) != 1 || groups[0].Package != "leech" {
		t.Fatalf("expected only the leech group, got %+v", groups)
	}
}

func TestBuildGroups_EntriesCheapestFirst(t *testing.T) {
	// Within a group, type-recovery cost orders entries: fully-typed (none)
	// before C2 void-inversions before genuinely-unknown blanks.
	blank := implUntested("leech", "BlankUnknown", "")
	blank.BlankReturn = true

	voidInv := implUntested("leech", "AVoidInv", "")
	voidInv.BlankReturn = true
	voidInv.ResolvableVoid = true
	voidInv.CReturn = "void"

	typed := implUntested("leech", "ZTyped", "")

	groups := buildGroups([]Record{blank, voidInv, typed})
	if len(groups) != 1 {
		t.Fatalf("expected one group, got %d", len(groups))
	}
	wantOrder := []string{"none", "void-inversion", "unknown-blank"}
	got := groups[0].Entries
	if len(got) != len(wantOrder) {
		t.Fatalf("entry count = %d, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].TypeRecovery != want {
			t.Errorf("entry[%d].TypeRecovery = %q (name %q), want %q",
				i, got[i].TypeRecovery, got[i].Name, want)
		}
	}
}

func TestBuildGroups_SplitsByReceiver(t *testing.T) {
	// Free functions and methods of the same package form distinct groups,
	// with the receiver-less group ordered first.
	recs := []Record{
		implUntested("reduce", "Reduce", "GtWord"),
		implUntested("reduce", "NewGtWord", ""),
	}
	groups := buildGroups(recs)
	if len(groups) != 2 {
		t.Fatalf("expected two groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Recv != "" || groups[1].Recv != "GtWord" {
		t.Errorf("receiver ordering wrong: %q then %q", groups[0].Recv, groups[1].Recv)
	}
}

func TestBuildGroups_SkipsTestedAndUnimplemented(t *testing.T) {
	tested := implUntested("leech", "Tested", "")
	tested.OracleTested = true
	unimpl := Record{Package: "leech", Name: "Unimpl", Implemented: false}
	groups := buildGroups([]Record{tested, unimpl})
	if len(groups) != 0 {
		t.Fatalf("expected empty queue, got %+v", groups)
	}
}
