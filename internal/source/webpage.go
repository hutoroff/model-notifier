package source

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"golang.org/x/net/html"

	"github.com/hutoroff/model-notifier/internal/config"
)

type webpage struct {
	url    string
	items  *xpath.Expr
	fields map[string]*xpath.Expr
}

func newWebpage(cfg config.Source) (Source, error) {
	w := &webpage{url: cfg.URL, fields: map[string]*xpath.Expr{}}
	var err error
	if w.items, err = xpath.Compile(cfg.Items); err != nil {
		return nil, fmt.Errorf("items xpath %q: %w", cfg.Items, err)
	}
	for name, expr := range cfg.Fields {
		if w.fields[name], err = xpath.Compile(expr); err != nil {
			return nil, fmt.Errorf("field %q xpath %q: %w", name, expr, err)
		}
	}
	return w, nil
}

func (w *webpage) Fetch(ctx context.Context) ([]Item, error) {
	body, err := get(ctx, w.url)
	if err != nil {
		return nil, err
	}
	return w.parse(body)
}

func (w *webpage) parse(body []byte) ([]Item, error) {
	doc, err := htmlquery.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var nodes []*html.Node
	var items []Item
	for _, n := range htmlquery.QuerySelectorAll(doc, w.items) {
		if k := text(n); k != "" {
			nodes = append(nodes, n)
			items = append(items, Item{Key: k, Fields: map[string]string{}})
		}
	}
	for name, expr := range w.fields {
		var global []*html.Node
		relative := strings.HasPrefix(expr.String(), ".")
		if !relative {
			global = htmlquery.QuerySelectorAll(doc, expr)
		}
		for i := range items {
			var v string
			if relative {
				if m := htmlquery.QuerySelector(nodes[i], expr); m != nil {
					v = text(m)
				}
			} else if i < len(global) {
				v = text(global[i])
			}
			items[i].Fields[name] = v
			if name == "link" && v != "" {
				items[i].Link = w.resolve(v)
			}
		}
	}
	return items, nil
}

func (w *webpage) resolve(ref string) string {
	base, err := url.Parse(w.url)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(r).String()
}

// text returns n's inner text with whitespace collapsed. Works for attribute nodes too.
func text(n *html.Node) string {
	return strings.Join(strings.Fields(htmlquery.InnerText(n)), " ")
}
