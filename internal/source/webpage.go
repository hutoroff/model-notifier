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
	url        string
	items      *xpath.Expr
	key, value *xpath.Expr // optional, relative to the item node
	fields     map[string]*xpath.Expr
}

func newWebpage(cfg config.Source) (Source, error) {
	w := &webpage{url: cfg.URL, fields: map[string]*xpath.Expr{}}
	var err error
	if w.items, err = xpath.Compile(cfg.Items); err != nil {
		return nil, fmt.Errorf("items xpath %q: %w", cfg.Items, err)
	}
	for _, x := range []struct {
		dst  **xpath.Expr
		expr string
	}{{&w.key, cfg.Key}, {&w.value, cfg.Value}} {
		if x.expr == "" {
			continue
		}
		if *x.dst, err = xpath.Compile(x.expr); err != nil {
			return nil, fmt.Errorf("xpath %q: %w", x.expr, err)
		}
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
		k := text(n)
		if w.key != nil {
			k = eval(n, w.key)
		}
		if k == "" {
			continue
		}
		it := Item{Key: k, Fields: map[string]string{}}
		if w.value != nil {
			it.Value = eval(n, w.value)
		}
		nodes = append(nodes, n)
		items = append(items, it)
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
	// Keep the first item per key; done after fields so index-zipped fields stay aligned.
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		if !seen[it.Key] {
			seen[it.Key] = true
			out = append(out, it)
		}
	}
	return out, nil
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

// text returns n's inner text with whitespace collapsed; <br> counts as a space. Works for attribute nodes too.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.CommentNode:
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteByte(' ')
		default:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// eval evaluates e relative to n: a node-set gives each non-empty node text joined with " · ",
// any other result (concat(), count(), …) its string form.
func eval(n *html.Node, e *xpath.Expr) string {
	r := e.Evaluate(htmlquery.CreateXPathNavigator(n))
	it, ok := r.(*xpath.NodeIterator)
	if !ok {
		return strings.Join(strings.Fields(fmt.Sprint(r)), " ")
	}
	var parts []string
	for it.MoveNext() {
		nav := it.Current().(*htmlquery.NodeNavigator)
		v := text(nav.Current())
		if nav.NodeType() == xpath.AttributeNode {
			v = strings.Join(strings.Fields(nav.Value()), " ")
		}
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}
