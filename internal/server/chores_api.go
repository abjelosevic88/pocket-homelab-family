package server

import (
	"fmt"
	"net/http"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- chores ----
//
// Grown-ups plan the chores; everyone ticks off their own (a child too),
// grown-ups can tick one off for someone else. Stars add up per week.

var repeats = []string{"once", "daily", "weekly", "monthly", "every"}

func (s *Server) handleChores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	list, err := s.store.ListChores(ctx, f.ID, queryBool(r, "all"))
	if err != nil {
		s.fail(w, err, "chores")
		return
	}
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	week := store.WeekStart(store.Today(), st.WeekStart)
	log, err := s.store.ChoreLog(ctx, f.ID, store.AddDays(store.Today(), -60), store.AddDays(store.Today(), 1))
	if err != nil {
		s.fail(w, err, "chores")
		return
	}
	for i := range list {
		c := &list[i]
		c.DoneOn = []string{}
		for j := range log {
			d := log[j]
			if d.ChoreID != c.ID {
				continue
			}
			if c.LastDone == nil {
				c.LastDone = &d
			}
			if d.Day >= week {
				c.DoneOn = append(c.DoneOn, d.Day)
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type choreIn struct {
	Title     *string  `json:"title"`
	Emoji     *string  `json:"emoji"`
	Notes     *string  `json:"notes"`
	Repeat    *string  `json:"repeat"`
	EveryDays *int     `json:"every_days"`
	Weekdays  *[]int   `json:"weekdays"`
	Assign    *string  `json:"assign"`
	People    *[]int64 `json:"people"`
	Stars     *int     `json:"stars"`
	PartOfDay *string  `json:"part_of_day"`
	NextDue   *string  `json:"next_due"`
	Active    *bool    `json:"active"`
}

func (in choreIn) apply(f *store.Family, c *store.Chore) string {
	trimP(&c.Title, in.Title)
	trimP(&c.Emoji, in.Emoji)
	trimP(&c.Notes, in.Notes)
	trimP(&c.Repeat, in.Repeat)
	trimP(&c.Assign, in.Assign)
	trimP(&c.PartOfDay, in.PartOfDay)
	trimP(&c.NextDue, in.NextDue)
	if in.EveryDays != nil {
		c.EveryDays = *in.EveryDays
	}
	if in.Weekdays != nil {
		c.Weekdays = *in.Weekdays
	}
	if in.People != nil {
		c.People = *in.People
		if c.Turn >= len(c.People) {
			c.Turn = 0
		}
	}
	if in.Stars != nil {
		c.Stars = *in.Stars
	}
	if in.Active != nil {
		c.Active = *in.Active
	}
	switch {
	case c.Title == "":
		return "say what the chore is"
	case !oneOf(c.Repeat, repeats):
		return "a chore repeats once, daily, weekly, monthly or every few days"
	case c.Repeat == "every" && (c.EveryDays < 1 || c.EveryDays > 365):
		return "every 1 to 365 days"
	case !oneOf(c.Assign, []string{"one", "turns", "anyone"}):
		return "a chore is for someone, done in turns, or for anyone"
	case c.Assign != "anyone" && len(c.People) == 0:
		return "say who does it"
	case !peopleIn(f, c.People):
		return "that person is not in this family"
	case c.Stars < 0 || c.Stars > 20:
		return "0 to 20 stars"
	case !oneOf(c.PartOfDay, []string{"", "morning", "afternoon", "evening"}):
		return "part of the day is morning, afternoon or evening"
	case !validDate(c.NextDue):
		return "a date is YYYY-MM-DD"
	}
	return ""
}

func (s *Server) handleCreateChore(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	var in choreIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := &store.Chore{FamilyID: f.ID, Repeat: "weekly", Assign: "one", Source: linkApp(ctx), People: []int64{}}
	if msg := in.apply(f, c); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateChore(ctx, actorID(ctx), c); err != nil {
		s.fail(w, err, "add chore")
		return
	}
	s.act(ctx, "chore", "added the chore "+c.Title, "chore", c.ID)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateChore(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	c, err := s.store.GetChore(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "chore")
		return
	}
	if app := linkApp(ctx); app != "" && c.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only change the chores it made itself")
		return
	}
	var in choreIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(f, c); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if c.Active && c.NextDue == "" {
		c.NextDue = store.FirstDue(c, store.Today())
	}
	if err := s.store.UpdateChore(ctx, c); err != nil {
		s.fail(w, err, "save chore")
		return
	}
	c.Whose = c.WhoseTurn()
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteChore(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	c, err := s.store.GetChore(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "chore")
		return
	}
	if app := linkApp(ctx); app != "" && c.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the chores it made itself")
		return
	}
	if err := s.store.DeleteChore(ctx, f.ID, c.ID); err != nil {
		s.fail(w, err, "remove chore")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChoreDone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	me := f.Me
	c, err := s.store.GetChore(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "chore")
		return
	}
	var in struct {
		PersonID int64  `json:"person_id"`
		Day      string `json:"day"`
	}
	_ = decode(r, &in)
	who := in.PersonID
	if who == 0 {
		// not said: whoever ticks it did it
		who = me.ID
	}
	if !personIn(f, who) {
		writeErr(w, http.StatusBadRequest, "that person is not in this family")
		return
	}
	if !me.CanEdit() && who != me.ID {
		writeErr(w, http.StatusForbidden, "tick off your own chores")
		return
	}
	day := in.Day
	if day == "" || !validDate(day) || day > store.Today() {
		day = store.Today()
	}
	if !c.Active {
		writeErr(w, http.StatusBadRequest, "that chore is not on the list any more")
		return
	}
	d, err := s.store.DoChore(ctx, actorID(ctx), c, who, day)
	if err != nil {
		s.fail(w, err, "done")
		return
	}
	text := "did " + c.Title
	if who != me.ID {
		text = "ticked off " + c.Title + " for " + nameOf(f, who)
	}
	s.act(ctx, "chore", text, "chore", c.ID)
	s.publish(ctx, actorID(ctx), "family.chore_done", fmt.Sprintf("chore:%d", c.ID), map[string]any{"family_id": f.ID, "chore_id": c.ID, "title": c.Title,
		"person": nameOf(f, who), "day": day, "stars": c.Stars})
	c, _ = s.store.GetChore(ctx, f.ID, c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"done": d, "chore": c})
}

func (s *Server) handleChoreUndo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	c, err := s.store.GetChore(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "chore")
		return
	}
	if err := s.store.UndoChore(ctx, f.ID, c.ID); err != nil {
		s.fail(w, err, "undo")
		return
	}
	c, _ = s.store.GetChore(ctx, f.ID, c.ID)
	writeJSON(w, http.StatusOK, c)
}

