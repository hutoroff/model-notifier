package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsOnlyBraced(t *testing.T) {
	t.Setenv("TG_TOKEN_T", "secret")
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte(`notifiers: [{name: tg, type: telegram, token: "${TG_TOKEN_T}", chat_id: "1"}]
providers: [{name: a, sources: [{name: s, type: github_releases, url: u, template: t, filter: "v1$"}]}]
`), 0o600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Notifiers[0].Token != "secret" || c.Providers[0].Sources[0].Filter != "v1$" {
		t.Fatalf("got token %q filter %q", c.Notifiers[0].Token, c.Providers[0].Sources[0].Filter)
	}
}
