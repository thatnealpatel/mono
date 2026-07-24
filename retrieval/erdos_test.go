package retrieval

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeErdosFixture(t *testing.T, dir, yaml string) string {
	t.Helper()
	dataDir := filepath.Join(dir, "erdosproblems.git", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "problems.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write problems.yaml: %v", err)
	}
	return dir
}

const fixtureProblemsYAML = `- number: "1"
  prize: "$500"
  status:
    state: "open"
    last_update: "2025-08-31"
  oeis: ["A276661"]
  tags: ["number theory", "additive combinatorics"]
  comments: "sums of distinct unit fractions"
  formalized:
    state: "yes"
    last_update: "2025-08-31"

- number: "2"
  prize: "$1000"
  status:
    state: "disproved"
    last_update: "2025-08-31"
    note: "counterexample found"
  oeis: ["A160559"]
  tags: ["number theory", "covering systems"]
  comments: "covering congruences with distinct moduli"
  formalized:
    state: "no"
    last_update: "2025-08-31"

- number: "3"
  prize: "$5000"
  status:
    state: "proved"
    last_update: "2025-08-31"
  tags: ["graph theory", "ramsey"]
  comments: "ramsey numbers grow at least exponentially"
  formalized:
    state: "yes"
    last_update: "2025-08-31"
`

func TestErdosStoreListParseFidelity(t *testing.T) {
	dir := writeErdosFixture(t, t.TempDir(), fixtureProblemsYAML)
	store, err := NewErdos(dir, 20)
	if err != nil {
		t.Fatalf("NewErdos: %v", err)
	}

	res := store.List()
	if got, want := res.Results, 3; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if got, want := len(res.Problems), 3; got != want {
		t.Fatalf("got %d problems, want %d", got, want)
	}

	p := res.Problems[0]
	if got, want := p.Number, "1"; got != want {
		t.Errorf("got number %q, want %q", got, want)
	}
	if got, want := p.Prize, "$500"; got != want {
		t.Errorf("got prize %q, want %q", got, want)
	}
	if got, want := p.Status.State, "open"; got != want {
		t.Errorf("got status.state %q, want %q", got, want)
	}
	if got, want := p.Status.LastUpdate, "2025-08-31"; got != want {
		t.Errorf("got status.last_update %q, want %q", got, want)
	}
	if got, want := strings.Join(p.OEIS, ","), "A276661"; got != want {
		t.Errorf("got oeis %q, want %q", got, want)
	}
	if got, want := strings.Join(p.Tags, "|"), "number theory|additive combinatorics"; got != want {
		t.Errorf("got tags %q, want %q", got, want)
	}
	if got, want := p.Comment, "sums of distinct unit fractions"; got != want {
		t.Errorf("got comments %q, want %q", got, want)
	}
	if got, want := p.Formal.State, "yes"; got != want {
		t.Errorf("got formalized.state %q, want %q", got, want)
	}
	if got, want := res.Problems[1].Status.Note, "counterexample found"; got != want {
		t.Errorf("got status.note %q, want %q", got, want)
	}
}

func TestErdosStoreSearch(t *testing.T) {
	dir := writeErdosFixture(t, t.TempDir(), fixtureProblemsYAML)
	store, err := NewErdos(dir, 20)
	if err != nil {
		t.Fatalf("NewErdos: %v", err)
	}

	res := store.Search("ramsey graph")
	if got, want := res.Query, "ramsey graph"; got != want {
		t.Errorf("got query %q, want %q", got, want)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("got no matches, want at least one")
	}
	if got, want := res.Matches[0].Number, "3"; got != want {
		t.Errorf("got top match number %q, want %q", got, want)
	}
	if got, want := res.Results, len(res.Matches); got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
}

func TestErdosStoreSearchTopCap(t *testing.T) {
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "- number: %q\n  comments: \"shared term problem\"\n  tags: [\"shared\"]\n", fmt.Sprint(i))
	}
	dir := writeErdosFixture(t, t.TempDir(), b.String())
	store, err := NewErdos(dir, 20)
	if err != nil {
		t.Fatalf("NewErdos: %v", err)
	}
	res := store.Search("shared term")
	if got, want := len(res.Matches), 20; got != want {
		t.Errorf("got %d matches, want capped at %d", got, want)
	}
	if got, want := res.Results, 30; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
	if !res.Truncated {
		t.Errorf("got Truncated false, want true")
	}
}

func TestLoadErdosStoreMissingDir(t *testing.T) {
	if _, err := NewErdos(filepath.Join(t.TempDir(), "absent"), 20); err == nil {
		t.Errorf("got nil error for missing data dir, want error")
	}
}

func TestLoadErdosStoreMalformedYAML(t *testing.T) {
	dir := writeErdosFixture(t, t.TempDir(), "this: is: not: a: problem: list\n\t- broken")
	if _, err := NewErdos(dir, 20); err == nil {
		t.Errorf("got nil error for malformed yaml, want error")
	}
}

func TestNewErdos(t *testing.T) {
	dir := writeErdosFixture(t, t.TempDir(), fixtureProblemsYAML)

	store, err := NewErdos(dir, 20)
	if err != nil {
		t.Fatalf("NewErdos: %v", err)
	}
	if got, want := store.List().Results, 3; got != want {
		t.Errorf("got results %d, want %d", got, want)
	}
}

func TestErdosStoreNoHTTPClient(t *testing.T) {
	rt := reflect.TypeFor[Erdos]()
	httpClientType := reflect.TypeFor[http.Client]()
	for field := range rt.Fields() {
		ft := field.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft == httpClientType {
			t.Errorf("Erdos.%s is an http.Client; stores must never dial out", field.Name)
		}
	}
}
