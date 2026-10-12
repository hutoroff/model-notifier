package state

import (
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(p)
	if err != nil || s.Has("a") {
		t.Fatalf("empty load: %v %v", err, s.Has("a"))
	}
	s.Add("a", "k1")
	s.Mark("b")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Seen("a", "k1") || s2.Seen("a", "k2") || !s2.Has("b") || s2.Has("c") {
		t.Fatal("round-trip mismatch")
	}
}
