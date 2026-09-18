package main

import "encoding/json"

type label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type searchResult struct {
	TotalCount        int               `json:"total_count"`
	IncompleteResults bool              `json:"incomplete_results"`
	Items             []json.RawMessage `json:"items"`
}

type issueRef struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

type issuePatch struct {
	Title       *string  `json:"title,omitempty"`
	Body        *string  `json:"body,omitempty"`
	State       *string  `json:"state,omitempty"`
	StateReason *string  `json:"state_reason,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type commentRequest struct {
	Body string `json:"body"`
}
