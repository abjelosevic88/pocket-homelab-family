package store

import (
	"context"
	"strings"
)

// ---- the family's own plans ----
//
// Single events, weekly ones (football on Tuesdays and Thursdays) and
// yearly ones (an anniversary). Repeats are worked out when read.

const eventCols = `id, family_id, title, day, end_day, start_time, end_time, place, people, kind, repeat, weekdays, until, bring, notes, COALESCE(letter_id, 0), source, created_at, updated_at`

func scanEvent(row interface{ Scan(...any) error }) (*Event, error) {
	e := &Event{}
	var people, days string
	if err := row.Scan(&e.ID, &e.FamilyID, &e.Title, &e.Day, &e.EndDay, &e.StartTime, &e.EndTime, &e.Place, &people, &e.Kind, &e.Repeat, &days, &e.Until,
		&e.Bring, &e.Notes, &e.LetterID, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	e.People, e.Weekdays = parseIDs(people), parseDays(days)
	return e, nil
}

func (s *Store) queryEvents(ctx context.Context, q string, args ...any) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+eventCols+` FROM events WHERE `+q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// ListEvents lists every event a family has (repeats as one row each).
func (s *Store) ListEvents(ctx context.Context, fid int64) ([]Event, error) {
	return s.queryEvents(ctx, `family_id = ? ORDER BY day DESC, start_time`, fid)
}

// EventsBetween lists the events on the days from…to, one row per day a
// repeating event falls on (Day set to that day).
func (s *Store) EventsBetween(ctx context.Context, fid int64, from, to string) ([]Event, error) {
	list, err := s.queryEvents(ctx, `family_id = ? AND day <= ? AND (repeat <> 'none' OR (CASE WHEN end_day = '' THEN day ELSE end_day END) >= ?) ORDER BY day, start_time`, fid, to, from)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, e := range list {
		out = append(out, Occurrences(e, from, to)...)
	}
	sortEvents(out)
	if out == nil {
		out = []Event{}
	}
	return out, nil
}

// Occurrences are the days of from…to an event is on.
func Occurrences(e Event, from, to string) []Event {
	var out []Event
	switch e.Repeat {
	case "weekly":
		days := e.Weekdays
		if len(days) == 0 {
			days = []int{Weekday(e.Day)}
		}
		start := from
		if e.Day > start {
			start = e.Day
		}
		for d := start; d <= to; d = AddDays(d, 1) {
			if e.Until != "" && d > e.Until {
				break
			}
			if HasDay(days, Weekday(d)) {
				x := e
				x.Day, x.EndDay = d, ""
				out = append(out, x)
			}
		}
	case "yearly":
		if len(e.Day) < 10 {
			return nil
		}
		for y := from[:4]; y <= to[:4]; y = nextYear(y) {
			d := y + e.Day[4:]
			if d >= from && d <= to && d >= e.Day && (e.Until == "" || d <= e.Until) {
				x := e
				x.Day, x.EndDay = d, ""
				out = append(out, x)
			}
		}
	default:
		end := e.EndDay
		if end == "" {
			end = e.Day
		}
		if e.Day <= to && end >= from {
			out = append(out, e)
		}
	}
	return out
}

func nextYear(y string) string {
	n := 0
	for _, c := range y {
		n = n*10 + int(c-'0')
	}
	n++
	b := []byte{byte('0' + n/1000%10), byte('0' + n/100%10), byte('0' + n/10%10), byte('0' + n%10)}
	return string(b)
}

func sortEvents(list []Event) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && eventLess(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func eventLess(a, b Event) bool {
	if a.Day != b.Day {
		return a.Day < b.Day
	}
	if (a.StartTime == "") != (b.StartTime == "") {
		return a.StartTime == ""
	}
	return a.StartTime < b.StartTime
}

// GetEvent is one event.
func (s *Store) GetEvent(ctx context.Context, fid, id int64) (*Event, error) {
	return scanEvent(s.DB.QueryRowContext(ctx, `SELECT `+eventCols+` FROM events WHERE family_id = ? AND id = ?`, fid, id))
}

// CreateEvent adds an event.
func (s *Store) CreateEvent(ctx context.Context, uid int64, e *Event) error {
	if e.Repeat == "" {
		e.Repeat = "none"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO events (family_id, title, day, end_day, start_time, end_time, place, people, kind, repeat, weekdays, until, bring, notes, letter_id, source, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.FamilyID, e.Title, e.Day, e.EndDay, e.StartTime, e.EndTime, e.Place, idsJSON(e.People), e.Kind, e.Repeat, daysCSV(e.Weekdays), e.Until, e.Bring, e.Notes,
		nullI(&e.LetterID), e.Source, nullI(&uid))
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// UpdateEvent saves an event.
func (s *Store) UpdateEvent(ctx context.Context, e *Event) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE events SET title = ?, day = ?, end_day = ?, start_time = ?, end_time = ?, place = ?, people = ?, kind = ?, repeat = ?, weekdays = ?, until = ?,
		bring = ?, notes = ?, updated_at = ? WHERE family_id = ? AND id = ?`,
		e.Title, e.Day, e.EndDay, e.StartTime, e.EndTime, e.Place, idsJSON(e.People), e.Kind, e.Repeat, daysCSV(e.Weekdays), e.Until,
		e.Bring, e.Notes, Now(), e.FamilyID, e.ID)
	return err
}

// DeleteEvent removes an event.
func (s *Store) DeleteEvent(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM events WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EventsOfLetter lists the events made from a letter.
func (s *Store) EventsOfLetter(ctx context.Context, fid, lid int64) ([]Event, error) {
	return s.queryEvents(ctx, `family_id = ? AND letter_id = ? ORDER BY day, start_time`, fid, lid)
}

// Fold lowercases and strips the accents a person might not type.
func Fold(s string) string {
	r := strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj", "Č", "c", "Ć", "c", "Š", "s", "Ž", "z", "Đ", "dj",
		"ä", "a", "ö", "o", "ü", "u", "ß", "ss", "é", "e", "è", "e", "à", "a", "á", "a", "í", "i", "ó", "o", "ú", "u", "ñ", "n")
	return strings.ToLower(r.Replace(s))
}
