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

func TestGoogleChangelog(t *testing.T) {
	items := parseWeb(t, "google/changelog", "google-changelog.html")
	k := keys(items)
	if len(k) != 87 {
		t.Errorf("got %d models, want 87: %v", len(k), k)
	}
	if it := items[0]; it.Key != "gemini-3.7-flash" || it.Fields["date"] != "October 8, 2026" || it.Fields["anchor"] != "10-08-2026" {
		t.Errorf("first = %+v", it)
	}
	seen := map[string]bool{}
	for _, key := range k {
		if seen[key] {
			t.Errorf("duplicate %s", key)
		}
		seen[key] = true
		if !strings.HasPrefix(key, "gemini-") || strings.Contains(key, "-latest") {
			t.Errorf("unexpected key %s", key)
		}
	}
	for _, want := range []string{"gemini-3.8-flash", "gemini-nano-banana-2.1", "gemini-omni-1.1-flash"} {
		if !slices.Contains(k, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestGoogleSDK(t *testing.T) {
	sc := cfg(t, "google/python-sdk-releases")
	items := parseFeed(t, sc, "google-releases.atom")
	// fixture has three model-adding releases: v2.28.0 (omni), v2.26.0 (3.8 Flash TTS), v2.22.0 (3.8 Flash)
	var titles []string
	for _, it := range items {
		titles = append(titles, it.Fields["title"])
	}
	if got := strings.Join(titles, ","); got != "v2.28.0,v2.26.0,v2.22.0" {
		t.Fatalf("titles = %s", got)
	}
	if it := items[0]; !strings.Contains(it.Fields["matches"], "gemini-omni-1.1-flash") || !strings.HasSuffix(it.Link, "/releases/tag/v2.28.0") {
		t.Errorf("unexpected item %+v", it)
	}
	re := regexp.MustCompile(sc.Filter)
	// Real release lines (as text, from python-genai history).
	for line, want := range map[string]bool{
		"Add gemini-3.7-flash (f898b61)":                       true,
		"Add the Gemini Robotics ER 2 Preview model (61d4645)": true,
		"Add lyria-3.5, gemini-omni-1.1-flash, and gemini-omni-flash-preview to Interactions Model enum (1e8c0cc)": true,
		"Add Tool.exa_ai_search for Gemini Enterprise API (a1b2c3d)":                                               false,
		"Add image_size field for Gemini Developer API Imagen 4 generation (a1b2c3d)":                              false,
		"Update code samples to gemini-3.8-flash-tts (5673494)":                                                    false,
		"Add missing fields to the model types (4b855e6)":                                                          false,
	} {
		if got := re.MatchString(line); got != want {
			t.Errorf("%q: match = %v, want %v", line, got, want)
		}
	}
	sc.Filter = ""
	if n := len(parseFeed(t, sc, "google-releases.atom")); n != 10 {
		t.Errorf("no filter: got %d items, want 10", n)
	}
}

func TestXAIAPI(t *testing.T) {
	items := parseWeb(t, "xai/api", "xai-api.html")
	want := "grok-4.7,grok-imagine-image-2.0,grok-imagine-video-1.5,grok-imagine-video-1.5-lite,grok-imagine-image-quality,grok-imagine-image,grok-imagine-video"
	if got := strings.Join(keys(items), ","); got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if got := items[2].Fields["about"]; got != "Text, Image, Audio → Video" {
		t.Errorf("about = %q", got)
	}
}

func TestXAISDK(t *testing.T) {
	sc := cfg(t, "xai/python-sdk-releases")
	items := parseFeed(t, sc, "xai-releases.atom")
	// fixture has three model-adding releases: v1.18.0 (grok-4.6), v1.16.0 (imagine), v1.15.0 (grok-build-0.1)
	var titles []string
	for _, it := range items {
		titles = append(titles, it.Fields["title"])
	}
	if got := strings.Join(titles, ","); got != "v1.18.0,v1.16.0,v1.15.0" {
		t.Fatalf("titles = %s", got)
	}
	if it := items[0]; it.Fields["matches"] != "add grok-4.6 to ChatModel" || !strings.HasSuffix(it.Link, "/releases/tag/v1.18.0") {
		t.Errorf("unexpected item %+v", it)
	}
	re := regexp.MustCompile(sc.Filter)
	// Real release lines (as text, from xai-sdk-python history).
	for line, want := range map[string]bool{
		"Add grok-imagine-image-pro to ImageGenerationModel by @x in https://github.com/xai-org/xai-sdk-python/pull/103":           true,
		"Update grok-4.20 and multi-agent model variants in ChatModel by @x in https://github.com/xai-org/xai-sdk-python/pull/117": true,
		"Update README examples to use grok-4.6 by @x in https://github.com/xai-org/xai-sdk-python/pull/191":                       false,
		"Add agent_count parameter to chat API for multi-agent models by @x in https://github.com/xai-org/xai-sdk-python/pull/104": false,
		"Use model type aliases in image and video method signatures by @x in https://github.com/xai-org/xai-sdk-python/pull/114":  false,
		"Add `cost_usd` tracking, model migration, example fixes by @x in https://github.com/xai-org/xai-sdk-python/pull/132":      false,
	} {
		if got := re.MatchString(line); got != want {
			t.Errorf("%q: match = %v, want %v", line, got, want)
		}
	}
	sc.Filter = ""
	if n := len(parseFeed(t, sc, "xai-releases.atom")); n != 10 {
		t.Errorf("no filter: got %d items, want 10", n)
	}
}

func TestBadXPath(t *testing.T) {
	if _, err := newWebpage(config.Source{Items: "//["}); err == nil {
		t.Error("want error")
	}
}
