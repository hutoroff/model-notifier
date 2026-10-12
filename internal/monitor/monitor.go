// Package monitor polls sources and notifies about new items.
package monitor

import (
	"context"
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
	Tmpl     *template.Template // data = source.Item
	Interval time.Duration

	mu sync.Mutex // serializes ticker and /run checks
}

type Monitor struct {
	sources      []*Entry
	notifiers    []notify.Notifier
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
			m.sources = append(m.sources, &Entry{ID: id, Src: src, Tmpl: tmpl, Interval: iv})
		}
	}
	for _, nc := range cfg.Notifiers {
		n, err := notify.New(nc)
		if err != nil {
			return nil, fmt.Errorf("notifier %s: %w", nc.Name, err)
		}
		m.notifiers = append(m.notifiers, n)
	}
	return m, nil
}

// Check fetches e once and notifies about unseen items. Failed sends are retried next check.
func (m *Monitor) Check(ctx context.Context, e *Entry) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	items, err := e.Src.Fetch(ctx)
	if err != nil {
		return 0, err
	}
	seeding := !m.state.Has(e.ID)
	sent, failed := 0, 0
	for _, it := range items {
		if m.state.Seen(e.ID, it.Key) {
			continue
		}
		if seeding && !m.NotifyOnSeed {
			m.state.Add(e.ID, it.Key)
			continue
		}
		var b strings.Builder
		if err := e.Tmpl.Execute(&b, it); err != nil {
			slog.Error("render", "source", e.ID, "key", it.Key, "err", err)
			failed++
			continue
		}
		ok := true
		for _, n := range m.notifiers {
			if err := n.Send(ctx, b.String()); err != nil {
				slog.Error("send", "source", e.ID, "key", it.Key, "err", err)
				ok = false
			}
		}
		if !ok {
			failed++
			continue
		}
		m.state.Add(e.ID, it.Key)
		sent++
		slog.Info("notified", "source", e.ID, "key", it.Key)
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
