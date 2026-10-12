package source

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hutoroff/model-notifier/internal/config"
)

// cfg returns the source "provider/name" from the real config.yaml, so fixtures test production rules.
func cfg(t *testing.T, id string) config.Source {
	t.Helper()
	t.Setenv("TG_BOT_TOKEN", "x")
	t.Setenv("TG_CHAT_ID", "y")
	c, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.Providers {
		for _, s := range p.Sources {
			if p.Name+"/"+s.Name == id {
				return s
			}
		}
	}
	t.Fatalf("source %s not in config.yaml", id)
	return config.Source{}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func parseWeb(t *testing.T, id, file string) []Item {
	t.Helper()
	s, err := newWebpage(cfg(t, id))
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.(*webpage).parse(fixture(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func parseFeed(t *testing.T, sc config.Source, file string) []Item {
	t.Helper()
	s, err := newGitHub(sc)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.(*github).parse(fixture(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func keys(items []Item) []string {
	var k []string
	for _, it := range items {
		k = append(k, it.Key)
	}
	return k
}

func TestAnthropicModels(t *testing.T) {
	items := parseWeb(t, "anthropic/models-overview", "anthropic-models-overview.html")
	if got, want := strings.Join(keys(items), ","), "claude-fable-5-1,claude-opus-5-5,claude-sonnet-5-5,claude-haiku-5-5"; got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if got := items[0].Fields["name"]; got != "Claude Fable 5.1" {
		t.Errorf("name = %q", got)
	}
	if got := items[0].Link; got != "https://platform.claude.com/docs/en/models/fable-5-1/overview" {
		t.Errorf("link = %q", got)
	}
}

func TestAnthropicSDK(t *testing.T) {
	sc := cfg(t, "anthropic/python-sdk-releases")
	items := parseFeed(t, sc, "anthropic-releases.atom")
	// fixture has three model-adding releases: v1.12.0 (haiku), v1.9.0 (sonnet), v1.8.0 (opus)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	it := items[0]
	if it.Fields["title"] != "v1.12.0" || !strings.Contains(it.Fields["matches"], "claude-haiku-5-5") ||
		!strings.HasSuffix(it.Link, "/releases/tag/v1.12.0") {
		t.Errorf("unexpected item %+v", it)
	}
	sc.Filter = ""
	if n := len(parseFeed(t, sc, "anthropic-releases.atom")); n != 10 {
		t.Errorf("no filter: got %d items, want 10", n)
	}
}

func TestOpenAIChangelog(t *testing.T) {
	items := parseWeb(t, "openai/changelog", "openai-changelog.html")
	k := keys(items)
	if len(k) != 39 {
		t.Errorf("got %d models, want 39: %v", len(k), k)
	}
	if items[0].Key != "gpt-6.1-sol" || items[0].Fields["date"] != "Oct 8" {
		t.Errorf("first = %+v", items[0])
	}
	seen := map[string]bool{}
	for _, key := range k {
		if seen[key] {
			t.Errorf("duplicate %s", key)
		}
		seen[key] = true
		if !regexp.MustCompile(`^gpt-\d`).MatchString(key) {
			t.Errorf("non gpt-N key %s", key)
		}
	}
	for _, want := range []string{"gpt-6-sol", "gpt-6-astra", "gpt-5.5"} {
		if !slices.Contains(k, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestOpenAISDK(t *testing.T) {
	sc := cfg(t, "openai/python-sdk-releases")
	if n := len(parseFeed(t, sc, "openai-releases.atom")); n != 0 {
		t.Errorf("fixture has no model releases, got %d", n)
	}
	re := regexp.MustCompile(sc.Filter)
	// Real release lines (as text, from openai-python history).
	for line, want := range map[string]bool{
		"Features api: add GPT-6.1 Sol model identifier (#3986) (a1b2c3d)":                    true,
		"api: 5.4 nano and mini model slugs (0f1e2d3)":                                        true,
		"api: Add gpt-5.5 and tool name/namespace to Responses types (#3569)":                 true,
		"api: add agent environment suspension and expiration (#4010)":                        false,
		"api: add GPT Image 2.5 models and image options (#3824)":                             false,
		"Chores internal: bump gpt-6 test fixtures":                                           false,
		"api: add billing_not_active session error code (#4020) api: add shida voice (#4021)": false,
	} {
		if got := re.MatchString(line); got != want {
			t.Errorf("%q: match = %v, want %v", line, got, want)
		}
	}
	sc.Filter = ""
	if n := len(parseFeed(t, sc, "openai-releases.atom")); n != 10 {
		t.Errorf("no filter: got %d items, want 10", n)
	}
}

func TestBadXPath(t *testing.T) {
	if _, err := newWebpage(config.Source{Items: "//["}); err == nil {
		t.Error("want error")
	}
}
