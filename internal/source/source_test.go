package source

import (
	"os"
	"strings"
	"testing"

	"github.com/hutoroff/model-notifier/internal/config"
)

func TestWebpage(t *testing.T) {
	s, err := newWebpage(config.Source{
		URL:   "https://platform.claude.com/docs/en/models/overview",
		Items: "//table//tr[th[contains(., 'Claude API ID')]]/td",
		Fields: map[string]string{
			"name": "//table/thead//th//a",
			"link": "//table/thead//th//a/@href",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("testdata/models-overview.html")
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.(*webpage).parse(body)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	if got, want := strings.Join(keys, ","), "claude-fable-5-1,claude-opus-5-5,claude-sonnet-5-5,claude-haiku-5-5"; got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if got := items[0].Fields["name"]; got != "Claude Fable 5.1" {
		t.Errorf("name = %q", got)
	}
	if got := items[0].Link; got != "https://platform.claude.com/docs/en/models/fable-5-1/overview" {
		t.Errorf("link = %q", got)
	}
}

func TestBadXPath(t *testing.T) {
	if _, err := newWebpage(config.Source{Items: "//["}); err == nil {
		t.Error("want error")
	}
}

func TestGitHub(t *testing.T) {
	body, err := os.ReadFile("testdata/releases.atom")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newGitHub(config.Source{Filter: `claude-[a-z]+-\d[\d-]*`})
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.(*github).parse(body)
	if err != nil {
		t.Fatal(err)
	}
	// fixture has three model-adding releases: v1.12.0 (haiku), v1.9.0 (sonnet), v1.8.0 (opus)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	it := items[0]
	if it.Fields["title"] != "v1.12.0" || !strings.Contains(it.Fields["matches"], "claude-haiku-5-5") ||
		!strings.HasSuffix(it.Link, "/releases/tag/v1.12.0") {
		t.Errorf("unexpected item %+v", it)
	}

	s, _ = newGitHub(config.Source{})
	if items, _ = s.(*github).parse(body); len(items) != 10 {
		t.Errorf("no filter: got %d items, want 10", len(items))
	}
}
