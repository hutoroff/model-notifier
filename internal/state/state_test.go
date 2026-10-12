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

func TestDelete(t *testing.T) {
	s, _ := Load(filepath.Join(t.TempDir(), "state.json"))
	s.Add("a", "k1")
	s.Add("a", "k2")
	s.Delete("a", "k1")
	if s.Seen("a", "k1") || !s.Seen("a", "k2") {
		t.Fatal("delete k1")
	}
	s.Delete("a", "k2")
	if s.Has("a") {
		t.Fatal("empty source kept")
	}
}

func TestFind(t *testing.T) {
	s, _ := Load(filepath.Join(t.TempDir(), "s.json"))
	s.Add("src", "k\tv1")
	if v, ok := s.Find("src", "k\t"); !ok || v != "v1" {
		t.Fatalf("Find = %q, %v", v, ok)
	}
	if _, ok := s.Find("src", "x\t"); ok {
		t.Fatal("found missing prefix")
	}
}
