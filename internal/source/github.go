package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/antchfx/htmlquery"

	"github.com/hutoroff/model-notifier/internal/config"
)

type github struct {
	url    string
	filter *regexp.Regexp
}

func newGitHub(cfg config.Source) (Source, error) {
	g := &github{url: cfg.URL}
	if cfg.Filter != "" {
		re, err := regexp.Compile(cfg.Filter)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", cfg.Filter, err)
		}
		g.filter = re
	}
	return g, nil
}

func (g *github) Fetch(ctx context.Context) ([]Item, error) {
	body, err := get(ctx, g.url)
	if err != nil {
		return nil, err
	}
	return g.parse(body)
}

type atomFeed struct {
	Entries []struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Content string `xml:"content"`
		Links   []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func (g *github) parse(body []byte) ([]Item, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range feed.Entries {
		content := e.Content
		if doc, err := htmlquery.Parse(strings.NewReader(e.Content)); err == nil {
			content = htmlquery.InnerText(doc)
		}
		content = strings.Join(strings.Fields(content), " ")
		it := Item{Key: e.ID, Fields: map[string]string{
			"title": e.Title, "updated": e.Updated, "content": content,
		}}
		for i, l := range e.Links {
			if i == 0 || l.Rel == "alternate" {
				it.Link = l.Href
			}
			if l.Rel == "alternate" {
				break
			}
		}
		if g.filter != nil {
			var uniq []string
			for _, m := range g.filter.FindAllString(e.Title+" "+content, -1) {
				if !slices.Contains(uniq, m) {
					uniq = append(uniq, m)
				}
			}
			if len(uniq) == 0 {
				continue
			}
			it.Fields["matches"] = strings.Join(uniq, ", ")
		}
		items = append(items, it)
	}
	return items, nil
}
