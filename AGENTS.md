# AGENTS.md

Canonical guide for AI agents (Claude, Codex, Pi, ...) and humans working on this repo.

## Purpose

Go daemon that polls AI-provider sources for new LLM releases and posts each new one to Telegram.
Sources are declared in `config.yaml`: web pages scraped via XPath, GitHub releases Atom feeds.
Detection is a set diff against a JSON state file; no database, no queue.
Providers today: Anthropic (models overview page, anthropic-sdk-python releases), OpenAI (API changelog, openai-python releases), Google (Gemini API changelog, python-genai releases) and xAI (x.ai/api pricing tables, xai-sdk-python releases).

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
| `internal/source/testdata/` | offline fixtures `<provider>-<source>.<ext>` (`make fixtures` refreshes) |
| `internal/source/source_test.go` | fixture tests; they load source rules from the real `config.yaml` |
| `internal/notify/` | `Notifier` interface, `New` switch, Telegram impl |
| `internal/state/state.go` | seen-items map, atomic Load/Save |
| `internal/monitor/monitor.go` | per-source loop, `Check`, `RunAll` |
| `config.yaml` | default config (Anthropic, OpenAI, Google, xAI) |
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
| `fields` | webpage | name → XPath. Starting with `.` → relative to the item node. Otherwise document-wide, zipped with items by index. Field `link` becomes `Item.Link`, resolved to absolute against `url`. Items with a duplicate `Key` are dropped (first match wins) |
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
- Source tests read XPaths/filters from `config.yaml` via `cfg(t, "provider/source")`; changing a rule there is tested automatically. Adding a source = add a fixture + a `Test<Provider><Source>`.
- `gofmt`-clean; `log/slog` for logs; errors logged, never fatal in the loop.
- `ponytail:` comments mark known deliberate shortcuts and their upgrade path.
- Never log tokens or full Telegram API URLs (they contain the bot token).

## Gotchas

- Only `${VAR}` is expanded; a bare `$` (e.g. a regex anchor) stays literal. An unset variable expands to empty and fails validation for notifier fields.
- Set diff ignores removals: items disappearing from a source are not reported and stay in state.
- First run for a source seeds state silently (no messages) unless `-notify-on-seed`.
- A failed send is not recorded, so the item is retried next tick.
- 429/503 (or 403 + `Retry-After`) pauses that source until `Retry-After` (capped at 24h); `POST /run` obeys the pause. Without the header the next tick just retries.
- Telegram HTML mode rejects unknown tags and unescaped `<`, `>`, `&`; keep authored tags in the template, let `html/template` escape data.
- Live smoke test: `make once ARGS=-notify-on-seed` with real `TG_BOT_TOKEN`/`TG_CHAT_ID` exported, then `rm state.json`.
- `config.yaml` points `state_file` at `/data/state.json` (container); `make run`/`make once` rewrite it to `./state.json` in `bin/config.local.yaml`.

- OpenAI changelog: class names are build-hashed (`_Badge_10t5o_1`), so XPaths match `data-*` attributes only. Items are gpt-N model tag badges; a model's first badge in document order is its newest mention (the release entry on release day). There are no per-entry anchors, so the docs link is built from the key in the template.
- openai-python release notes word model additions loosely; the filter matches the `api:` line naming a `gpt-N` id or "model identifier/slug". Known false positive: "gpt-5.6-sol updates"-style lines.
- Gemini changelog: entries are `h2[@data-text]` (date) + sibling `ul`s; items are `gemini-*` `<code>` ids (`-latest` aliases skipped). The `h2` id is a bare `10-08-2026`, so the anchor link is built in the template. Deprecation lines also name old ids; harmless, they are already in state.
- Google SDK feed is `googleapis/python-genai`; `google-gemini/generative-ai-python` is archived. The filter matches "Add …" lines naming a `gemini-N` id or a Gemini … model. Known false positive: tokenizer-map lines ("Add gemini-3-pro-preview support for local tokenizer").
- x.ai/api shows only the current flagship text model plus Imagine models (older grok-4.x live in news/JS only); the voice table lists modes, not ids. Ids are matched inside `<table>` only, since mobile cards duplicate them. No per-model anchors, so links point at `#models` and the docs pricing page.
- xai-sdk-python lines are "<text> by @user in <PR url>"; the filter matches "Add/Update … to/in <X>Model" (new ids land in the `ChatModel`/`ImageGenerationModel` types), bounded by `[^@]` so it can't run into the next line.

## Analyzing a new source

1. Download into a fresh temp dir: `curl -sL -o /tmp/x/page.html <url>`. Confirm the content is server-rendered (grep for a known model id); if only in JS/JSON, XPath won't see it.
2. Try XPaths without writing Go: `xmllint --html --xpath "count(<xpath>)" page.html 2>/dev/null`. Prefer `data-*`/semantic attributes over generated class names.
3. For GitHub feeds the Atom has only 10 entries; tune `filter` on history: `gh api 'repos/<o>/<r>/releases?per_page=100'` and test the regex over `.body`.
4. Save fixtures as `testdata/<provider>-<source>.*`, add the curl to `make fixtures`, add the source to `config.yaml`, write the test.

## Before you say done

```sh
make lint test
```
