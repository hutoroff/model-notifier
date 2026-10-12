// Package source fetches release items from provider data sources.
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	ra := resp.Header.Get("Retry-After")
	switch {
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode == http.StatusServiceUnavailable,
		resp.StatusCode == http.StatusForbidden && ra != "":
		return nil, &RateLimitedError{URL: url, Status: resp.Status, Until: time.Now().Add(retryAfter(ra, time.Now()))}
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}

// RateLimitedError means the server asked us to back off: 429/503, or 403 with Retry-After.
// Until is when to try again; it equals the time of the response when no usable Retry-After was sent.
type RateLimitedError struct {
	URL    string
	Status string
	Until  time.Time
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("GET %s: %s, retry after %s", e.URL, e.Status, e.Until.Format(time.RFC3339))
}

// retryAfter parses a Retry-After value (seconds or HTTP date). Missing or bad → 0.
// ponytail: capped at 24h so a broken header can't silence a source for good.
func retryAfter(h string, now time.Time) time.Duration {
	var d time.Duration
	if n, err := strconv.Atoi(h); err == nil {
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = t.Sub(now)
	}
	return min(max(d, 0), 24*time.Hour)
}
