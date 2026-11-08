package store

import (
	"context"
	"strconv"
	"strings"
)

// ---- the family board, the family info, who did what, and what was made ----

// ListPosts lists the board: pinned first, then newest; notes past their
// day are left out unless all is asked for.
func (s *Store) ListPosts(ctx context.Context, fid, viewer int64, all bool, limit int) ([]Post, error) {
	q := `SELECT id, family_id, COALESCE(person_id, 0), body, pinned, until, source, created_at, updated_at FROM posts WHERE family_id = ?`
	args := []any{fid}
	if !all {
		q += ` AND (until = '' OR until >= ?)`
		args = append(args, Today())
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY pinned DESC, created_at DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	out := []Post{}
	for rows.Next() {
		var p Post
		var pinned int
		if err := rows.Scan(&p.ID, &p.FamilyID, &p.PersonID, &p.Body, &pinned, &p.Until, &p.Source, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.Pinned, p.SeenBy = pinned == 1, []int64{}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]string, len(out))
	idx := map[int64]int{}
	for i, p := range out {
		ids[i] = strconv.FormatInt(p.ID, 10)
		idx[p.ID] = i
	}
	srows, err := s.DB.QueryContext(ctx, `SELECT post_id, person_id FROM post_seen WHERE post_id IN (`+strings.Join(ids, ",")+`) ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var pid, person int64
		if err := srows.Scan(&pid, &person); err != nil {
			return nil, err
		}
		i := idx[pid]
		out[i].SeenBy = append(out[i].SeenBy, person)
		if person == viewer {
			out[i].Seen = true
		}
	}
	for i := range out {
		if out[i].PersonID == viewer {
			out[i].Seen = true
		}
	}
	return out, srows.Err()
}

// GetPost is one post.
func (s *Store) GetPost(ctx context.Context, fid, id int64) (*Post, error) {
	p := &Post{SeenBy: []int64{}}
	var pinned int
	err := s.DB.QueryRowContext(ctx, `SELECT id, family_id, COALESCE(person_id, 0), body, pinned, until, source, created_at, updated_at FROM posts WHERE family_id = ? AND id = ?`, fid, id).
		Scan(&p.ID, &p.FamilyID, &p.PersonID, &p.Body, &pinned, &p.Until, &p.Source, &p.CreatedAt, &p.UpdatedAt)
	p.Pinned = pinned == 1
	return p, notFound(err)
}

// CreatePost puts a note on the board (its author has seen it).
func (s *Store) CreatePost(ctx context.Context, p *Post) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO posts (family_id, person_id, body, pinned, until, source) VALUES (?, ?, ?, ?, ?, ?)`,
		p.FamilyID, nullI(&p.PersonID), p.Body, boolInt(p.Pinned), p.Until, p.Source)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	p.CreatedAt, p.UpdatedAt, p.SeenBy, p.Seen = Now(), Now(), []int64{}, true
	return nil
}

// SavePost writes a post.
func (s *Store) SavePost(ctx context.Context, p *Post) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE posts SET body = ?, pinned = ?, until = ?, updated_at = ? WHERE family_id = ? AND id = ?`, p.Body, boolInt(p.Pinned), p.Until, Now(), p.FamilyID, p.ID)
	return err
}

// DeletePost removes a post.
func (s *Store) DeletePost(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM posts WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SeePosts marks every board note as seen by a person.
func (s *Store) SeePosts(ctx context.Context, fid, person int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO post_seen (post_id, person_id) SELECT id, ? FROM posts WHERE family_id = ? AND COALESCE(person_id, 0) <> ?`, person, fid, person)
	return err
}

