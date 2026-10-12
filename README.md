# model-notifier

Go daemon that watches AI providers (docs pages, GitHub releases) for new LLM releases and posts them to a Telegram channel.

## Quick start

```sh
cp .env.example .env   # fill in TG_BOT_TOKEN and TG_CHAT_ID
docker compose up -d
curl localhost:8080/healthz
```

Sources live in `config.yaml`. Details, config reference and development guide: [AGENTS.md](AGENTS.md).
