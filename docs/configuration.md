# Configuration

Environment variables; every one has a default. Personal settings (notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What |
|---|---|---|
| `PORT`, `HOST` | `8080`, `0.0.0.0` | Listen address |
| `BASE_URL` | – | Where the app is (links in pushes and in things made in other apps, the OIDC callback) |
| `DATA_DIR` | `./data` | Database, Core token, backups |
| `DB_PATH` | `$DATA_DIR/pocket-family.db` | SQLite file |
| `MAX_BODY_BYTES` | `67108864` | Largest JSON request (64 MB) |
| `TZ` | – | Time zone for days, pick-up times, quiet hours and "tomorrow for you" |
| `AUTH_MODE` | `local` | `local` (accounts), `proxy` (trust `AUTH_PROXY_HEADER`/`AUTH_PROXY_EMAIL_HEADER`), `none` |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | Seed the first admin while the database is empty |
| `SESSION_TTL`, `SECURE_COOKIES` | `720h`, `false` | Set `SECURE_COOKIES=true` behind HTTPS |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_NAME`, `OIDC_AUTO_CREATE` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL`, `NOTIFY_FORMAT` | – | Apprise (`…/notify/key`), ntfy topic URL or JSON webhook; format `apprise`/`ntfy`/`json` |
| `NOTIFY_HOUR` | `19` | Default hour of "tomorrow for you", the evening before (each person can change theirs) |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` | – | AI without Pocket AI (any OpenAI-compatible API: Ollama, LM Studio, OpenRouter…) |
| `CORE_URL`, `CORE_REGISTRATION_KEY`, `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY`, `METRICS_ENABLED`, `LOG_LEVEL`, `LOG_FORMAT` | `true`, `true`, `info`, `text` | |
| `DEMO_DATA` | `false` | Seed a busy family (school runs, chores, letters, lists, the board, the info) into the owner account |

Commands: `pocket-family backup [-o FILE]`, `pocket-family reset-password [-email] [-password]`, `pocket-family version`.

## Families and people

A family is shared by the people in it and seen by nobody else (admins neither). Someone who looks after the family adds people: grown-ups, children and helpers (a grandparent, the sitter). A person with an email links to the account with that email the first time it signs in (with Pocket Core: the first time they open Pocket Family). Children can have accounts too: they tick their own chores, add to lists and post on the board, but do not plan.

## Connected apps

Per person under Settings → Connected apps: through Pocket Core (nothing to paste) or directly with an app's URL and an API token. Read for Today and the week: Pocket Calendar (your own events), League (games), Travel (trips), Kitchen (the meal plan), Weather, Pets and Home (care due), Renewals (papers running out), Contacts (birthdays). Made when you ask: a task (Tasks), an event (Calendar), a note (Notes), a payment (Money), a shared cost (Split), a contact (Contacts), a message (Chat). An app that is not there is greyed with what to do; the rest keeps working.
