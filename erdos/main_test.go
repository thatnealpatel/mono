package main

import "testing"

func TestParseHTML(t *testing.T) {
	for _, tt := range []struct {
		name     string
		html     string
		wantStmt string
		wantSecs int
	}{
		{
			name:     "statement only",
			html:     `<div id="content">Problem statement here.</div>`,
			wantStmt: "Problem statement here.",
			wantSecs: 0,
		},
		{
			name: "statement with additional text",
			html: `<div id="content">Statement.</div>` +
				`<div class="problem-additional-text">Additional info.</div>`,
			wantStmt: "Statement.",
			wantSecs: 1,
		},
		{
			name: "multiple additional sections",
			html: `<div id="content">Main.</div>` +
				`<div class="problem-additional-text">Section 1.</div>` +
				`<div class="problem-additional-text">Section 2.</div>`,
			wantStmt: "Main.",
			wantSecs: 2,
		},
		{
			name:     "no content div",
			html:     `<div id="other">Not a statement.</div>`,
			wantStmt: "",
			wantSecs: 0,
		},
		{
			name:     "nested divs in statement",
			html:     `<div id="content"><div>inner</div> outer</div>`,
			wantStmt: "inner outer",
			wantSecs: 0,
		},
		{
			name:     "br and p tags",
			html:     `<div id="content">line1<br>line2<p>para</p></div>`,
			wantStmt: "line1\nline2\n\npara",
			wantSecs: 0,
		},
		{
			name:     "italic tags",
			html:     `<div id="content">this is <i>emphasized</i> text</div>`,
			wantStmt: "this is *emphasized* text",
			wantSecs: 0,
		},
		{
			name:     "noise removal",
			html:     `<div id="content">Real content. Back to the problem</div>`,
			wantStmt: "Real content.",
			wantSecs: 0,
		},
		{
			name:     "empty content div",
			html:     `<div id="content">   </div>`,
			wantStmt: "",
			wantSecs: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stmt, secs := parseHTML(tt.html)
			if stmt != tt.wantStmt {
				t.Errorf("statement = %q, want %q", stmt, tt.wantStmt)
			}
			if len(secs) != tt.wantSecs {
				t.Errorf("sections = %d, want %d", len(secs), tt.wantSecs)
			}
		})
	}
}

func TestCleanMath(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "triple newlines collapsed",
			in:   "a\n\n\nb",
			want: "a\n\nb",
		},
		{
			name: "noise removed",
			in:   "text Back to the problem more text",
			want: "text  more text",
		},
		{
			name: "whitespace trimmed",
			in:   "  hello  ",
			want: "hello",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanMath(tt.in); got != tt.want {
				t.Errorf("cleanMath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