// handleStars is each person's chores and stars this week (and last week).
func (s *Server) handleStars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	week := store.WeekStart(firstNonEmpty(r.URL.Query().Get("week"), store.Today()), st.WeekStart)
	log, err := s.store.ChoreLog(ctx, f.ID, store.AddDays(week, -7), store.AddDays(week, 6))
	if err != nil {
		s.fail(w, err, "stars")
		return
	}
	type row struct {
		PersonID  int64 `json:"person_id"`
		Done      int   `json:"done"`
		Stars     int   `json:"stars"`
		LastStars int   `json:"last_week_stars"`
	}
	by := map[int64]*row{}
	var order []int64
	for _, p := range f.People {
		if !p.Archived {
			by[p.ID] = &row{PersonID: p.ID}
			order = append(order, p.ID)
		}
	}
	for _, d := range log {
		x := by[d.PersonID]
		if x == nil {
			continue
		}
		if d.Day >= week {
			x.Done++
			x.Stars += d.Stars
		} else {
			x.LastStars += d.Stars
		}
	}
	out := []row{}
	for _, id := range order {
		out = append(out, *by[id])
	}
	recent := []store.Done{}
	for _, d := range log {
		if d.Day >= week && len(recent) < 30 {
			recent = append(recent, d)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"week": week, "people": out, "recent": recent})
}
