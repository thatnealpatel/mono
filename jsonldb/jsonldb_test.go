package jsonldb

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"patel.codes/jsonldb/internal/uuid"
)

type entry struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Age  int       `json:"age"`
}

func TestCRUD(t *testing.T) {
	s := Open[entry](t.TempDir())
	var id uuid.UUID
	s.Write(func(tx *Tx[entry]) error {
		var err error
		id, err = tx.Add(&entry{Name: "alice", Age: 30})
		return err
	})
	if id == (uuid.UUID{}) {
		t.Fatal("Add returned zero UUID")
	}
	s.Read(nil, func(items []entry) error {
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].Name != "alice" || items[0].Age != 30 {
			t.Fatalf("unexpected item: %+v", items[0])
		}
		return nil
	})
	s.Write(func(tx *Tx[entry]) error {
		got := tx.Get(id)
		if got == nil {
			t.Fatal("Get returned nil")
		}
		if got.Name != "alice" {
			t.Fatalf("expected alice, got %s", got.Name)
		}
		return nil
	})
	s.Write(func(tx *Tx[entry]) error {
		if !tx.Delete(id) {
			t.Fatal("Delete returned false")
		}
		return nil
	})
	s.Read(nil, func(items []entry) error {
		if len(items) != 0 {
			t.Fatalf("expected 0 items after delete, got %d", len(items))
		}
		return nil
	})
}

func TestGetNonexistent(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		if got := tx.Get(uuid.NewV7()); got != nil {
			t.Fatal("expected nil for nonexistent ID")
		}
		return nil
	})
}

func TestDeleteNonexistent(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		if tx.Delete(uuid.NewV7()) {
			t.Fatal("expected false for nonexistent ID")
		}
		return nil
	})
}

func TestAllLen(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		for _, name := range []string{"a", "b", "c"} {
			if _, err := tx.Add(&entry{Name: name}); err != nil {
				return err
			}
		}
		if tx.Len() != 3 {
			t.Fatalf("expected Len=3, got %d", tx.Len())
		}
		if len(tx.All()) != 3 {
			t.Fatalf("expected All len=3, got %d", len(tx.All()))
		}
		return nil
	})
	s.Write(func(tx *Tx[entry]) error {
		tx.Delete(tx.All()[0].ID)
		if tx.Len() != 2 {
			t.Fatalf("expected Len=2 after delete, got %d", tx.Len())
		}
		return nil
	})
}

func TestWriteRollbackOnPanic(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		_, err := tx.Add(&entry{Name: "keeper"})
		return err
	})

	func() {
		defer func() { recover() }()
		s.Write(func(tx *Tx[entry]) error {
			tx.Add(&entry{Name: "transient"})
			panic("simulated failure")
		})
	}()

	s.Read(nil, func(items []entry) error {
		if len(items) != 1 {
			t.Fatalf("expected 1 item after rollback, got %d", len(items))
		}
		if items[0].Name != "keeper" {
			t.Fatalf("expected keeper, got %s", items[0].Name)
		}
		return nil
	})
}

func TestWriteRollbackOnError(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		_, err := tx.Add(&entry{Name: "keeper"})
		return err
	})

	err := s.Write(func(tx *Tx[entry]) error {
		tx.Add(&entry{Name: "transient"})
		return fmt.Errorf("simulated error")
	})
	if err == nil {
		t.Fatal("expected error from Write")
	}

	s.Read(nil, func(items []entry) error {
		if len(items) != 1 {
			t.Fatalf("expected 1 item after error rollback, got %d", len(items))
		}
		if items[0].Name != "keeper" {
			t.Fatalf("expected keeper, got %s", items[0].Name)
		}
		return nil
	})
}

func TestConcurrentReads(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		_, err := tx.Add(&entry{Name: "shared"})
		return err
	})

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Read(nil, func(items []entry) error {
				if len(items) != 1 {
					t.Errorf("expected 1 item, got %d", len(items))
				}
				return nil
			})
		}()
	}
	wg.Wait()
}

func TestReadDuringWriteSerializes(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		_, err := tx.Add(&entry{Name: "initial"})
		return err
	})

	writerStarted := make(chan struct{})
	writerDone := make(chan struct{})

	go func() {
		s.Write(func(tx *Tx[entry]) error {
			close(writerStarted)
			time.Sleep(100 * time.Millisecond)
			_, err := tx.Add(&entry{Name: "added-during-write"})
			return err
		})
		close(writerDone)
	}()

	<-writerStarted
	time.Sleep(10 * time.Millisecond)

	<-writerDone
	s.Read(nil, func(items []entry) error {
		if len(items) != 2 {
			t.Fatalf("expected 2 items after write, got %d", len(items))
		}
		return nil
	})
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s1 := Open[entry](dir)
	var id1, id2 uuid.UUID
	s1.Write(func(tx *Tx[entry]) error {
		var err error
		id1, err = tx.Add(&entry{Name: "alice", Age: 30})
		if err != nil {
			return err
		}
		id2, err = tx.Add(&entry{Name: "bob", Age: 25})
		return err
	})

	s2 := Open[entry](dir)
	s2.Read(nil, func(items []entry) error {
		if len(items) != 2 {
			t.Fatalf("expected 2 items on reopen, got %d", len(items))
		}
		return nil
	})
	s2.Write(func(tx *Tx[entry]) error {
		a := tx.Get(id1)
		if a == nil || a.Name != "alice" {
			t.Fatalf("expected alice, got %+v", a)
		}
		b := tx.Get(id2)
		if b == nil || b.Name != "bob" {
			t.Fatalf("expected bob, got %+v", b)
		}
		return nil
	})
}

func TestReadFiltered(t *testing.T) {
	s := Open[entry](t.TempDir())
	s.Write(func(tx *Tx[entry]) error {
		for _, e := range []entry{
			{Name: "alice", Age: 30},
			{Name: "bob", Age: 25},
			{Name: "carol", Age: 30},
		} {
			if _, err := tx.Add(&e); err != nil {
				return err
			}
		}
		return nil
	})
	s.Read(func(e entry) bool { return e.Age == 30 }, func(items []entry) error {
		if len(items) != 2 {
			t.Fatalf("expected 2 filtered items, got %d", len(items))
		}
		for _, e := range items {
			if e.Age != 30 {
				t.Fatalf("unexpected item in filtered result: %+v", e)
			}
		}
		return nil
	})
}

type noIDEntry struct {
	Value string `json:"value"`
}

func TestNoIDField(t *testing.T) {
	s := Open[noIDEntry](t.TempDir())
	var id uuid.UUID
	s.Write(func(tx *Tx[noIDEntry]) error {
		var err error
		id, err = tx.Add(&noIDEntry{Value: "hello"})
		return err
	})
	if id == (uuid.UUID{}) {
		t.Fatal("expected non-zero UUID")
	}
	s.Write(func(tx *Tx[noIDEntry]) error {
		got := tx.Get(id)
		if got == nil {
			t.Fatal("Get returned nil")
		}
		if got.Value != "hello" {
			t.Fatalf("expected hello, got %s", got.Value)
		}
		return nil
	})
}
