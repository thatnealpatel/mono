// Package jsonldb implements a simple
// wrapper around a JSONL flatfile that
// safe for concurrent use as a portable
// database in unserious programs where
// sqlite is too thick or undesirable.
package jsonldb

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"patel.codes/unsafe/uuid"

	"golang.org/x/sys/unix"
)

// Store encapsulates a simple
// contract around a given dir
// containing a 'db.jsonl' with
// filled with T.
//
// Store is safe for concurrent
// use by multiple goroutines.
type Store[T any] struct{ dir string }

// Open returns a [Store] around
// a $dir/db.jsonl.
func Open[T any](dir string) *Store[T] { return &Store[T]{dir: dir} }

// Read acquires a non-exclusive FLOCK
// and filters all []T provided as an
// argument to f by calling ff on it.
func (s *Store[T]) Read(ff func(T) bool, f func([]T) error) error {
	unlock := s.rlock()
	defer unlock()
	d := s.load()
	if ff == nil {
		return f(d.items)
	}
	var filtered []T
	for _, item := range d.items {
		if ff(item) {
			filtered = append(filtered, item)
		}
	}
	return f(filtered)
}

// Write acquires an exclusive FLOCK
// and provides the [Tx] handle for
// callers to mutate the underlying
// database.
//
// If fn returns an error, the final
// write operation is aborted.
func (s *Store[T]) Write(fn func(tx *Tx[T]) error) error {
	unlock := s.wlock()
	defer unlock()
	d := s.load()

	tx := &Tx[T]{db: d}
	if err := fn(tx); err != nil {
		return err
	}
	return s.write(d)
}

// Tx provides an abstraction
// over doing operations over
// T in a consistent, safe way.
type Tx[T any] struct{ db *db[T] }

// All returns the underlying items
// in the current transaction.
func (tx *Tx[T]) All() []T { return tx.db.items }

// Len returns the number of
// records in the database.
func (tx *Tx[T]) Len() int { return len(tx.db.items) }

// Get does a linear scan for id
// in the backing database.
//
// If an element with id is not
// found Get returns nil.
func (tx *Tx[T]) Get(id uuid.UUID) *T {
	for i, uid := range tx.db.ids {
		if uid == id {
			return &tx.db.items[i]
		}
	}
	return nil
}

// Add assignes a [uuid.NewV7] uuid
// and stores t in the database and
// returns the UUIDv7 ID.
func (tx *Tx[T]) Add(t *T) (uuid.UUID, error) {
	id := uuid.NewV7()

	raw, err := json.Marshal(t)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("jsonldb: marshal: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return uuid.UUID{}, fmt.Errorf("jsonldb: unmarshal to map: %w", err)
	}

	idBytes, err := json.Marshal(id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("jsonldb: marshal id: %w", err)
	}
	m["id"] = idBytes

	merged, err := json.Marshal(m)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("jsonldb: re-marshal: %w", err)
	}

	var item T
	if err := json.Unmarshal(merged, &item); err != nil {
		return uuid.UUID{}, fmt.Errorf("jsonldb: unmarshal merged: %w", err)
	}

	tx.db.ids = append(tx.db.ids, id)
	tx.db.items = append(tx.db.items, item)
	return id, nil
}

// Delete does a linear scan looking
// for id and then removes it in-place
// return true if successful or false
// if the element was not found.
func (tx *Tx[T]) Delete(id uuid.UUID) bool {
	for i, uid := range tx.db.ids {
		if uid == id {
			tx.db.ids = append(tx.db.ids[:i], tx.db.ids[i+1:]...)
			tx.db.items = append(tx.db.items[:i], tx.db.items[i+1:]...)
			return true
		}
	}
	return false
}

type db[T any] struct {
	ids   []uuid.UUID // TODO(nealpatel): ugly, but whatever.
	items []T
}

func (s *Store[T]) load() *db[T] {
	path := filepath.Join(s.dir, "db.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &db[T]{}
		}
		panic("jsonldb: open: " + err.Error())
	}
	defer f.Close()

	var d db[T]
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var partial struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(line, &partial); err != nil {
			panic(fmt.Sprintf("jsonldb: decode id: %v", err))
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			panic(fmt.Sprintf("jsonldb: decode item: %v", err))
		}
		d.ids = append(d.ids, partial.ID)
		d.items = append(d.items, item)
	}
	if err := sc.Err(); err != nil {
		panic("jsonldb: scan: " + err.Error())
	}
	return &d
}

// write creates a temporary file that
// contains the new database and attempts
// to atomically update the existing file.
//
// write can fail but leaves data on disk
// in a recoverable state.
func (s *Store[T]) write(d *db[T]) error {
	path := filepath.Join(s.dir, "db.jsonl")
	f, err := os.CreateTemp(s.dir, "db-*.jsonl.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if f != nil {
			f.Close()
			os.Remove(tmp) // ignore error
		}
	}()
	enc := json.NewEncoder(f)
	for i, item := range d.items {
		raw, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal item %d: %w", i, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("unmarshal item %d: %w", i, err)
		}
		idBytes, err := json.Marshal(d.ids[i])
		if err != nil {
			return fmt.Errorf("marshal id %d: %w", i, err)
		}
		m["id"] = idBytes
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	return os.Rename(tmp, path)
}

func (s *Store[T]) rlock() func() {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		panic("jsonldb: mkdir: " + err.Error())
	}
	path := filepath.Join(s.dir, "lockfile")
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		panic("jsonldb: rlock open: " + err.Error())
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_SH); err != nil {
		panic("jsonldb: rlock: " + err.Error())
	}
	return func() { f.Close() }
}

func (s *Store[T]) wlock() func() {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		panic("jsonldb: mkdir: " + err.Error())
	}
	path := filepath.Join(s.dir, "lockfile")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		panic("jsonldb: wlock open: " + err.Error())
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		panic("jsonldb: wlock: " + err.Error())
	}
	return func() { f.Close() }
}
