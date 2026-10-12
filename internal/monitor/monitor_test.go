package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
		notifiers: []namedNotifier{{"tg", &notify.Telegram{Token: "TOKEN", ChatID: "42", BaseURL: srv.URL}}},
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

type fakeNotifier struct {
	fail bool
	sent int
	msgs []string
}

func (f *fakeNotifier) Send(_ context.Context, text string) error {
	if f.fail {
		return errors.New("down")
	}
	f.sent++
	f.msgs = append(f.msgs, text)
	return nil
}

func TestPartialFailureRetriesOnlyFailedNotifier(t *testing.T) {
	st, _ := state.Load(filepath.Join(t.TempDir(), "s.json"))
	st.Mark("p/s")
	e := &Entry{ID: "p/s", Src: &fakeSource{items: []source.Item{{Key: "k"}}}, Tmpl: template.Must(template.New("t").Parse("{{.Key}}"))}
	a, b := &fakeNotifier{}, &fakeNotifier{fail: true}
	m := &Monitor{sources: []*Entry{e}, notifiers: []namedNotifier{{"a", a}, {"b", b}}, state: st}
	if _, err := m.Check(context.Background(), e); err == nil || a.sent != 1 || st.Seen("p/s", "k") {
		t.Fatalf("first: err=%v a=%d", err, a.sent)
	}
	b.fail = false
	if n, err := m.Check(context.Background(), e); err != nil || n != 1 || a.sent != 1 || b.sent != 1 {
		t.Fatalf("retry: n=%d err=%v a=%d b=%d", n, err, a.sent, b.sent)
	}
	if !st.Seen("p/s", "k") || st.Has("p/s@a") || st.Has("p/s@b") {
		t.Fatal("state not settled")
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

type limitedSource struct{ calls int }

func (l *limitedSource) Fetch(context.Context) ([]source.Item, error) {
	l.calls++
	return nil, &source.RateLimitedError{URL: "u", Status: "429", Until: time.Now().Add(time.Hour)}
}

func TestRateLimitPause(t *testing.T) {
	st, _ := state.Load(filepath.Join(t.TempDir(), "s.json"))
	src := &limitedSource{}
	e := &Entry{ID: "p/s", Src: src, Tmpl: template.Must(template.New("t").Parse("{{.Key}}"))}
	m := &Monitor{sources: []*Entry{e}, state: st}
	if _, err := m.Check(context.Background(), e); err == nil || src.calls != 1 {
		t.Fatalf("first: err=%v calls=%d", err, src.calls)
	}
	if _, err := m.Check(context.Background(), e); err == nil || !strings.Contains(err.Error(), "rate-limited") || src.calls != 1 {
		t.Fatalf("paused: err=%v calls=%d", err, src.calls)
	}
	e.pausedUntil = time.Now().Add(-time.Second)
	if m.Check(context.Background(), e); src.calls != 2 {
		t.Fatalf("after pause: calls=%d", src.calls)
	}
}

func TestDigest(t *testing.T) {
	st, _ := state.Load(filepath.Join(t.TempDir(), "s.json"))
	src := &fakeSource{items: []source.Item{{Key: "a", Value: "$1"}, {Key: "b", Value: "$2"}, {Key: "c", Value: "$3"}}}
	e := &Entry{ID: "p/s", Src: src, Digest: true,
		Tmpl: template.Must(template.New("t").Parse("{{range .}}{{.Key}}: {{.Old}} → {{.Value}}\n{{end}}"))}
	a, b := &fakeNotifier{}, &fakeNotifier{}
	m := &Monitor{sources: []*Entry{e}, notifiers: []namedNotifier{{"a", a}, {"b", b}}, state: st, NotifyOnSeed: true}
	check := func(wantN int) {
		t.Helper()
		if n, err := m.Check(context.Background(), e); err != nil || n != wantN {
			t.Fatalf("n=%d err=%v, want %d", n, err, wantN)
		}
	}

	check(0) // first sight of every key is silent, even with NotifyOnSeed
	src.items = []source.Item{{Key: "a", Value: "$1.5"}, {Key: "b", Value: "$2"}, {Key: "c", Value: "$4"}, {Key: "d", Value: "$9"}}
	check(2) // a and c changed; d is new → silent
	if len(a.msgs) != 1 || a.msgs[0] != "a: $1 → $1.5\nc: $3 → $4\n" {
		t.Fatalf("msgs = %q", a.msgs)
	}
	if !st.Seen("p/s", "a\t$1.5") || st.Seen("p/s", "a\t$1") || !st.Seen("p/s", "d\t$9") {
		t.Fatal("state not replaced")
	}
	check(0)

	// One notifier down: only it gets the retry.
	src.items[0].Value = "$2.5"
	b.fail = true
	if _, err := m.Check(context.Background(), e); err == nil || len(a.msgs) != 2 {
		t.Fatalf("partial: err=%v a=%d", err, len(a.msgs))
	}
	b.fail = false
	check(1)
	if len(a.msgs) != 2 || len(b.msgs) != 2 || b.msgs[1] != "a: $1.5 → $2.5\n" || st.Has("p/s@a") || st.Has("p/s@b") {
		t.Fatalf("retry: a=%q b=%q", a.msgs, b.msgs)
	}

	// Over maxMsg → split into several messages.
	big := strings.Repeat("x", maxMsg/3)
	for i := range src.items {
		src.items[i].Value += big
	}
	check(4)
	if len(a.msgs) != 4 { // 2 earlier + 2 batches
		t.Fatalf("want 2 batches, got %d msgs", len(a.msgs)-2)
	}
}
