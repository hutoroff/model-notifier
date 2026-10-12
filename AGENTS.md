# AGENTS.md

Canonical guide for AI agents (Claude, Codex, Pi, ...) and humans working on this repo.

## Purpose

Go daemon that polls AI-provider sources for new LLM releases and posts each new one to Telegram.
Sources are declared in `config.yaml`: web pages scraped via XPath, GitHub releases Atom feeds.
Detection is a set diff against a JSON state file; no database, no queue.

## Architecture

```
            ticker per source / POST /run / -once
                          │
 source.Fetch ──[]Item──▶ monitor.Check ──diff──▶ state (JSON file)
 (webpage, github)        │  new items only
                          ▼
                 html/template render ──▶ notify.Send (telegram)
```

## Layout

| Path | What |
|---|---|
| `cmd/model-notifier/main.go` | flags, wire-up, HTTP server (`GET /healthz`, `POST /run`) |
| `internal/config/config.go` | YAML structs, `${ENV}` expansion, defaults, validation |
| `internal/source/source.go` | `Item`, `Source` interface, `New` switch, shared HTTP `get` |
| `internal/source/webpage.go` | XPath scraper |
| `internal/source/github.go` | GitHub releases Atom feed + regex filter |
| `internal/source/testdata/` | offline fixtures (`make fixtures` refreshes) |
| `internal/notify/` | `Notifier` interface, `New` switch, Telegram impl |
| `internal/state/state.go` | seen-items map, atomic Load/Save |
| `internal/monitor/monitor.go` | per-source loop, `Check`, `RunAll` |
| `config.yaml` | default config (Anthropic) |
| `Dockerfile`, `docker-compose.yml`, `.env.example` | deployment |

## Commands

| Command | Does |
|---|---|
| `make build` | `bin/model-notifier` |
| `make test` | `go test ./...` (offline) |
| `make lint` | gofmt check + `go vet ./...` |
| `make run` | run daemon locally, state in `./state.json` |
| `make once` | run every check once, print summary, exit. `ARGS=-notify-on-seed` to send on first run |
| `make docker` / `make up` | build image / `docker compose up -d --build` |
| `make fixtures` | re-download test fixtures from live sources |

Binary flags: `-config` (default `config.yaml`), `-once`, `-notify-on-seed`, `-healthcheck` (GETs
`http://127.0.0.1:8080/healthz`, exit code; used by Docker since distroless has no curl).

## Config reference

Top level: `interval` (default `30m`), `state_file` (default `state.json`), `listen` (default `:8080`),
`notifiers[]`, `providers[]`. `${VAR}` anywhere is expanded from the environment (`TG_BOT_TOKEN`, `TG_CHAT_ID`).

`notifiers[]`: `name`, `type` (`telegram`), `token`, `chat_id`. Every notifier receives every message.

`providers[]`: `name`, `sources[]`. Each source:

| Field | Applies to | Meaning |
|---|---|---|
| `name`, `url`, `template` | all | required |
| `type` | all | `webpage` or `github_releases` |
| `interval` | all | per-source override of global interval |
| `items` | webpage | XPath; one item per matched node, `Key` = normalized text of the node |
| `fields` | webpage | name → XPath. Starting with `.` → relative to the item node. Otherwise document-wide, zipped with items by index. Field `link` becomes `Item.Link`, resolved to absolute against `url` |
| `filter` | github_releases | regex over title+content; empty → every release. Fields: `title`, `content`, `updated`, `matches` |

`template` is Go `html/template`. Data: `.Key`, `.Link`, `.Fields.<name>`. Values are escaped; the output
must stay within Telegram's HTML subset (`b i u s a code pre blockquote`; no `<br>`, use newlines).

## How to add

- **A provider**: config only. Add an entry under `providers:` with its sources.
- **A source type**: implement `source.Source` in `internal/source/<type>.go`, add a case in `source.New`
  and in `config.validate`, add a fixture under `testdata/` and an offline test.
- **A notifier**: implement `notify.Notifier`, add a case in `notify.New` and in `config.validate`.

## Conventions

- Stdlib first. No new dependency without stating why in the PR. Current deps: `antchfx/htmlquery`, `antchfx/xpath`, `gopkg.in/yaml.v3`.
- Every parser has an offline fixture test. Tests never hit the network.
- `gofmt`-clean; `log/slog` for logs; errors logged, never fatal in the loop.
- `ponytail:` comments mark known deliberate shortcuts and their upgrade path.
- Never log tokens or full Telegram API URLs (they contain the bot token).

## Gotchas

- Only `${VAR}` is expanded; a bare `$` (e.g. a regex anchor) stays literal. An unset variable expands to empty and fails validation for notifier fields.
- Set diff ignores removals: items disappearing from a source are not reported and stay in state.
- First run for a source seeds state silently (no messages) unless `-notify-on-seed`.
- A failed send is not recorded, so the item is retried next tick.
- Telegram HTML mode rejects unknown tags and unescaped `<`, `>`, `&`; keep authored tags in the template, let `html/template` escape data.
- Live smoke test: `make once ARGS=-notify-on-seed` with real `TG_BOT_TOKEN`/`TG_CHAT_ID` exported, then `rm state.json`.
- `config.yaml` points `state_file` at `/data/state.json` (container); `make run`/`make once` rewrite it to `./state.json` in `bin/config.local.yaml`.

## Before you say done

```sh
make lint test
```
