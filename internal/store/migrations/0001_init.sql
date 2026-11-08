CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Family ----------
--
-- A family is shared by the people in it; nobody else (an admin neither) can
-- see it. The access check is always the caller's own row in people (a
-- grown-up or a child with an account on this box). Children without an
-- account, grandparents and the babysitter are people without a user_id.
-- Every query starts from the family the caller belongs to.

CREATE TABLE families (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL DEFAULT '',
  emoji      TEXT NOT NULL DEFAULT '',
  color      TEXT NOT NULL DEFAULT '',
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- The people in a family. user_id links a person to an account here; email
-- waits for that account (it links the first time they sign in).
CREATE TABLE people (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id   INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
  email       TEXT NOT NULL DEFAULT '',
  name        TEXT NOT NULL DEFAULT '',
  role        TEXT NOT NULL DEFAULT 'grownup',   -- grownup | child | helper (grandparent, sitter: helps, no say)
  manager     INTEGER NOT NULL DEFAULT 0,        -- may change the family and its people
  color       TEXT NOT NULL DEFAULT '',
  emoji       TEXT NOT NULL DEFAULT '',
  birthday    TEXT NOT NULL DEFAULT '',          -- YYYY-MM-DD (or --MM-DD without a year)
  school      TEXT NOT NULL DEFAULT '',
  class_name  TEXT NOT NULL DEFAULT '',
  phone       TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  contact_ref TEXT NOT NULL DEFAULT '',          -- a card in Pocket Contacts
  position    INTEGER NOT NULL DEFAULT 0,
  archived    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX people_user ON people(family_id, user_id) WHERE user_id IS NOT NULL;
CREATE INDEX people_by_user ON people(user_id);
CREATE INDEX people_email ON people(email) WHERE user_id IS NULL AND email <> '';

-- Pick-ups, drop-offs and lifts that repeat every week ("Mila from school,
-- Mon–Fri 13:30, Ana"). Each day becomes a run the family can change.
CREATE TABLE run_plans (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL DEFAULT 'pickup',     -- pickup | dropoff | lift
  title      TEXT NOT NULL DEFAULT '',
  place      TEXT NOT NULL DEFAULT '',
  weekdays   TEXT NOT NULL DEFAULT '1,2,3,4,5',  -- ISO weekdays, 1 = Monday
  time       TEXT NOT NULL DEFAULT '',           -- HH:MM
  children   TEXT NOT NULL DEFAULT '[]',         -- person ids
  driver_id  INTEGER REFERENCES people(id) ON DELETE SET NULL,
  -- who takes it on which weekday ({"2": 5}: Tuesdays person 5), else driver_id
  rota       TEXT NOT NULL DEFAULT '{}',
  term_only  INTEGER NOT NULL DEFAULT 0,         -- not in school holidays
  starts_on  TEXT NOT NULL DEFAULT '',
  ends_on    TEXT NOT NULL DEFAULT '',
  notes      TEXT NOT NULL DEFAULT '',
  active     INTEGER NOT NULL DEFAULT 1,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX run_plans_family ON run_plans(family_id);

-- One pick-up, drop-off or lift on one day: who takes it, who asked for
-- someone else to, and whether it happened.
CREATE TABLE runs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id    INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  plan_id      INTEGER REFERENCES run_plans(id) ON DELETE CASCADE,
  day          TEXT NOT NULL,
  time         TEXT NOT NULL DEFAULT '',
  kind         TEXT NOT NULL DEFAULT 'pickup',
  title        TEXT NOT NULL DEFAULT '',
  place        TEXT NOT NULL DEFAULT '',
  children     TEXT NOT NULL DEFAULT '[]',
  driver_id    INTEGER REFERENCES people(id) ON DELETE SET NULL,
  state        TEXT NOT NULL DEFAULT 'planned',  -- planned | asking | done | skipped
  asked_by     INTEGER REFERENCES people(id) ON DELETE SET NULL,
  asked_note   TEXT NOT NULL DEFAULT '',
  done_at      TEXT NOT NULL DEFAULT '',
  done_by      INTEGER REFERENCES people(id) ON DELETE SET NULL,
  notes        TEXT NOT NULL DEFAULT '',
  touched      INTEGER NOT NULL DEFAULT 0,       -- changed by hand: the plan no longer rewrites it
  source       TEXT NOT NULL DEFAULT '',         -- the app that made it through a Pocket Core link
  created_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX runs_plan_day ON runs(plan_id, day) WHERE plan_id IS NOT NULL;
CREATE INDEX runs_family_day ON runs(family_id, day, time);

-- Jobs at home, for one person, in turns or for anyone, with stars for kids.
CREATE TABLE chores (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id   INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  emoji       TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  repeat      TEXT NOT NULL DEFAULT 'weekly',    -- once | daily | weekly | monthly | every
  every_days  INTEGER NOT NULL DEFAULT 0,        -- repeat = every
  weekdays    TEXT NOT NULL DEFAULT '',          -- weekly: ISO weekdays, "" = the weekday of next_due
  assign      TEXT NOT NULL DEFAULT 'one',       -- one | turns | anyone
  people      TEXT NOT NULL DEFAULT '[]',        -- person ids
  turn        INTEGER NOT NULL DEFAULT 0,        -- whose turn (index into people)
  stars       INTEGER NOT NULL DEFAULT 0,
  part_of_day TEXT NOT NULL DEFAULT '',          -- "" | morning | afternoon | evening
  next_due    TEXT NOT NULL DEFAULT '',
  active      INTEGER NOT NULL DEFAULT 1,
  source      TEXT NOT NULL DEFAULT '',
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX chores_family ON chores(family_id, active, next_due);

CREATE TABLE chore_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  chore_id   INTEGER NOT NULL REFERENCES chores(id) ON DELETE CASCADE,
  person_id  INTEGER REFERENCES people(id) ON DELETE SET NULL,
  day        TEXT NOT NULL,
  stars      INTEGER NOT NULL DEFAULT 0,
  by_user    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  prev_due   TEXT NOT NULL DEFAULT '',           -- to undo: when it was due and whose turn it was
  prev_turn  INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX chore_log_family ON chore_log(family_id, day);
CREATE INDEX chore_log_chore ON chore_log(chore_id, day);

-- The family's own plans: school things, clubs, appointments, visits.
CREATE TABLE events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id   INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  day         TEXT NOT NULL,
  end_day     TEXT NOT NULL DEFAULT '',
  start_time  TEXT NOT NULL DEFAULT '',
  end_time    TEXT NOT NULL DEFAULT '',
  place       TEXT NOT NULL DEFAULT '',
  people      TEXT NOT NULL DEFAULT '[]',
  kind        TEXT NOT NULL DEFAULT 'family',    -- school | activity | appointment | family | other
  repeat      TEXT NOT NULL DEFAULT 'none',      -- none | weekly | yearly
  weekdays    TEXT NOT NULL DEFAULT '',          -- weekly
  until       TEXT NOT NULL DEFAULT '',
  bring       TEXT NOT NULL DEFAULT '',          -- what to take along
  notes       TEXT NOT NULL DEFAULT '',
  letter_id   INTEGER REFERENCES letters(id) ON DELETE SET NULL,
  source      TEXT NOT NULL DEFAULT '',
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX events_family_day ON events(family_id, day);

-- Shared shopping lists; shopper_id is who is at the shop right now.
CREATE TABLE lists (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id     INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  name          TEXT NOT NULL DEFAULT '',
  emoji         TEXT NOT NULL DEFAULT '',
  position      INTEGER NOT NULL DEFAULT 0,
  shopper_id    INTEGER REFERENCES people(id) ON DELETE SET NULL,
  shopper_since TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX lists_family ON lists(family_id, position);

CREATE TABLE items (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  list_id    INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  qty        TEXT NOT NULL DEFAULT '',
  note       TEXT NOT NULL DEFAULT '',
  aisle      TEXT NOT NULL DEFAULT '',
  added_by   INTEGER REFERENCES people(id) ON DELETE SET NULL,
  done       INTEGER NOT NULL DEFAULT 0,
  done_by    INTEGER REFERENCES people(id) ON DELETE SET NULL,
  done_at    TEXT NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  source     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX items_list ON items(list_id, done, position);
CREATE INDEX items_family ON items(family_id);

-- School terms, holidays and closed days (term_only runs skip holidays).
CREATE TABLE terms (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'holiday',    -- term | holiday | closed
  starts_on  TEXT NOT NULL,
  ends_on    TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX terms_family ON terms(family_id, starts_on);

-- A child's school week: hours and what to take each day (PE kit on Tuesday).
CREATE TABLE school_days (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  person_id  INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  weekday    INTEGER NOT NULL,                   -- 1 = Monday
  starts     TEXT NOT NULL DEFAULT '',
  ends       TEXT NOT NULL DEFAULT '',
  bring      TEXT NOT NULL DEFAULT '',
  UNIQUE (person_id, weekday)
);
CREATE INDEX school_days_family ON school_days(family_id);

-- Letters and messages from school or a club, and what they ask for.
CREATE TABLE letters (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id   INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  sender      TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT '',
  summary     TEXT NOT NULL DEFAULT '',
  received_on TEXT NOT NULL DEFAULT '',
  people      TEXT NOT NULL DEFAULT '[]',
  kind        TEXT NOT NULL DEFAULT 'info',      -- info | permission | payment | bring | event
  action      TEXT NOT NULL DEFAULT '',          -- what to do ("sign and return the slip")
  due_on      TEXT NOT NULL DEFAULT '',
  amount      INTEGER NOT NULL DEFAULT 0,        -- minor units
  currency    TEXT NOT NULL DEFAULT '',
  state       TEXT NOT NULL DEFAULT 'todo',      -- todo | done | none (nothing to do)
  done_by     INTEGER REFERENCES people(id) ON DELETE SET NULL,
  done_at     TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT '',
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX letters_family ON letters(family_id, state, due_on);

-- The fridge door: short notes for everyone, and who has seen them.
CREATE TABLE posts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  person_id  INTEGER REFERENCES people(id) ON DELETE SET NULL,
  body       TEXT NOT NULL DEFAULT '',
  pinned     INTEGER NOT NULL DEFAULT 0,
  until      TEXT NOT NULL DEFAULT '',           -- off the board after this day
  source     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX posts_family ON posts(family_id, created_at);

CREATE TABLE post_seen (
  post_id   INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  person_id INTEGER NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (post_id, person_id)
);

-- What everyone needs to know: who to call, allergies, the doctor, the wifi.
CREATE TABLE info (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id   INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  person_id   INTEGER REFERENCES people(id) ON DELETE CASCADE,   -- NULL = the whole family
  section     TEXT NOT NULL DEFAULT 'other',     -- emergency | health | school | home | sitter | other
  label       TEXT NOT NULL DEFAULT '',
  value       TEXT NOT NULL DEFAULT '',
  phone       TEXT NOT NULL DEFAULT '',
  contact_ref TEXT NOT NULL DEFAULT '',
  on_sheet    INTEGER NOT NULL DEFAULT 1,        -- printed on the babysitter sheet
  position    INTEGER NOT NULL DEFAULT 0,
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX info_family ON info(family_id, section, position);

-- Who did what, for "done by whom and when".
CREATE TABLE activity (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  person_id  INTEGER REFERENCES people(id) ON DELETE SET NULL,
  kind       TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL DEFAULT '',
  ref_kind   TEXT NOT NULL DEFAULT '',
  ref_id     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activity_family ON activity(family_id, created_at);

-- What a person made from something here in their own other apps (a task,
-- a calendar event, a payment, a note). Private to that person.
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id  INTEGER NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  owner      TEXT NOT NULL,                      -- run | event | letter | chore | post | info | person
  owner_id   INTEGER NOT NULL,
  target     TEXT NOT NULL,                      -- task | event | note | payment | expense | contact | chat
  app        TEXT NOT NULL,
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX made_user ON made(user_id, owner, owner_id);
