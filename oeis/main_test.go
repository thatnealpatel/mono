package main

import (
	"encoding/json"
	"testing"
)

func TestSearchOutputFormat(t *testing.T) {
	type result struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	out := struct {
		Query   string   `json:"query"`
		Results int      `json:"results"`
		Matches []result `json:"matches"`
	}{
		"groups",
		2,
		[]result{
			{"A000001", "Number of groups of order n", 5.0},
			{"A000040", "The prime numbers", 3.0},
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Query   string   `json:"query"`
		Results int      `json:"results"`
		Matches []result `json:"matches"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Query != "groups" {
		t.Errorf("query = %q, want %q", parsed.Query, "groups")
	}
	if len(parsed.Matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(parsed.Matches))
	}
	if parsed.Matches[0].ID != "A000001" {
		t.Errorf("matches[0].id = %q, want %q", parsed.Matches[0].ID, "A000001")
	}
}
