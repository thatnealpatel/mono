package main

import (
	"os"
	"testing"
)

func TestHarvestIlean(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		wantNames map[string]string
	}{
		{
			name:    "DeclsAndReferences",
			fixture: "testdata/basic.ilean",
			wantNames: map[string]string{
				"Plausible.Testable":     "Plausible.Testable",
				"Plausible.Testable.run": "Plausible.Testable",
				"Sum":                    "Init.Core",
				"Classical.em":           "Init.Classical",
			},
		},
		{
			name:    "GeneratedNameOnlyInReferences",
			fixture: "testdata/generated_only.ilean",
			wantNames: map[string]string{
				"Fin.prod_univ_eq_prod_range": "Mathlib.Data.Fintype.BigOperators",
				"Fin.sum_univ_eq_sum_range":   "Mathlib.Data.Fintype.BigOperators",
				"Summable.tsum_eq_zero_add":   "Mathlib.Topology.Algebra.InfiniteSum",
			},
		},
		{
			name:    "DeclsWinOnCollision",
			fixture: "testdata/collision.ilean",
			wantNames: map[string]string{
				"Plausible.Testable": "Other.Module",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got, err := harvestIlean(data)
			if err != nil {
				t.Fatalf("harvestIlean: %v", err)
			}
			for name, wantMod := range tt.wantNames {
				if gotMod, ok := got[name]; !ok {
					t.Errorf("missing name %q", name)
				} else if gotMod != wantMod {
					t.Errorf("name %q: got module %q, want %q", name, gotMod, wantMod)
				}
			}
		})
	}
}

func TestHarvestIleanCollisionPrecedence(t *testing.T) {
	data := []byte(`{
		"version": 7,
		"module": "MyModule",
		"decls": {"Foo.bar": [[0, 1]]},
		"references": {
			"{\"c\":{\"m\":\"Foreign\",\"n\":\"Foo.bar\"}}": {"usages": [[0, 2]]}
		}
	}`)
	got, err := harvestIlean(data)
	if err != nil {
		t.Fatalf("harvestIlean: %v", err)
	}
	if gotMod, wantMod := got["Foo.bar"], "MyModule"; gotMod != wantMod {
		t.Errorf("got module %q, want %q", gotMod, wantMod)
	}
}

func TestWalkIleanNames(t *testing.T) {
	got, err := walkIleanNames("testdata")
	if err != nil {
		t.Fatalf("walkIleanNames: %v", err)
	}

	checks := map[string]string{
		"Plausible.Testable.run":    "Plausible.Testable",
		"Classical.em":              "Init.Classical",
		"Fin.sum_univ_eq_sum_range": "Mathlib.Data.Fintype.BigOperators",
		"Summable.tsum_eq_zero_add": "Mathlib.Topology.Algebra.InfiniteSum",
	}
	for name, wantMod := range checks {
		if gotMod, ok := got[name]; !ok {
			t.Errorf("missing name %q", name)
		} else if gotMod != wantMod {
			t.Errorf("name %q: got module %q, want %q", name, gotMod, wantMod)
		}
	}
}
