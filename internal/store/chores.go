package store

import (
	"context"
	"database/sql"
	"time"
)

// ---- chores ----
//
// A chore is due on next_due. Ticking it off writes who did it (and the
// stars it earns), moves next_due on by its repeat and, for chores done in
// turns, passes the turn on. Undo puts both back.

const choreCols = `id, family_id, title, emoji, notes, repeat, every_days, weekdays, assign, people, turn, stars, part_of_day, next_due, active, source, created_at, updated_at`

func scanChore(row interface{ Scan(...any) error }) (*Chore, error) {
	c := &Chore{}
	var days, people string
	var active int
	if err := row.Scan(&c.ID, &c.FamilyID, &c.Title, &c.Emoji, &c.Notes, &c.Repeat, &c.EveryDays, &days, &c.Assign, &people, &c.Turn, &c.Stars, &c.PartOfDay,
		&c.NextDue, &active, &c.Source, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	c.Weekdays, c.People, c.Active = parseDays(days), parseIDs(people), active == 1
	c.Whose = c.WhoseTurn()
	return c, nil
}

// WhoseTurn is the person the chore is for now (0 = anyone).
func (c *Chore) WhoseTurn() int64 {
	if len(c.People) == 0 || c.Assign == "anyone" {
		return 0
	}
	if c.Assign == "turns" {
		return c.People[((c.Turn%len(c.People))+len(c.People))%len(c.People)]
	}
	return c.People[0]
}

// For reports whether the chore is someone's to do now.
func (c *Chore) For(pid int64) bool {
	if c.Assign == "anyone" || len(c.People) == 0 {
		return true
	}
	if c.Assign == "one" {
		return HasID(c.People, pid)
	}
	return c.WhoseTurn() == pid
}

// ListChores lists a family's chores (inactive ones too when asked).
func (s *Store) ListChores(ctx context.Context, fid int64, all bool) ([]Chore, error) {
	q := `SELECT ` + choreCols + ` FROM chores WHERE family_id = ?`
	if !all {
		q += ` AND active = 1`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY active DESC, CASE WHEN next_due = '' THEN '9999' ELSE next_due END, CASE part_of_day WHEN 'morning' THEN 0 WHEN 'afternoon' THEN 1 WHEN 'evening' THEN 2 ELSE 3 END, title`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Chore{}
	for rows.Next() {
		c, err := scanChore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetChore is one chore.
func (s *Store) GetChore(ctx context.Context, fid, id int64) (*Chore, error) {
	return scanChore(s.DB.QueryRowContext(ctx, `SELECT `+choreCols+` FROM chores WHERE family_id = ? AND id = ?`, fid, id))
}

// CreateChore adds a chore.
func (s *Store) CreateChore(ctx context.Context, uid int64, c *Chore) error {
	if c.NextDue == "" {
		c.NextDue = FirstDue(c, Today())
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO chores (family_id, title, emoji, notes, repeat, every_days, weekdays, assign, people, turn, stars, part_of_day, next_due, active, source, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		c.FamilyID, c.Title, c.Emoji, c.Notes, c.Repeat, c.EveryDays, daysCSV(c.Weekdays), c.Assign, idsJSON(c.People), c.Turn, c.Stars, c.PartOfDay, c.NextDue, c.Source, nullI(&uid))
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.Active, c.Whose = true, c.WhoseTurn()
	return nil
}

// UpdateChore saves a chore.
func (s *Store) UpdateChore(ctx context.Context, c *Chore) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE chores SET title = ?, emoji = ?, notes = ?, repeat = ?, every_days = ?, weekdays = ?, assign = ?, people = ?, turn = ?, stars = ?, part_of_day = ?,
		next_due = ?, active = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		c.Title, c.Emoji, c.Notes, c.Repeat, c.EveryDays, daysCSV(c.Weekdays), c.Assign, idsJSON(c.People), c.Turn, c.Stars, c.PartOfDay,
		c.NextDue, boolInt(c.Active), Now(), c.FamilyID, c.ID)
	return err
}

// DeleteChore removes a chore and its history.
func (s *Store) DeleteChore(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM chores WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FirstDue is the first day on or after from that a new chore is due.
func FirstDue(c *Chore, from string) string {
	if c.Repeat == "weekly" && len(c.Weekdays) > 0 {
		for d := from; d < AddDays(from, 7); d = AddDays(d, 1) {
			if HasDay(c.Weekdays, Weekday(d)) {
				return d
			}
		}
	}
	return from
}

// NextAfter is when a chore done on day is due again ("" = never: a one-off).
func NextAfter(c *Chore, day string) string {
	switch c.Repeat {
	case "once":
		return ""
	case "daily":
		return AddDays(day, 1)
	case "every":
		n := c.EveryDays
		if n < 1 {
			n = 1
		}
		return AddDays(day, n)
	case "monthly":
		t, err := ParseDate(c.NextDue)
		if err != nil {
			t, _ = ParseDate(day)
		}
		for t.Format("2006-01-02") <= day {
			t = t.AddDate(0, 1, 0)
		}
		return t.Format("2006-01-02")
	}
	// weekly
	if len(c.Weekdays) > 0 {
		for d := AddDays(day, 1); d <= AddDays(day, 7); d = AddDays(d, 1) {
			if HasDay(c.Weekdays, Weekday(d)) {
				return d
			}
		}
	}
	next := c.NextDue
	if next == "" {
		next = day
	}
	for next <= day {
		next = AddDays(next, 7)
	}
	return next
}

// DoChore ticks a chore off for a person on a day.
func (s *Store) DoChore(ctx context.Context, uid int64, c *Chore, pid int64, day string) (*Done, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	d := &Done{ChoreID: c.ID, PersonID: pid, Day: day, Stars: c.Stars}
	res, err := tx.ExecContext(ctx, `INSERT INTO chore_log (family_id, chore_id, person_id, day, stars, by_user, prev_due, prev_turn) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.FamilyID, c.ID, nullI(&pid), day, c.Stars, nullI(&uid), c.NextDue, c.Turn)
	if err != nil {
		return nil, err
	}
	d.ID, _ = res.LastInsertId()
	next := NextAfter(c, day)
	active := 1
	if next == "" {
		active = 0
	}
	turn := c.Turn
	if c.Assign == "turns" && len(c.People) > 0 {
		turn = (turn + 1) % len(c.People)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chores SET next_due = ?, turn = ?, active = ?, updated_at = ? WHERE id = ?`, next, turn, active, Now(), c.ID); err != nil {
		return nil, err
	}
	return d, tx.Commit()
}

// UndoChore takes back the latest tick of a chore.
func (s *Store) UndoChore(ctx context.Context, fid, choreID int64) error {
	var id int64
	var due string
	var turn int
	err := s.DB.QueryRowContext(ctx, `SELECT id, prev_due, prev_turn FROM chore_log WHERE family_id = ? AND chore_id = ? ORDER BY id DESC LIMIT 1`, fid, choreID).Scan(&id, &due, &turn)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM chore_log WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chores SET next_due = ?, turn = ?, active = 1, updated_at = ? WHERE family_id = ? AND id = ?`, due, turn, Now(), fid, choreID); err != nil {
		return err
	}
	return tx.Commit()
}

// ChoreLog lists the ticks between two days, newest first.
func (s *Store) ChoreLog(ctx context.Context, fid int64, from, to string) ([]Done, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id, l.chore_id, COALESCE(l.person_id, 0), l.day, l.stars, l.created_at, c.title, c.emoji FROM chore_log l JOIN chores c ON c.id = l.chore_id
		WHERE l.family_id = ? AND l.day >= ? AND l.day <= ? ORDER BY l.day DESC, l.id DESC`, fid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Done{}
	for rows.Next() {
		var d Done
		if err := rows.Scan(&d.ID, &d.ChoreID, &d.PersonID, &d.Day, &d.Stars, &d.CreatedAt, &d.Title, &d.Emoji); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// WeekStart is the Monday (or Sunday/Saturday, by setting) on or before a day.
func WeekStart(day string, start int) string {
	t, err := ParseDate(day)
	if err != nil {
		t = time.Now()
	}
	if start < 1 || start > 7 {
		start = 1
	}
	w := Weekday(t.Format("2006-01-02"))
	back := (w - start + 7) % 7
	return t.AddDate(0, 0, -back).Format("2006-01-02")
}
