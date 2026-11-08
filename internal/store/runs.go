package store

import (
	"context"
	"encoding/json"
	"time"
)

// ---- pick-ups, drop-offs and lifts ----
//
// A plan repeats every week; each day it falls on becomes a run (made when
// someone looks at those days), so a single day can change hands without
// touching the plan. A run changed by hand is "touched": editing the plan
// later rewrites only the untouched future runs.

const planCols = `id, family_id, kind, title, place, weekdays, time, children, COALESCE(driver_id, 0), rota, term_only, starts_on, ends_on, notes, active, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*RunPlan, error) {
	p := &RunPlan{}
	var days, kids, rota string
	var term, active int
	if err := row.Scan(&p.ID, &p.FamilyID, &p.Kind, &p.Title, &p.Place, &days, &p.Time, &kids, &p.DriverID, &rota, &term, &p.StartsOn, &p.EndsOn, &p.Notes, &active, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	p.Weekdays, p.Children, p.TermOnly, p.Active = parseDays(days), parseIDs(kids), term == 1, active == 1
	p.Rota = map[string]int64{}
	_ = json.Unmarshal([]byte(rota), &p.Rota)
	return p, nil
}

func rotaJSON(r map[string]int64) string {
	clean := map[string]int64{}
	for k, v := range r {
		if len(k) == 1 && k[0] >= '1' && k[0] <= '7' && v > 0 {
			clean[k] = v
		}
	}
	b, _ := json.Marshal(clean)
	return string(b)
}

// ListPlans lists a family's weekly plans.
func (s *Store) ListPlans(ctx context.Context, fid int64) ([]RunPlan, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+planCols+` FROM run_plans WHERE family_id = ? ORDER BY active DESC, time, id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunPlan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPlan is one plan.
func (s *Store) GetPlan(ctx context.Context, fid, id int64) (*RunPlan, error) {
	return scanPlan(s.DB.QueryRowContext(ctx, `SELECT `+planCols+` FROM run_plans WHERE family_id = ? AND id = ?`, fid, id))
}

