package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patel.codes/retrieval"
)

func TestSearchMatchFields(t *testing.T) {
	m := retrieval.ErdosMatch{
		ErdosProblem: retrieval.ErdosProblem{
			Number: "165",
			Tags:   []string{"combinatorics"},
			Status: retrieval.ErdosStatus{State: "open"},
		},
		Score: 1.5,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"number", "tags", "score"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("match missing top-level %q field", key)
		}
	}
	if _, ok := fields["problem"]; ok {
		t.Error("match has nested problem wrapper")
	}
}

func TestRemoteList(t *testing.T) {
	want := `{"results":1,"problems":[{"number":"42"}]}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/erdos" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var req struct {
			Subcommand string `json:"subcommand"`
			Query      string `json:"query"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if got, wantSub := req.Subcommand, "list"; got != wantSub {
			t.Errorf("subcommand = %q, want %q", got, wantSub)
		}
		io.WriteString(w, want)
	}))
	defer srv.Close()
	t.Setenv("RETRIEVAL_HOST", srv.URL)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	if err := cmdList(); err != nil {
		os.Stdout = old
		t.Fatalf("cmdList: %v", err)
	}
	w.Close()
	os.Stdout = old
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoteSearch(t *testing.T) {
	want := `{"query":"graphs","results":0,"truncated":false,"matches":[]}` + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var req struct {
			Subcommand string `json:"subcommand"`
			Query      string `json:"query"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if got, wantSub := req.Subcommand, "search"; got != wantSub {
			t.Errorf("subcommand = %q, want %q", got, wantSub)
		}
		if got, wantQ := req.Query, "graphs"; got != wantQ {
			t.Errorf("query = %q, want %q", got, wantQ)
		}
		io.WriteString(w, want)
	}))
	defer srv.Close()
	t.Setenv("RETRIEVAL_HOST", srv.URL)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	if err := cmdSearch("graphs"); err != nil {
		os.Stdout = old
		t.Fatalf("cmdSearch: %v", err)
	}
	w.Close()
	os.Stdout = old
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRemoteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "store unavailable", 503)
	}))
	defer srv.Close()
	t.Setenv("RETRIEVAL_HOST", srv.URL)
	err := cmdList()
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
	if got, want := err.Error(), "retrieval returned"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want substring %q", got, want)
	}
}

func countPosts(posts []ForumPost) int {
	n := len(posts)
	for _, p := range posts {
		n += countPosts(p.Replies)
	}
	return n
}

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
			posts, err := parseForumPosts(string(data))
			if err != nil {
				t.Fatalf("parseForumPosts: %v", err)
			}
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
	posts, err := parseForumPosts(string(data))
	if err != nil {
		t.Fatalf("parseForumPosts: %v", err)
	}

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
