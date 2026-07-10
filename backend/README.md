# Opportunity IM Assistant Backend

FastAPI backend for Telegram and WeCom opportunity detection, human review, and after-hours AI replies.

## Run locally with Docker

```bash
cd backend
cp .env.example .env
docker compose build
docker compose up -d postgres redis
docker compose run --rm migrate
docker compose run --rm api python scripts/seed_demo.py
docker compose up api celery_worker celery_beat
```

API docs: <http://localhost:8000/docs>

Admin endpoints require:

```http
Authorization: Bearer change-me
```

## Main frontend-facing endpoints

- `GET /api/v1/opportunities`
- `GET /api/v1/opportunities/{id}`
- `GET /api/v1/messages?opportunity_id={id}`
- `POST /api/v1/opportunities/{id}/manual-reply`
- `POST /api/v1/opportunities/{id}/ai-draft`
- `GET /api/v1/templates`
- `GET /api/v1/configs/work-mode`
- `GET /api/v1/stats/summary`

## Webhooks

- `POST /api/v1/webhooks/telegram`
- `GET /api/v1/webhooks/wecom`
- `POST /api/v1/webhooks/wecom`

## Telegram user-client ingestion

Telegram Bot API cannot read a group unless the bot is added to that group. For job groups where
you are only a normal member, use the MTProto user-client listener. It logs in with your own
Telegram account session, reads only chats that account can already access, and feeds messages into
the same opportunity detection pipeline as the webhook adapters.

1. Create a Telegram API app at <https://my.telegram.org/apps> and copy `api_id` and `api_hash`.
2. Set these values in `.env`:

```env
TELEGRAM_USER_ENABLED=true
TELEGRAM_USER_API_ID=123456
TELEGRAM_USER_API_HASH=your-api-hash
TELEGRAM_USER_SESSION=
TELEGRAM_USER_CHATS=[]
TELEGRAM_USER_BACKFILL_LIMIT=50
```

3. Generate a string session:

```bash
docker compose --env-file .env run --rm -it telegram_listener \
  python scripts/create_telegram_user_session.py
```

4. Paste the printed `TELEGRAM_USER_SESSION=...` value into `.env`, then list available dialogs:

```bash
docker compose --env-file .env run --rm telegram_listener \
  python scripts/list_telegram_dialogs.py
```

5. Configure the specific job groups or channels:

```env
TELEGRAM_USER_CHATS=["-1001234567890","public_jobs_channel"]
```

6. Start the listener:

```bash
docker compose --env-file .env up -d --force-recreate telegram_listener
```

The user-client listener only ingests messages by default. It does not send AI replies from your
personal Telegram account.