// CreatePlan adds a weekly plan.
func (s *Store) CreatePlan(ctx context.Context, uid int64, p *RunPlan) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO run_plans (family_id, kind, title, place, weekdays, time, children, driver_id, rota, term_only, starts_on, ends_on, notes, active, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.FamilyID, p.Kind, p.Title, p.Place, daysCSV(p.Weekdays), p.Time, idsJSON(p.Children), nullI(&p.DriverID), rotaJSON(p.Rota), boolInt(p.TermOnly), p.StartsOn, p.EndsOn, p.Notes, boolInt(p.Active), nullI(&uid))
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// UpdatePlan saves a plan and lets its untouched future runs be made again.
func (s *Store) UpdatePlan(ctx context.Context, p *RunPlan) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE run_plans SET kind = ?, title = ?, place = ?, weekdays = ?, time = ?, children = ?, driver_id = ?, rota = ?, term_only = ?, starts_on = ?, ends_on = ?,
		notes = ?, active = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		p.Kind, p.Title, p.Place, daysCSV(p.Weekdays), p.Time, idsJSON(p.Children), nullI(&p.DriverID), rotaJSON(p.Rota), boolInt(p.TermOnly), p.StartsOn, p.EndsOn,
		p.Notes, boolInt(p.Active), Now(), p.FamilyID, p.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE family_id = ? AND plan_id = ? AND day >= ? AND touched = 0 AND state = 'planned'`, p.FamilyID, p.ID, Today()); err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePlan removes a plan; runs already done or changed by hand stay as single runs.
func (s *Store) DeletePlan(ctx context.Context, fid, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE family_id = ? AND plan_id = ? AND touched = 0 AND state = 'planned'`, fid, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET plan_id = NULL WHERE family_id = ? AND plan_id = ?`, fid, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM run_plans WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// EnsureRuns makes the runs the active plans have between from and to
// (inclusive, at most 70 days, never further back than a week ago).
func (s *Store) EnsureRuns(ctx context.Context, fid int64, from, to string) error {
	if min := AddDays(Today(), -7); from < min {
		from = min
	}
	if max := AddDays(from, 70); to > max {
		to = max
	}
	if to < from {
		return nil
	}
	plans, err := s.ListPlans(ctx, fid)
	if err != nil {
		return err
	}
	terms, err := s.ListTerms(ctx, fid)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range plans {
		if !p.Active {
			continue
		}
		for d := from; d <= to; d = AddDays(d, 1) {
			if !p.FallsOn(d, terms) {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO runs (family_id, plan_id, day, time, kind, title, place, children, driver_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				fid, p.ID, d, p.Time, p.Kind, p.Title, p.Place, idsJSON(p.Children), nullI(ptrI(p.DriverOn(Weekday(d))))); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func ptrI(v int64) *int64 { return &v }

// FallsOn reports whether a plan has a run on a day.
func (p *RunPlan) FallsOn(day string, terms []Term) bool {
	if !HasDay(p.Weekdays, Weekday(day)) || (p.StartsOn != "" && day < p.StartsOn) || (p.EndsOn != "" && day > p.EndsOn) {
		return false
	}
	return !p.TermOnly || !NoSchool(day, terms)
}

// NoSchool reports whether a day is in a school holiday or a closed day.
func NoSchool(day string, terms []Term) bool {
	for _, t := range terms {
		if (t.Kind == "holiday" || t.Kind == "closed") && day >= t.StartsOn && day <= t.EndsOn {
			return true
		}
	}
	return false
}

const runCols = `id, family_id, COALESCE(plan_id, 0), day, time, kind, title, place, children, COALESCE(driver_id, 0), state, COALESCE(asked_by, 0), asked_note, done_at, COALESCE(done_by, 0), notes, touched, source, created_at, updated_at`

func scanRun(row interface{ Scan(...any) error }) (*Run, error) {
	r := &Run{}
	var kids string
	var touched int
	if err := row.Scan(&r.ID, &r.FamilyID, &r.PlanID, &r.Day, &r.Time, &r.Kind, &r.Title, &r.Place, &kids, &r.DriverID, &r.State, &r.AskedBy, &r.AskedNote,
		&r.DoneAt, &r.DoneBy, &r.Notes, &touched, &r.Source, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	r.Children, r.Touched = parseIDs(kids), touched == 1
	return r, nil
}

// ListRuns lists the runs between two days (inclusive), made from the plans first.
func (s *Store) ListRuns(ctx context.Context, fid int64, from, to string) ([]Run, error) {
	if err := s.EnsureRuns(ctx, fid, from, to); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+runCols+` FROM runs WHERE family_id = ? AND day >= ? AND day <= ? ORDER BY day, time, id`, fid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	markClashes(out)
	return out, nil
}

// markClashes flags runs where the same person is due somewhere else within 20 minutes.
func markClashes(runs []Run) {
	for i := range runs {
		a := &runs[i]
		if a.DriverID == 0 || a.State == RunSkipped || a.Time == "" {
			continue
		}
		for j := range runs {
			b := &runs[j]
			if i == j || b.DriverID != a.DriverID || b.Day != a.Day || b.State == RunSkipped || b.Time == "" || b.Place == a.Place {
				continue
			}
			if gap := minutesBetween(a.Time, b.Time); gap < 20 {
				a.Clash = b.Time + " " + firstNonEmptyS(b.Place, b.Title)
				break
			}
		}
	}
}

func minutesBetween(a, b string) int {
	ta, err1 := time.Parse("15:04", a)
	tb, err2 := time.Parse("15:04", b)
	if err1 != nil || err2 != nil {
		return 1 << 30
	}
	d := int(ta.Sub(tb).Minutes())
	if d < 0 {
		d = -d
	}
	return d
}

func firstNonEmptyS(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// GetRun is one run.
func (s *Store) GetRun(ctx context.Context, fid, id int64) (*Run, error) {
	return scanRun(s.DB.QueryRowContext(ctx, `SELECT `+runCols+` FROM runs WHERE family_id = ? AND id = ?`, fid, id))
}

// CreateRun adds a single run.
func (s *Store) CreateRun(ctx context.Context, uid int64, r *Run) error {
	if r.State == "" {
		r.State = RunPlanned
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO runs (family_id, day, time, kind, title, place, children, driver_id, state, notes, touched, source, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		r.FamilyID, r.Day, r.Time, r.Kind, r.Title, r.Place, idsJSON(r.Children), nullI(&r.DriverID), r.State, r.Notes, r.Source, nullI(&uid))
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// SaveRun writes every field of a run and marks it changed by hand.
func (s *Store) SaveRun(ctx context.Context, r *Run) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE runs SET day = ?, time = ?, kind = ?, title = ?, place = ?, children = ?, driver_id = ?, state = ?, asked_by = ?, asked_note = ?,
		done_at = ?, done_by = ?, notes = ?, touched = 1, updated_at = ? WHERE family_id = ? AND id = ?`,
		r.Day, r.Time, r.Kind, r.Title, r.Place, idsJSON(r.Children), nullI(&r.DriverID), r.State, nullI(&r.AskedBy), r.AskedNote,
		r.DoneAt, nullI(&r.DoneBy), r.Notes, Now(), r.FamilyID, r.ID)
	return err
}

// DeleteRun removes a single run (a plan's run is skipped instead, so it does not come back).
func (s *Store) DeleteRun(ctx context.Context, fid, id int64) error {
	r, err := s.GetRun(ctx, fid, id)
	if err != nil {
		return err
	}
	if r.PlanID > 0 {
		r.State = RunSkipped
		return s.SaveRun(ctx, r)
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM runs WHERE family_id = ? AND id = ?`, fid, id)
	return err
}
