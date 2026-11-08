package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// ---- families and their people ----
//
// The caller's own row in people is the access check for everything in a
// family: no row, no family (ErrNotFound, a 404 to the outside).

const personCols = `id, family_id, COALESCE(user_id, 0), email, name, role, manager, color, emoji, birthday, school, class_name, phone, notes, contact_ref, position, archived, created_at, updated_at`

func scanPerson(row interface{ Scan(...any) error }) (*Person, error) {
	p := &Person{}
	var mgr, arch int
	if err := row.Scan(&p.ID, &p.FamilyID, &p.UserID, &p.Email, &p.Name, &p.Role, &mgr, &p.Color, &p.Emoji, &p.Birthday, &p.School, &p.ClassName,
		&p.Phone, &p.Notes, &p.ContactRef, &p.Position, &arch, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	p.Manager, p.Archived = mgr == 1, arch == 1
	p.Waiting = p.UserID == 0 && p.Email != ""
	return p, nil
}

// ClaimInvites links the people waiting for this email to the account.
func (s *Store) ClaimInvites(ctx context.Context, uid int64, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || uid <= 0 {
		return nil
	}
	// one account per family: skip families this account is already in
	_, err := s.DB.ExecContext(ctx, `UPDATE people SET user_id = ?, updated_at = ? WHERE user_id IS NULL AND lower(email) = ?
		AND family_id NOT IN (SELECT family_id FROM people WHERE user_id = ?)`, uid, Now(), email, uid)
	return err
}

// CreateFamily makes a family with the caller in it as its first manager.
func (s *Store) CreateFamily(ctx context.Context, uid int64, f *Family, me *Person) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO families (name, emoji, color, created_by) VALUES (?, ?, ?, ?)`, f.Name, f.Emoji, f.Color, uid)
	if err != nil {
		return err
	}
	f.ID, _ = res.LastInsertId()
	me.FamilyID, me.UserID, me.Manager = f.ID, uid, true
	if me.Role == "" {
		me.Role = RoleGrownup
	}
	if err := insertPerson(ctx, tx, me); err != nil {
		return err
	}
	// every family starts with a shopping list
	if _, err := tx.ExecContext(ctx, `INSERT INTO lists (family_id, name, emoji, position) VALUES (?, 'Groceries', '🛒', 0)`, f.ID); err != nil {
		return err
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func insertPerson(ctx context.Context, x execer, p *Person) error {
	res, err := x.ExecContext(ctx, `INSERT INTO people (family_id, user_id, email, name, role, manager, color, emoji, birthday, school, class_name, phone, notes, contact_ref, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM people WHERE family_id = ?))`,
		p.FamilyID, nullI(&p.UserID), strings.ToLower(strings.TrimSpace(p.Email)), p.Name, p.Role, boolInt(p.Manager), p.Color, p.Emoji, p.Birthday, p.School, p.ClassName,
		p.Phone, p.Notes, p.ContactRef, p.FamilyID)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// MyFamilies lists the families the account is in (after claiming waiting
// invites for its email), each with the caller as Me.
func (s *Store) MyFamilies(ctx context.Context, uid int64, email string) ([]Family, error) {
	_ = s.ClaimInvites(ctx, uid, email)
	rows, err := s.DB.QueryContext(ctx, `SELECT f.id, f.name, f.emoji, f.color, COALESCE(f.created_by, 0), f.created_at, f.updated_at FROM families f
		JOIN people p ON p.family_id = f.id AND p.user_id = ? ORDER BY f.id`, uid)
	if err != nil {
		return nil, err
	}
	var out []Family
	for rows.Next() {
		var f Family
		if err := rows.Scan(&f.ID, &f.Name, &f.Emoji, &f.Color, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		people, err := s.ListPeople(ctx, out[i].ID, false)
		if err != nil {
			return nil, err
		}
		out[i].People = people
		for j := range people {
			if people[j].UserID == uid {
				out[i].Me = &people[j]
			}
		}
	}
	return out, nil
}

// FamilyFor is a family the account is in, with its people and the caller.
func (s *Store) FamilyFor(ctx context.Context, uid, fid int64) (*Family, error) {
	f := &Family{}
	err := s.DB.QueryRowContext(ctx, `SELECT f.id, f.name, f.emoji, f.color, COALESCE(f.created_by, 0), f.created_at, f.updated_at FROM families f
		JOIN people p ON p.family_id = f.id AND p.user_id = ? WHERE f.id = ?`, uid, fid).Scan(&f.ID, &f.Name, &f.Emoji, &f.Color, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	if f.People, err = s.ListPeople(ctx, fid, true); err != nil {
		return nil, err
	}
	for i := range f.People {
		if f.People[i].UserID == uid {
			f.Me = &f.People[i]
		}
	}
	if f.Me == nil {
		return nil, ErrNotFound
	}
	return f, nil
}

// UpdateFamily saves the name, emoji and colour.
func (s *Store) UpdateFamily(ctx context.Context, f *Family) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE families SET name = ?, emoji = ?, color = ?, updated_at = ? WHERE id = ?`, f.Name, f.Emoji, f.Color, Now(), f.ID)
	return err
}

