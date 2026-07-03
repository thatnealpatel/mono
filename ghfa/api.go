package main

import "encoding/json"

type issue struct {
	Number    int              `json:"number"`
	Title     string           `json:"title"`
	Labels    []label          `json:"labels"`
	State     string           `json:"state"`
	Locked    bool             `json:"locked"`
	Assignees []user           `json:"assignees"`
	Milestone *json.RawMessage `json:"milestone"`
	Comments  int              `json:"comments"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
	ClosedAt  *string          `json:"closed_at"`
	Assignee  *user            `json:"assignee"`
	Body      string           `json:"body"`
	ClosedBy  *user            `json:"closed_by"`
}

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
	User      *user  `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Body      string `json:"body"`
}

type searchResult struct {
	TotalCount        int     `json:"total_count"`
	IncompleteResults bool    `json:"incomplete_results"`
	Items             []issue `json:"items"`
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
