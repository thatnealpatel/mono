package main

import "encoding/json"

type label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type user struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

type comment struct {
	ID              *int    `json:"id,omitempty"`
	User            *user   `json:"user"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	Body            string  `json:"body"`
	IsMinimized     *bool   `json:"is_minimized,omitempty"`
	MinimizedReason *string `json:"minimized_reason,omitempty"`
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
