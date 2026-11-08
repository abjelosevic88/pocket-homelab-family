package store

import (
	"context"
)

// ---- school: terms and holidays, each child's week, letters ----

// ListTerms lists a family's terms, holidays and closed days.
func (s *Store) ListTerms(ctx context.Context, fid int64) ([]Term, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, family_id, name, kind, starts_on, ends_on, created_at FROM terms WHERE family_id = ? ORDER BY starts_on, id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Term{}
	for rows.Next() {
		var t Term
		if err := rows.Scan(&t.ID, &t.FamilyID, &t.Name, &t.Kind, &t.StartsOn, &t.EndsOn, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTerm is one term.
func (s *Store) GetTerm(ctx context.Context, fid, id int64) (*Term, error) {
	t := &Term{}
	err := s.DB.QueryRowContext(ctx, `SELECT id, family_id, name, kind, starts_on, ends_on, created_at FROM terms WHERE family_id = ? AND id = ?`, fid, id).
		Scan(&t.ID, &t.FamilyID, &t.Name, &t.Kind, &t.StartsOn, &t.EndsOn, &t.CreatedAt)
	return t, notFound(err)
}

// SaveTerm adds or updates a term and lets runs that depend on school days be made again.
func (s *Store) SaveTerm(ctx context.Context, t *Term) error {
	var err error
	if t.ID == 0 {
		var res interface{ LastInsertId() (int64, error) }
		res, err = s.DB.ExecContext(ctx, `INSERT INTO terms (family_id, name, kind, starts_on, ends_on) VALUES (?, ?, ?, ?, ?)`, t.FamilyID, t.Name, t.Kind, t.StartsOn, t.EndsOn)
		if err == nil {
			t.ID, _ = res.LastInsertId()
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE terms SET name = ?, kind = ?, starts_on = ?, ends_on = ? WHERE family_id = ? AND id = ?`, t.Name, t.Kind, t.StartsOn, t.EndsOn, t.FamilyID, t.ID)
	}
	if err != nil {
		return err
	}
	return s.resetTermRuns(ctx, t.FamilyID)
}

// DeleteTerm removes a term.
func (s *Store) DeleteTerm(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM terms WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return s.resetTermRuns(ctx, fid)
}

// resetTermRuns drops the untouched future runs of term-time plans (they are made again).
func (s *Store) resetTermRuns(ctx context.Context, fid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM runs WHERE family_id = ? AND day >= ? AND touched = 0 AND state = 'planned'
		AND plan_id IN (SELECT id FROM run_plans WHERE family_id = ? AND term_only = 1)`, fid, Today(), fid)
	return err
}

// SchoolDays lists every child's school week in a family.
func (s *Store) SchoolDays(ctx context.Context, fid int64) ([]SchoolDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, person_id, weekday, starts, ends, bring FROM school_days WHERE family_id = ? ORDER BY person_id, weekday`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SchoolDay{}
	for rows.Next() {
		var d SchoolDay
		if err := rows.Scan(&d.ID, &d.PersonID, &d.Weekday, &d.Starts, &d.Ends, &d.Bring); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetSchoolWeek replaces one child's school week.
func (s *Store) SetSchoolWeek(ctx context.Context, fid, pid int64, days []SchoolDay) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM school_days WHERE family_id = ? AND person_id = ?`, fid, pid); err != nil {
		return err
	}
	for _, d := range days {
		if d.Weekday < 1 || d.Weekday > 7 || (d.Starts == "" && d.Ends == "" && d.Bring == "") {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO school_days (family_id, person_id, weekday, starts, ends, bring) VALUES (?, ?, ?, ?, ?, ?)`,
			fid, pid, d.Weekday, d.Starts, d.Ends, d.Bring); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const letterCols = `id, family_id, title, sender, body, summary, received_on, people, kind, action, due_on, amount, currency, state, COALESCE(done_by, 0), done_at, source, created_at, updated_at`

func scanLetter(row interface{ Scan(...any) error }) (*Letter, error) {
	l := &Letter{}
	var people string
	if err := row.Scan(&l.ID, &l.FamilyID, &l.Title, &l.Sender, &l.Body, &l.Summary, &l.ReceivedOn, &people, &l.Kind, &l.Action, &l.DueOn, &l.Amount, &l.Currency,
		&l.State, &l.DoneBy, &l.DoneAt, &l.Source, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	l.People = parseIDs(people)
	return l, nil
}

// ListLetters lists a family's letters: to do first (soonest due), then the rest, newest first.
func (s *Store) ListLetters(ctx context.Context, fid int64, state string, limit int) ([]Letter, error) {
	q := `SELECT ` + letterCols + ` FROM letters WHERE family_id = ?`
	args := []any{fid}
	if state != "" {
		q += ` AND state = ?`
		args = append(args, state)
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY state = 'todo' DESC, CASE WHEN state = 'todo' AND due_on <> '' THEN due_on ELSE '9999' END, received_on DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Letter{}
	for rows.Next() {
		l, err := scanLetter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// GetLetter is one letter.
func (s *Store) GetLetter(ctx context.Context, fid, id int64) (*Letter, error) {
	return scanLetter(s.DB.QueryRowContext(ctx, `SELECT `+letterCols+` FROM letters WHERE family_id = ? AND id = ?`, fid, id))
}

// CreateLetter adds a letter.
func (s *Store) CreateLetter(ctx context.Context, uid int64, l *Letter) error {
	if l.State == "" {
		l.State = "todo"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO letters (family_id, title, sender, body, summary, received_on, people, kind, action, due_on, amount, currency, state, source, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.FamilyID, l.Title, l.Sender, l.Body, l.Summary, l.ReceivedOn, idsJSON(l.People), l.Kind, l.Action, l.DueOn, l.Amount, l.Currency, l.State, l.Source, nullI(&uid))
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	return nil
}

// SaveLetter writes a letter.
func (s *Store) SaveLetter(ctx context.Context, l *Letter) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE letters SET title = ?, sender = ?, body = ?, summary = ?, received_on = ?, people = ?, kind = ?, action = ?, due_on = ?, amount = ?, currency = ?,
		state = ?, done_by = ?, done_at = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		l.Title, l.Sender, l.Body, l.Summary, l.ReceivedOn, idsJSON(l.People), l.Kind, l.Action, l.DueOn, l.Amount, l.Currency,
		l.State, nullI(&l.DoneBy), l.DoneAt, Now(), l.FamilyID, l.ID)
	return err
}

// DeleteLetter removes a letter (its events stay).
func (s *Store) DeleteLetter(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM letters WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
