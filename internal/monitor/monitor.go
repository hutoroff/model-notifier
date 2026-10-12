// Package monitor polls sources and notifies about new items.
package monitor

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hutoroff/model-notifier/internal/config"
	"github.com/hutoroff/model-notifier/internal/notify"
	"github.com/hutoroff/model-notifier/internal/source"
	"github.com/hutoroff/model-notifier/internal/state"
)

type Entry struct {
	ID       string // provider/source
	Src      source.Source
	Tmpl     *template.Template // data = source.Item, or []source.Item when Digest
	Interval time.Duration
	Digest   bool // report Value changes per Key, all of one check in one message

	mu          sync.Mutex // serializes ticker and /run checks
	pausedUntil time.Time  // set from a source's Retry-After; guarded by mu
}

type namedNotifier struct {
	name string
	notify.Notifier
}

type Monitor struct {
	sources      []*Entry
	notifiers    []namedNotifier
	state        *state.State
	NotifyOnSeed bool
}

func New(cfg *config.Config, st *state.State, notifyOnSeed bool) (*Monitor, error) {
	m := &Monitor{state: st, NotifyOnSeed: notifyOnSeed}
	for _, p := range cfg.Providers {
		for _, sc := range p.Sources {
			id := p.Name + "/" + sc.Name
			src, err := source.New(sc)
			if err != nil {
				return nil, fmt.Errorf("source %s: %w", id, err)
			}
			tmpl, err := template.New(id).Parse(sc.Template)
			if err != nil {
				return nil, fmt.Errorf("source %s: template: %w", id, err)
			}
			iv := sc.Interval
			if iv == 0 {
				iv = cfg.Interval
			}
			m.sources = append(m.sources, &Entry{ID: id, Src: src, Tmpl: tmpl, Interval: iv, Digest: sc.Value != ""})
		}
	}
	for _, nc := range cfg.Notifiers {
		n, err := notify.New(nc)
		if err != nil {
			return nil, fmt.Errorf("notifier %s: %w", nc.Name, err)
		}
		m.notifiers = append(m.notifiers, namedNotifier{nc.Name, n})
	}
	return m, nil
}

// maxMsg keeps a digest under Telegram's 4096-character message limit.
const maxMsg = 4000

// pending is an item to announce. key is its state key ("<Key>\t<Value>" for digests);
// prev is the state key it replaces, if any.
type pending struct {
	it        source.Item
	key, prev string
}

// Check fetches e once and notifies about unseen items. A failed send is retried next
// check for that notifier only; deliveries so far are kept under "<source>@<notifier>".
// Digest sources report changed values only: a Key seen for the first time is recorded silently.
func (m *Monitor) Check(ctx context.Context, e *Entry) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Now().Before(e.pausedUntil) {
		return 0, fmt.Errorf("rate-limited until %s", e.pausedUntil.Format(time.RFC3339))
	}
	items, err := e.Src.Fetch(ctx)
	if err != nil {
		if rl, ok := errors.AsType[*source.RateLimitedError](err); ok {
			e.pausedUntil = rl.Until
		}
		return 0, err
	}
	seeding := !m.state.Has(e.ID)
	var todo []pending
	for _, it := range items {
		p := pending{it: it, key: it.Key}
		if e.Digest {
			p.key = it.Key + "\t" + it.Value
		}
		if m.state.Seen(e.ID, p.key) {
			continue
		}
		if e.Digest {
			old, ok := m.state.Find(e.ID, it.Key+"\t")
			if !ok {
				m.state.Add(e.ID, p.key)
				continue
			}
			p.it.Old, p.prev = old, it.Key+"\t"+old
		}
		if seeding && !m.NotifyOnSeed {
			m.state.Add(e.ID, p.key)
			continue
		}
		todo = append(todo, p)
	}
	sent, failed := 0, 0
	for _, batch := range m.batches(e, todo) {
		if !m.deliver(ctx, e, batch) {
			failed += len(batch)
			continue
		}
		for _, p := range batch {
			m.state.Add(e.ID, p.key)
			if p.prev != "" {
				m.state.Delete(e.ID, p.prev)
			}
			for _, n := range m.notifiers {
				m.state.Delete(e.ID+"@"+n.name, p.key)
			}
			sent++
			slog.Info("notified", "source", e.ID, "key", p.it.Key)
		}
	}
	m.state.Mark(e.ID) // seeding with zero items still counts as seeded
	if seeding {
		slog.Info("seeded", "source", e.ID, "items", len(items))
	}
	if err := m.state.Save(); err != nil {
		return sent, fmt.Errorf("save state: %w", err)
	}
	if failed > 0 {
		return sent, fmt.Errorf("%d item(s) failed, will retry", failed)
	}
	return sent, nil
}

// batches groups items into messages: one per item, or for digests as many items as fit in maxMsg.
func (m *Monitor) batches(e *Entry, todo []pending) [][]pending {
	var out [][]pending
	for _, p := range todo {
		if e.Digest && len(out) > 0 {
			last := out[len(out)-1]
			if s, err := render(e, append(last[:len(last):len(last)], p)); err == nil && len(s) <= maxMsg {
				out[len(out)-1] = append(last, p)
				continue
			}
		}
		out = append(out, []pending{p})
	}
	return out
}

// deliver sends batch to every notifier that hasn't got it yet and records per-notifier progress.
// ponytail: if a value changes again while a delivery is pending, the old "@notifier" key stays behind.
func (m *Monitor) deliver(ctx context.Context, e *Entry, batch []pending) bool {
	ok := true
	for _, n := range m.notifiers {
		sub := e.ID + "@" + n.name
		var rest []pending
		for _, p := range batch {
			if !m.state.Seen(sub, p.key) {
				rest = append(rest, p)
			}
		}
		if len(rest) == 0 {
			continue
		}
		text, err := render(e, rest)
		if err != nil {
			slog.Error("render", "source", e.ID, "key", rest[0].it.Key, "err", err)
			return false
		}
		if err := n.Send(ctx, text); err != nil {
			slog.Error("send", "source", e.ID, "key", rest[0].it.Key, "items", len(rest), "notifier", n.name, "err", err)
			ok = false
			continue
		}
		for _, p := range rest {
			m.state.Add(sub, p.key)
		}
	}
	return ok
}

// render executes e's template with the batch's items: a slice for digests, else the single item.
func render(e *Entry, batch []pending) (string, error) {
	var data any = batch[0].it
	if e.Digest {
		items := make([]source.Item, len(batch))
		for i, p := range batch {
			items[i] = p.it
		}
		data = items
	}
	var b strings.Builder
	err := e.Tmpl.Execute(&b, data)
	return b.String(), err
}

// RunAll checks every source once and returns one summary line per source.
func (m *Monitor) RunAll(ctx context.Context) (string, bool) {
	var b strings.Builder
	allOK := true
	for _, e := range m.sources {
		n, err := m.Check(ctx, e)
		if err != nil {
			allOK = false
			fmt.Fprintf(&b, "%s: %d new, error: %v\n", e.ID, n, err)
			continue
		}
		fmt.Fprintf(&b, "%s: %d new\n", e.ID, n)
	}
	return b.String(), allOK
}

// Run checks each source immediately and then on its interval until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, e := range m.sources {
		wg.Go(func() {
			t := time.NewTicker(e.Interval)
			defer t.Stop()
			for {
				if n, err := m.Check(ctx, e); err != nil {
					slog.Error("check", "source", e.ID, "err", err)
				} else {
					slog.Info("check", "source", e.ID, "new", n)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		})
	}
	wg.Wait()
}
