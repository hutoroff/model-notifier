package monitor

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hutoroff/model-notifier/internal/notify"
	"github.com/hutoroff/model-notifier/internal/source"
	"github.com/hutoroff/model-notifier/internal/state"
)

type fakeSource struct{ items []source.Item }

func (f *fakeSource) Fetch(context.Context) ([]source.Item, error) { return f.items, nil }

func TestCheck(t *testing.T) {
	var mu sync.Mutex
	var reqs []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTOKEN/sendMessage" {
			t.Errorf("path %s", r.URL.Path)
		}
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		reqs = append(reqs, m)
		mu.Unlock()
	}))
	defer srv.Close()

	st, err := state.Load(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{items: []source.Item{{Key: "old"}}}
	e := &Entry{ID: "p/s", Src: src, Tmpl: template.Must(template.New("t").Parse("New: <b>{{.Key}}</b>"))}
	m := &Monitor{
		sources:   []*Entry{e},
		notifiers: []notify.Notifier{&notify.Telegram{Token: "TOKEN", ChatID: "42", BaseURL: srv.URL}},
		state:     st,
	}

	if n, err := m.Check(context.Background(), e); err != nil || n != 0 || len(reqs) != 0 {
		t.Fatalf("seed: n=%d err=%v reqs=%d", n, err, len(reqs))
	}
	src.items = append(src.items, source.Item{Key: "a<b"})
	if n, err := m.Check(context.Background(), e); err != nil || n != 1 {
		t.Fatalf("second: n=%d err=%v", n, err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r["chat_id"] != "42" || r["parse_mode"] != "HTML" {
		t.Fatalf("bad request %v", r)
	}
	if text, _ := r["text"].(string); !strings.Contains(text, "<b>a&lt;b</b>") {
		t.Fatalf("text not escaped: %q", text)
	}
	if n, _ := m.Check(context.Background(), e); n != 0 || len(reqs) != 1 {
		t.Fatal("resent seen item")
	}
}

func TestSeedEmpty(t *testing.T) {
	st, _ := state.Load(filepath.Join(t.TempDir(), "s.json"))
	e := &Entry{ID: "p/s", Src: &fakeSource{}, Tmpl: template.Must(template.New("t").Parse("{{.Key}}"))}
	m := &Monitor{sources: []*Entry{e}, state: st}
	if _, err := m.Check(context.Background(), e); err != nil || !st.Has("p/s") {
		t.Fatalf("empty seed not recorded: %v", err)
	}
}
