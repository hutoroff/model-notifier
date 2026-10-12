// Package source fetches release items from provider data sources.
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hutoroff/model-notifier/internal/config"
)

// Item is one thing a source can announce. Key is stable and unique within the source.
type Item struct {
	Key    string
	Link   string
	Fields map[string]string
}

type Source interface {
	Fetch(ctx context.Context) ([]Item, error)
}

// New builds a Source from config. cfg has already been validated.
func New(cfg config.Source) (Source, error) {
	switch cfg.Type {
	case "webpage":
		return newWebpage(cfg)
	case "github_releases":
		return newGitHub(cfg)
	}
	return nil, fmt.Errorf("unknown source type %q", cfg.Type)
}

var client = &http.Client{Timeout: 30 * time.Second}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "model-notifier/1.0 (+https://github.com/hutoroff/model-notifier)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}
