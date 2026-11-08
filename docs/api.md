# API

JSON over `/api/v1`, with a session cookie or `Authorization: Bearer <token>` (Settings → API & shortcuts). Errors are `{"error": "…"}`. Every person sees only the families they are in; anything else answers 404, for admins too. A request is about the family named by the `X-Family` header or `?family=`, else the one the person last chose (`POST /families/switch`), else their first.

## Families and people

| Call | What |
|---|---|
| `GET /families` | `{families, current}` — each with its `people` and `me` |
| `POST /families` | `{name, emoji, color, my_name}` → a family with you in it (you look after it) |
| `GET/PUT/DELETE /family` | The current family (PUT/DELETE: who looks after it) |
| `POST /family/leave` | Your account leaves; the person stays |
| `POST /people`, `PUT/DELETE /people/{id}` | `{name, role: grownup|child|helper, email, manager, color, emoji, birthday, school, class_name, phone, notes}` |
| `GET /activity` | Who did what, newest first |

## The day and the week

| Call | What |
|---|---|
| `GET /today?quick=` | Runs today and tomorrow, what needs someone, the agenda (own plans + each person's own apps), chores due, what to take along, letters, lists, the board, weather, papers running out, sources |
| `GET /week?from=&days=` | Day by day: runs, agenda, chores, take along |
| `GET /day/YYYY-MM-DD` | One day for Pocket Day and Pocket AI |
| `GET /summary` | At a glance (Homepage, briefings) |
| `GET /search?q=` | People, plans, letters, chores, items, info, notes |

## Pick-ups

`GET /runs?from=&to=&mine=` · `POST /runs` `{day, time, kind: pickup|dropoff|lift, children, driver_id, place, title, notes}` · `PUT/DELETE /runs/{id}` · `POST /runs/{id}/take|ask|done|undo|skip|assign` (`{note}`, `{person_id}`) · `GET/POST /plans`, `PUT/DELETE /plans/{id}` `{kind, weekdays, time, children, driver_id, rota: {"2": person}, term_only, starts_on, ends_on}`.

## Chores

`GET /chores?all=` · `POST /chores` `{title, emoji, repeat: once|daily|weekly|monthly|every, every_days, weekdays, assign: one|turns|anyone, people, stars, part_of_day}` · `PUT/DELETE /chores/{id}` · `POST /chores/{id}/done` `{person_id, day}` · `POST /chores/{id}/undo` · `GET /stars?week=`.

## Plans, shopping, school, board, info

- `GET /events?from=&to=`, `POST /events`, `GET/PUT/DELETE /events/{id}` — `{title, day, end_day, start_time, end_time, place, people, kind, repeat: none|weekly|yearly, weekdays, until, bring, notes}`
- `GET/POST /lists`, `PUT/DELETE /lists/{id}`, `GET/POST /lists/{id}/items` (`{name}` — several lines, "2 milk" works), `POST /lists/{id}/shopping` `{on}`, `/clear`, `/from-plan` (Pocket Kitchen), `POST /items` `{name, list}` (the first list when none is named), `PUT/DELETE /items/{id}`, `GET /items/recent`
- `GET /school`, `POST /terms`, `PUT/DELETE /terms/{id}`, `PUT /school/week/{person}` `[{weekday, starts, ends, bring}]`, `GET/POST /letters`, `GET/PUT/DELETE /letters/{id}` (POST may carry `events`)
- `GET/POST /posts`, `PUT/DELETE /posts/{id}`, `POST /posts/seen`
- `GET/POST /info`, `PUT/DELETE /info/{id}`

## Other apps and AI

`POST /make` `{owner: run|event|letter|chore|person|sheet|week, owner_id, target: task|event|note|payment|expense|contact|chat, account_id, group_id, amount, to}` · `GET /made?owner=&owner_id=` · `GET /make/options|accounts|groups|chats` · `GET /pick/contacts?q=` · `GET /apps`.
`POST /ai/letter` `{text}` → what a letter asks for (a suggestion to check) · `POST /ai/quick` `{text}` → a run, event, chore, items or a note · `GET /ai/week`.

## For other apps (Pocket Core links)

| Route | Scope |
|---|---|
| `GET /families`, `/family`, `/today`, `/week`, `/summary`, `/search`, `/runs`, `/plans`, `/chores`, `/stars`, `/events[/{id}]`, `/lists`, `/lists/{id}/items`, `/school`, `/letters[/{id}]`, `/posts`, `/info`, `/activity`, `/day/{date}` | `family:read` |
| `POST /items`, `/lists/{id}/items`, `/events`, `/runs`, `/chores`, `/posts`, `/letters`, `/runs/{id}/take…`, `/chores/{id}/done|undo`; `PUT/DELETE` of what that app added | `family:write` |

Events on the Pocket Core bus: `family.run_needs_cover`, `family.run_taken`, `family.run_done`, `family.chore_done`, `family.post_added`, `family.letter_added`, `family.shopping_trip`.

## Box

`GET /healthz`, `/readyz`, `/metrics`; `GET /export.json`, `POST /import` (a Pocket Family export; twice adds nothing twice); `GET /backup` (admin).
