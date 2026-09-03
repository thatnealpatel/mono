// Package indexing builds and queries a SQLite
// FTS5 index of documented declarations.
package indexing

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// SchemaVersion is stored in the
// database as user_version; Open refuses
// a mismatch.
const SchemaVersion = 1

// Tokenizer splits identifiers and
// queries into terms. Identifier
// columns are stored pre-tokenized so
// that camelCase and dotted names are
// searchable by their parts; prose
// columns are stored raw and stemmed by
// FTS5.
type Tokenizer interface {
	Tokenize(text string) []string
}

// Record is one declaration to index.
type Record struct {
	Name      string
	Qualname  string
	Kind      string
	Signature string
	Docstring string
	Examples  string
	File      string
	Line      int
}

// Match is one declaration in an Envelope.
type Match struct {
	Name      string  `json:"name"`
	Qualname  string  `json:"qualname,omitempty"`
	Kind      string  `json:"kind"`
	Signature string  `json:"signature,omitempty"`
	Docstring string  `json:"docstring,omitempty"`
	Snippet   string  `json:"snippet,omitempty"`
	Body      string  `json:"body,omitempty"`
	File      string  `json:"file"`
	Line      int     `json:"line"`
	Score     float64 `json:"score,omitempty"`
}

// Envelope is the JSON result of a query.
// Mode is "exact", "miss", or "search".
type Envelope struct {
	Mode       string   `json:"mode"`
	Matches    []Match  `json:"matches"`
	Candidates []string `json:"candidates,omitempty"`
}

const schema = `
create table decls(
	id        integer primary key,
	name      text not null,
	qualname  text not null,
	kind      text not null,
	signature text not null,
	docstring text not null,
	file      text not null,
	line      integer not null,
	final     text not null
);
create table names(
	name  text primary key,
	final text not null,
	file  text not null
) without rowid;
create virtual table fts using fts5(
	name, signature, docstring, examples,
	tokenize='porter unicode61'
);
insert into fts(fts, rank) values('rank', 'bm25(10.0, 3.0, 1.0, 2.0)');
`

// Builder writes a new index. It writes
// to a temporary file and renames it into
// place on Close, so a failed build never
// leaves a partial index.
type Builder struct {
	path string
	db   *sql.DB
	tx   *sql.Tx
	decl *sql.Stmt
	fts  *sql.Stmt
	name *sql.Stmt
	tok  Tokenizer
}

// Create starts building the index at path.
func Create(path string, tok Tokenizer) (*Builder, error) {
	tmp := path + ".tmp"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", "file:"+tmp+"?_pragma=journal_mode(off)&_pragma=synchronous(off)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(fmt.Sprintf("pragma user_version = %d", SchemaVersion)); err != nil {
		db.Close()
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, err
	}
	b := &Builder{path: path, db: db, tx: tx, tok: tok}
	b.decl, err = tx.Prepare(`insert into decls(name, qualname, kind, signature, docstring, file, line, final)
		values(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err == nil {
		b.fts, err = tx.Prepare(`insert into fts(rowid, name, signature, docstring, examples) values(?, ?, ?, ?, ?)`)
	}
	if err == nil {
		b.name, err = tx.Prepare(`insert or ignore into names(name, final, file) values(?, ?, ?)`)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return b, nil
}

// Add indexes one declaration.
func (b *Builder) Add(r Record) error {
	res, err := b.decl.Exec(r.Name, r.Qualname, r.Kind, r.Signature, r.Docstring, r.File, r.Line, final(r.Name))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	_, err = b.fts.Exec(id, b.shadow(r.Name), b.shadow(r.Signature), r.Docstring, r.Examples)
	return err
}

// AddName records a name that exists but has no
// declaration to index, such as a compiled name
// with no source.
//
// It is served by Lookup only.
func (b *Builder) AddName(name, file string) error {
	_, err := b.name.Exec(name, final(name), file)
	return err
}

// Close commits the index and moves it into place.
func (b *Builder) Close() error {
	if err := b.tx.Commit(); err != nil {
		b.db.Close()
		return err
	}
	for _, s := range []string{
		"create index decls_name on decls(name)",
		"create index decls_final on decls(final)",
		"create index names_final on names(final)",
		"insert into fts(fts) values('optimize')",
	} {
		if _, err := b.db.Exec(s); err != nil {
			b.db.Close()
			return err
		}
	}
	if err := b.db.Close(); err != nil {
		return err
	}
	return os.Rename(b.path+".tmp", b.path)
}

func (b *Builder) shadow(text string) string {
	return strings.Join(b.tok.Tokenize(text), " ")
}

func final(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(name)
}

// Index is an open, read-only index.
type Index struct {
	db  *sql.DB
	tok Tokenizer
}

// Open opens the index at path.
func Open(path string, tok Tokenizer) (*Index, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	var v int
	if err := db.QueryRow("pragma user_version").Scan(&v); err != nil {
		db.Close()
		return nil, err
	}
	if v != SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("%s: schema version %d, want %d", path, v, SchemaVersion)
	}
	return &Index{db: db, tok: tok}, nil
}

func (ix *Index) Close() error {
	return ix.db.Close()
}

// Lookup resolves an exact name. The mode is "exact"
// when the name is a declaration or a bare name, and
// "miss" otherwise, with candidates that share the same
// final dotted component.
func (ix *Index) Lookup(name string) (*Envelope, error) {
	rows, err := ix.db.Query(`select name, qualname, kind, signature, docstring, file, line
		from decls where name = ? order by id`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.Name, &m.Qualname, &m.Kind, &m.Signature, &m.Docstring, &m.File, &m.Line); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(matches) > 0 {
		return &Envelope{Mode: "exact", Matches: matches}, nil
	}

	var file string
	err = ix.db.QueryRow("select file from names where name = ?", name).Scan(&file)
	if err == nil {
		m := Match{Name: name, Kind: "generated", File: file}
		return &Envelope{Mode: "exact", Matches: []Match{m}}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	candidates, err := ix.candidates(final(name))
	if err != nil {
		return nil, err
	}
	return &Envelope{Mode: "miss", Matches: []Match{}, Candidates: candidates}, nil
}

func (ix *Index) candidates(final string) ([]string, error) {
	rows, err := ix.db.Query(`select name from (
		select name, 0 as src from decls where final = ?
		union all
		select name, 1 from names where final = ?
	) group by name order by min(src) limit 10`, final, final)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Search ranks declarations by BM25 against the query
// terms. Results carry a docstring snippet rather than
// the full docstring; use Lookup for the full record.
func (ix *Index) Search(query string) (*Envelope, error) {
	env := &Envelope{Mode: "search", Matches: []Match{}}
	terms := ix.tok.Tokenize(query)
	if len(terms) == 0 {
		return env, nil
	}
	rows, err := ix.db.Query(`select d.name, d.qualname, d.kind, d.signature, d.file, d.line,
			snippet(fts, 2, '', '', '…', 16), -rank
		from fts join decls d on d.id = fts.rowid
		where fts match ? order by rank`, matchExpr(terms))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.Name, &m.Qualname, &m.Kind, &m.Signature, &m.File, &m.Line, &m.Snippet, &m.Score); err != nil {
			return nil, err
		}
		env.Matches = append(env.Matches, m)
	}
	return env, rows.Err()
}

func matchExpr(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR ")
}
