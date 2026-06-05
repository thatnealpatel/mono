package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestParseForumPosts(t *testing.T) {
	for _, tt := range []struct {
		file       string
		wantTotal  int
		wantTop    int
		firstID    string
		firstAuthr string
	}{
		{
			file:       filepath.Join("..", "erdos727.html"),
			wantTotal:  6,
			wantTop:    5,
			firstID:    "post-3424",
			firstAuthr: "Dogmachine",
		},
		{
			file:       filepath.Join("..", "erdos20.html"),
			wantTotal:  9,
			wantTop:    7,
			firstID:    "post-6089",
			firstAuthr: "Dogmachine",
		},
	} {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(tt.file)
			if err != nil {
				t.Skipf("sample file not available: %v", err)
			}
			posts := parseForumPosts(string(data))
			total := countPosts(posts)
			if total != tt.wantTotal {
				t.Errorf("total posts = %d, want %d", total, tt.wantTotal)
			}
			if len(posts) != tt.wantTop {
				t.Errorf("top-level posts = %d, want %d", len(posts), tt.wantTop)
			}
			if len(posts) > 0 {
				if posts[0].ID != tt.firstID {
					t.Errorf("first post ID = %q, want %q", posts[0].ID, tt.firstID)
				}
				if posts[0].Author != tt.firstAuthr {
					t.Errorf("first post author = %q, want %q", posts[0].Author, tt.firstAuthr)
				}
				if posts[0].BodyHTML == "" {
					t.Error("first post body_html is empty")
				}
				if posts[0].Date == "" {
					t.Error("first post date is empty")
				}
			}
		})
	}
}

func TestParseForumPostsReplies(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "erdos727.html"))
	if err != nil {
		t.Skipf("sample file not available: %v", err)
	}
	posts := parseForumPosts(string(data))

	var withReplies *ForumPost
	for i := range posts {
		if len(posts[i].Replies) > 0 {
			withReplies = &posts[i]
			break
		}
	}
	if withReplies == nil {
		t.Fatal("expected at least one post with replies in erdos727.html")
	}
	if withReplies.ID != "post-851" {
		t.Errorf("post with replies ID = %q, want %q", withReplies.ID, "post-851")
	}
	if len(withReplies.Replies) != 1 {
		t.Fatalf("replies count = %d, want 1", len(withReplies.Replies))
	}
	reply := withReplies.Replies[0]
	if reply.ID != "post-852" {
		t.Errorf("reply ID = %q, want %q", reply.ID, "post-852")
	}
	if reply.Author != "Thomas Bloom" {
		t.Errorf("reply author = %q, want %q", reply.Author, "Thomas Bloom")
	}
}