// UnseenPosts is how many board notes a person has not seen.
func (s *Store) UnseenPosts(ctx context.Context, fid, person int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM posts p WHERE p.family_id = ? AND COALESCE(p.person_id, 0) <> ? AND (p.until = '' OR p.until >= ?)
		AND NOT EXISTS (SELECT 1 FROM post_seen s WHERE s.post_id = p.id AND s.person_id = ?)`, fid, person, Today(), person).Scan(&n)
	return n
}

// ---- family info ----

const infoCols = `id, family_id, COALESCE(person_id, 0), section, label, value, phone, contact_ref, on_sheet, position, created_at, updated_at`

func scanInfo(row interface{ Scan(...any) error }) (*Info, error) {
	in := &Info{}
	var sheet int
	if err := row.Scan(&in.ID, &in.FamilyID, &in.PersonID, &in.Section, &in.Label, &in.Value, &in.Phone, &in.ContactRef, &sheet, &in.Position, &in.CreatedAt, &in.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	in.OnSheet = sheet == 1
	return in, nil
}

// ListInfo lists a family's info in its sections.
func (s *Store) ListInfo(ctx context.Context, fid int64) ([]Info, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+infoCols+` FROM info WHERE family_id = ? ORDER BY CASE section WHEN 'emergency' THEN 0 WHEN 'health' THEN 1 WHEN 'school' THEN 2
		WHEN 'home' THEN 3 WHEN 'sitter' THEN 4 ELSE 5 END, COALESCE(person_id, 0), position, id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Info{}
	for rows.Next() {
		in, err := scanInfo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// GetInfo is one info line.
func (s *Store) GetInfo(ctx context.Context, fid, id int64) (*Info, error) {
	return scanInfo(s.DB.QueryRowContext(ctx, `SELECT `+infoCols+` FROM info WHERE family_id = ? AND id = ?`, fid, id))
}

// SaveInfo adds or updates an info line.
func (s *Store) SaveInfo(ctx context.Context, uid int64, in *Info) error {
	if in.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO info (family_id, person_id, section, label, value, phone, contact_ref, on_sheet, position, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM info WHERE family_id = ?), ?)`,
			in.FamilyID, nullI(&in.PersonID), in.Section, in.Label, in.Value, in.Phone, in.ContactRef, boolInt(in.OnSheet), in.FamilyID, nullI(&uid))
		if err != nil {
			return err
		}
		in.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE info SET person_id = ?, section = ?, label = ?, value = ?, phone = ?, contact_ref = ?, on_sheet = ?, position = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		nullI(&in.PersonID), in.Section, in.Label, in.Value, in.Phone, in.ContactRef, boolInt(in.OnSheet), in.Position, Now(), in.FamilyID, in.ID)
	return err
}

// DeleteInfo removes an info line.
func (s *Store) DeleteInfo(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM info WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- who did what ----

// AddActivity writes one line of who did what.
func (s *Store) AddActivity(ctx context.Context, a *Activity) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO activity (family_id, person_id, kind, text, ref_kind, ref_id) VALUES (?, ?, ?, ?, ?, ?)`,
		a.FamilyID, nullI(&a.PersonID), a.Kind, a.Text, a.RefKind, a.RefID)
}

// ListActivity lists the latest lines, newest first.
func (s *Store) ListActivity(ctx context.Context, fid int64, since string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, family_id, COALESCE(person_id, 0), kind, text, ref_kind, ref_id, created_at FROM activity WHERE family_id = ? AND created_at >= ?
		ORDER BY id DESC LIMIT ?`, fid, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.FamilyID, &a.PersonID, &a.Kind, &a.Text, &a.RefKind, &a.RefID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- made in other apps ----

// AddMade remembers what a person made in another app.
func (s *Store) AddMade(ctx context.Context, uid int64, m *Made) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, family_id, owner, owner_id, target, app, ref, title, url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, m.FamilyID, m.Owner, m.OwnerID, m.Target, m.App, m.Ref, m.Title, m.URL)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	m.CreatedAt = Now()
	return nil
}

// MadeFor lists what a person made from one thing (owner = run, event, letter…).
func (s *Store) MadeFor(ctx context.Context, uid int64, owner string, ownerID int64) ([]Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, family_id, owner, owner_id, target, app, ref, title, url, created_at FROM made WHERE user_id = ? AND owner = ? AND owner_id = ? ORDER BY id`, uid, owner, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.FamilyID, &m.Owner, &m.OwnerID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MadeFrom lists everything a person made from a family's things of one kind.
func (s *Store) MadeFrom(ctx context.Context, uid, fid int64, owner, target string) (map[int64][]Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, family_id, owner, owner_id, target, app, ref, title, url, created_at FROM made WHERE user_id = ? AND family_id = ? AND owner = ?
		AND (? = '' OR target = ?) ORDER BY id`, uid, fid, owner, target, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.FamilyID, &m.Owner, &m.OwnerID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.CreatedAt); err != nil {
			return nil, err
		}
		out[m.OwnerID] = append(out[m.OwnerID], m)
	}
	return out, rows.Err()
}