// DeleteFamily removes a family and everything in it.
func (s *Store) DeleteFamily(ctx context.Context, fid int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM families WHERE id = ?`, fid)
	return err
}

// ListPeople lists a family's people in their order (archived too when asked).
func (s *Store) ListPeople(ctx context.Context, fid int64, archived bool) ([]Person, error) {
	q := `SELECT ` + personCols + ` FROM people WHERE family_id = ?`
	if !archived {
		q += ` AND archived = 0`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY archived, CASE role WHEN 'grownup' THEN 0 WHEN 'child' THEN 1 ELSE 2 END, position, id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPerson is one person in a family.
func (s *Store) GetPerson(ctx context.Context, fid, pid int64) (*Person, error) {
	return scanPerson(s.DB.QueryRowContext(ctx, `SELECT `+personCols+` FROM people WHERE family_id = ? AND id = ?`, fid, pid))
}

// CreatePerson adds someone to a family; an email that already has an
// account here links at once.
func (s *Store) CreatePerson(ctx context.Context, p *Person) error {
	p.Email = strings.ToLower(strings.TrimSpace(p.Email))
	if p.Email != "" && p.UserID == 0 {
		if u, err := s.GetUserByEmail(ctx, p.Email); err == nil {
			var n int
			_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM people WHERE family_id = ? AND user_id = ?`, p.FamilyID, u.ID).Scan(&n)
			if n > 0 {
				return ErrConflict
			}
			p.UserID = u.ID
		}
	}
	return insertPerson(ctx, s.DB, p)
}

// UpdatePerson saves a person (the account link changes only through email).
func (s *Store) UpdatePerson(ctx context.Context, p *Person) error {
	p.Email = strings.ToLower(strings.TrimSpace(p.Email))
	cur, err := s.GetPerson(ctx, p.FamilyID, p.ID)
	if err != nil {
		return err
	}
	uid := cur.UserID
	if cur.UserID == 0 && p.Email != "" {
		if u, err := s.GetUserByEmail(ctx, p.Email); err == nil {
			var n int
			_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM people WHERE family_id = ? AND user_id = ?`, p.FamilyID, u.ID).Scan(&n)
			if n == 0 {
				uid = u.ID
			}
		}
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE people SET user_id = ?, email = ?, name = ?, role = ?, manager = ?, color = ?, emoji = ?, birthday = ?, school = ?, class_name = ?,
		phone = ?, notes = ?, contact_ref = ?, position = ?, archived = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		nullI(&uid), p.Email, p.Name, p.Role, boolInt(p.Manager), p.Color, p.Emoji, p.Birthday, p.School, p.ClassName,
		p.Phone, p.Notes, p.ContactRef, p.Position, boolInt(p.Archived), Now(), p.FamilyID, p.ID)
	p.UserID = uid
	return conflict(err)
}

// UnlinkPerson lets a person's account go (they leave; the person stays).
func (s *Store) UnlinkPerson(ctx context.Context, fid, pid int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE people SET user_id = NULL, email = '', manager = 0, updated_at = ? WHERE family_id = ? AND id = ?`, Now(), fid, pid)
	return err
}

// DeletePerson removes a person from a family.
func (s *Store) DeletePerson(ctx context.Context, fid, pid int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM people WHERE family_id = ? AND id = ?`, fid, pid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountManagers is how many people with an account may manage the family.
func (s *Store) CountManagers(ctx context.Context, fid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM people WHERE family_id = ? AND manager = 1 AND user_id IS NOT NULL`, fid).Scan(&n)
	return n
}

// FamiliesOf lists the ids of every family with at least one account (background work).
func (s *Store) AllFamilies(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT family_id FROM people WHERE user_id IS NOT NULL ORDER BY family_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---- small JSON helpers for id lists ----

func idsJSON(ids []int64) string {
	if len(ids) == 0 {
		return "[]"
	}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func parseIDs(s string) []int64 {
	out := []int64{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []int64{}
	}
	return out
}

func daysCSV(days []int) string {
	seen := map[int]bool{}
	var parts []int
	for _, d := range days {
		if d >= 1 && d <= 7 && !seen[d] {
			seen[d] = true
			parts = append(parts, d)
		}
	}
	sort.Ints(parts)
	ss := make([]string, len(parts))
	for i, d := range parts {
		ss[i] = strconv.Itoa(d)
	}
	return strings.Join(ss, ",")
}

func parseDays(s string) []int {
	out := []int{}
	for _, p := range strings.Split(s, ",") {
		if d, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && d >= 1 && d <= 7 {
			out = append(out, d)
		}
	}
	return out
}

// Weekday is the ISO weekday (1 = Monday … 7 = Sunday) of a YYYY-MM-DD date.
func Weekday(day string) int {
	t, err := ParseDate(day)
	if err != nil {
		return 0
	}
	w := int(t.Weekday())
	if w == 0 {
		return 7
	}
	return w
}

// HasDay reports whether an ISO weekday is in the list.
func HasDay(days []int, d int) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// HasID reports whether an id is in the list.
func HasID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
