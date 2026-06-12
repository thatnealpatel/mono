package main

import "testing"

// rec builds a coverage Record with the two columns the report consumes.
func rec(impl, tested bool) Record {
	return Record{Implemented: impl, OracleTested: tested}
}

func TestBuildReport_SpecAndOracleCounting(t *testing.T) {
	// Four entries: implemented+tested, implemented+untested, unimplemented
	// but referenced in a test (must NOT count toward oracle), and neither.
	// Spec = implemented/entries = 2/4; oracle = impl&tested/implemented = 1/2.
	records := []Record{
		rec(true, true),
		rec(true, false),
		rec(false, true), // unimplemented: never counts as implemented or oracle
		rec(false, false),
	}
	rep := buildReport(records, surfaceReport{})

	if rep.Spec.Numerator != 2 || rep.Spec.Denominator != 4 {
		t.Errorf("spec = %d/%d, want 2/4", rep.Spec.Numerator, rep.Spec.Denominator)
	}
	if rep.Oracle.Numerator != 1 || rep.Oracle.Denominator != 2 {
		t.Errorf("oracle = %d/%d, want 1/2", rep.Oracle.Numerator, rep.Oracle.Denominator)
	}
	if rep.Spec.Value != 0.5 || rep.Oracle.Value != 0.5 {
		t.Errorf("ratio values = %g, %g, want 0.5, 0.5", rep.Spec.Value, rep.Oracle.Value)
	}
}

func TestBuildReport_OracleRequiresImplemented(t *testing.T) {
	// An entry referenced in a test but not implemented is unverified
	// translation, not oracle coverage: it must inflate neither numerator.
	rep := buildReport([]Record{rec(false, true)}, surfaceReport{})
	if rep.Oracle.Numerator != 0 {
		t.Errorf("oracle numerator = %d, want 0 (untested-because-unimplemented)", rep.Oracle.Numerator)
	}
	if rep.Oracle.Denominator != 0 || rep.Oracle.Value != 0 {
		t.Errorf("oracle = %d/%d value %g, want 0/0 value 0", rep.Oracle.Numerator, rep.Oracle.Denominator, rep.Oracle.Value)
	}
}

func TestBuildReport_CSurfaceRatios(t *testing.T) {
	// full-surface = (correlated+internal+untranslated)/exports; in-scope =
	// translated/translated. With every export accounted for, both read 1.0.
	surf := surfaceReport{C: cSurface{
		Exports: 100, Correlated: 80, Internal: 10, Untranslated: 10,
	}}
	rep := buildReport(nil, surf)
	if rep.CSurf.FullSurface.Numerator != 100 || rep.CSurf.FullSurface.Denominator != 100 {
		t.Errorf("c full-surface = %d/%d, want 100/100", rep.CSurf.FullSurface.Numerator, rep.CSurf.FullSurface.Denominator)
	}
	if rep.CSurf.InScope.Numerator != 90 || rep.CSurf.InScope.Denominator != 90 {
		t.Errorf("c in-scope = %d/%d, want 90/90 (translated only)", rep.CSurf.InScope.Numerator, rep.CSurf.InScope.Denominator)
	}
	if !rep.CSurf.FullSurfaceIsHeadline {
		t.Error("Hu1: full-surface must be marked the headline for the C surface")
	}
}

func TestBuildReport_CSurfaceHeadroomShows(t *testing.T) {
	// A genuine gap (an export accounted by neither translation nor
	// classification) must drop the full-surface ratio below 1.0: the
	// headline is honest, not pinned complete.
	surf := surfaceReport{C: cSurface{
		Exports: 100, Correlated: 80, Internal: 10, Untranslated: 5,
		Unclassified: []string{"mm_op99_new (in x.ske)"},
	}}
	rep := buildReport(nil, surf)
	if rep.CSurf.FullSurface.Numerator != 95 || rep.CSurf.FullSurface.Denominator != 100 {
		t.Errorf("c full-surface = %d/%d, want 95/100 (5 accounted of denominator missing one)", rep.CSurf.FullSurface.Numerator, rep.CSurf.FullSurface.Denominator)
	}
	if rep.CSurf.FullSurface.Value >= 1.0 {
		t.Errorf("c full-surface value = %g, want < 1.0 with an unclassified export", rep.CSurf.FullSurface.Value)
	}
}

func TestBuildReport_PySurfaceRatios(t *testing.T) {
	surf := surfaceReport{Py: pySurface{
		Surface: 50, Covered: 40, Delegated: 5, OutOfScope: 5,
	}}
	rep := buildReport(nil, surf)
	if rep.PySurf.FullSurface.Numerator != 50 || rep.PySurf.FullSurface.Denominator != 50 {
		t.Errorf("py full-surface = %d/%d, want 50/50", rep.PySurf.FullSurface.Numerator, rep.PySurf.FullSurface.Denominator)
	}
	if rep.PySurf.InScope.Numerator != 45 || rep.PySurf.InScope.Denominator != 45 {
		t.Errorf("py in-scope = %d/%d, want 45/45 (covered+delegated)", rep.PySurf.InScope.Numerator, rep.PySurf.InScope.Denominator)
	}
}

func TestBuildReport_GreenAndGaps(t *testing.T) {
	clean := buildReport(nil, surfaceReport{})
	if !clean.Green || clean.Gaps != 0 {
		t.Errorf("empty surface: green=%v gaps=%d, want green=true gaps=0", clean.Green, clean.Gaps)
	}

	gapped := buildReport(nil, surfaceReport{
		C:  cSurface{Unclassified: []string{"c_gap"}},
		Py: pySurface{Unclassified: []string{"Py.gap1", "Py.gap2"}},
	})
	if gapped.Green {
		t.Error("a surface with unclassified symbols must not be green")
	}
	if gapped.Gaps != 3 {
		t.Errorf("gaps = %d, want 3 (1 C + 2 Py)", gapped.Gaps)
	}
}

func TestRatio_ZeroDenominator(t *testing.T) {
	// An empty denominator must yield value 0, never NaN, so the JSON is
	// always well-formed.
	r := ratio(0, 0)
	if r.Value != 0 {
		t.Errorf("ratio(0,0).Value = %g, want 0", r.Value)
	}
}
