package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- school: terms and holidays, each child's week, letters ----

func (s *Server) handleSchool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	terms, err := s.store.ListTerms(ctx, f.ID)
	if err != nil {
		s.fail(w, err, "terms")
		return
	}
	days, err := s.store.SchoolDays(ctx, f.ID)
	if err != nil {
		s.fail(w, err, "school days")
		return
	}
	today := store.Today()
	now := map[string]any{"no_school": store.NoSchool(today, terms)}
	for _, t := range terms {
		if today >= t.StartsOn && today <= t.EndsOn {
			now["term"] = t
		}
		if t.StartsOn > today && (t.Kind == "holiday" || t.Kind == "closed") {
			if _, ok := now["next_break"]; !ok {
				now["next_break"] = t
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": terms, "days": days, "now": now})
}

func (s *Server) handleSaveTerm(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	t := &store.Term{FamilyID: f.ID, Kind: "holiday"}
	if id := idParam(r, "id"); id > 0 {
		cur, err := s.store.GetTerm(ctx, f.ID, id)
		if err != nil {
			s.fail(w, err, "term")
			return
		}
		t = cur
	}
	var in struct {
		Name     *string `json:"name"`
		Kind     *string `json:"kind"`
		StartsOn *string `json:"starts_on"`
		EndsOn   *string `json:"ends_on"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	trimP(&t.Name, in.Name)
	trimP(&t.Kind, in.Kind)
	trimP(&t.StartsOn, in.StartsOn)
	trimP(&t.EndsOn, in.EndsOn)
	if t.EndsOn == "" {
		t.EndsOn = t.StartsOn
	}
	switch {
	case t.Name == "":
		writeErr(w, http.StatusBadRequest, "give it a name (\"Winter holiday\")")
		return
	case !oneOf(t.Kind, []string{"term", "holiday", "closed"}):
		writeErr(w, http.StatusBadRequest, "a term, a holiday or a closed day")
		return
	case t.StartsOn == "" || !validDate(t.StartsOn) || !validDate(t.EndsOn) || t.EndsOn < t.StartsOn:
		writeErr(w, http.StatusBadRequest, "dates are YYYY-MM-DD, the end not before the start")
		return
	}
	created := t.ID == 0
	if err := s.store.SaveTerm(ctx, t); err != nil {
		s.fail(w, err, "save term")
		return
	}
	if created {
		s.act(ctx, "term", "added "+t.Name+" to the school year", "term", t.ID)
		writeJSON(w, http.StatusCreated, t)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTerm(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	if err := s.store.DeleteTerm(r.Context(), famID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "remove term")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSchoolWeek: PUT /school/week/{person} [{weekday, starts, ends, bring}].
func (s *Server) handleSchoolWeek(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	pid := idParam(r, "person")
	if pid == 0 || !personIn(f, pid) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var days []store.SchoolDay
	if err := decode(r, &days); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	for i := range days {
		d := &days[i]
		d.Starts, d.Ends, d.Bring = strings.TrimSpace(d.Starts), strings.TrimSpace(d.Ends), clip(strings.TrimSpace(d.Bring), 300)
		if !validTime(d.Starts) || !validTime(d.Ends) {
			writeErr(w, http.StatusBadRequest, "a time is HH:MM")
			return
		}
	}
	if err := s.store.SetSchoolWeek(ctx, f.ID, pid, days); err != nil {
		s.fail(w, err, "save week")
		return
	}
	all, _ := s.store.SchoolDays(ctx, f.ID)
	writeJSON(w, http.StatusOK, all)
}

// ---- letters ----

var letterKinds = []string{"info", "permission", "payment", "bring", "event"}

func (s *Server) handleLetters(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListLetters(r.Context(), famID(r.Context()), r.URL.Query().Get("state"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "letters")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetLetter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetLetter(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "letter")
		return
	}
	l.Events, _ = s.store.EventsOfLetter(ctx, f.ID, l.ID)
	l.Made, _ = s.store.MadeFor(ctx, actorID(ctx), "letter", l.ID)
	writeJSON(w, http.StatusOK, l)
}

type letterIn struct {
	Title      *string  `json:"title"`
	Sender     *string  `json:"sender"`
	Body       *string  `json:"body"`
	Summary    *string  `json:"summary"`
	ReceivedOn *string  `json:"received_on"`
	People     *[]int64 `json:"people"`
	Kind       *string  `json:"kind"`
	Action     *string  `json:"action"`
	DueOn      *string  `json:"due_on"`
	Amount     *int64   `json:"amount"`
	Currency   *string  `json:"currency"`
	State      *string  `json:"state"`
	// new events from the letter (the trip, the parents' evening)
	Events []eventIn `json:"events"`
}

func (in letterIn) apply(f *store.Family, l *store.Letter) string {
	trimP(&l.Title, in.Title)
	trimP(&l.Sender, in.Sender)
	trimP(&l.Body, in.Body)
	trimP(&l.Summary, in.Summary)
	trimP(&l.ReceivedOn, in.ReceivedOn)
	trimP(&l.Kind, in.Kind)
	trimP(&l.Action, in.Action)
	trimP(&l.DueOn, in.DueOn)
	trimP(&l.State, in.State)
	if in.Currency != nil {
		l.Currency = cleanCurrency(*in.Currency)
	}
	if in.People != nil {
		l.People = *in.People
	}
	if in.Amount != nil {
		l.Amount = *in.Amount
	}
	l.Body = clip(l.Body, 20000)
	switch {
	case l.Title == "":
		return "give the letter a title"
	case !validDate(l.ReceivedOn) || !validDate(l.DueOn):
		return "dates are YYYY-MM-DD"
	case !oneOf(l.Kind, letterKinds):
		return "unknown kind of letter"
	case !oneOf(l.State, []string{"todo", "done", "none"}):
		return "a letter is to do, done, or needs nothing"
	case l.Amount < 0:
		return "the amount cannot be negative"
	case !peopleIn(f, l.People):
		return "that person is not in this family"
	}
	return ""
}

func (s *Server) handleCreateLetter(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	var in letterIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	l := &store.Letter{FamilyID: f.ID, Kind: "info", State: "todo", ReceivedOn: store.Today(), Currency: st.Currency, Source: linkApp(ctx), People: []int64{}}
	if msg := in.apply(f, l); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if l.Kind == "info" && l.Action == "" && l.DueOn == "" && (in.State == nil) {
		l.State = "none"
	}
	// check the events before saving anything
	var events []*store.Event
	for _, ei := range in.Events {
		e := &store.Event{FamilyID: f.ID, Kind: "school", Repeat: "none", Source: linkApp(ctx), People: l.People}
		if msg := ei.apply(f, e); msg != "" {
			writeErr(w, http.StatusBadRequest, "event: "+msg)
			return
		}
		events = append(events, e)
	}
	if err := s.store.CreateLetter(ctx, actorID(ctx), l); err != nil {
		s.fail(w, err, "add letter")
		return
	}
	for _, e := range events {
		e.LetterID = l.ID
		if err := s.store.CreateEvent(ctx, actorID(ctx), e); err != nil {
			s.fail(w, err, "add event")
			return
		}
		l.Events = append(l.Events, *e)
	}
	s.act(ctx, "letter", "added the letter "+l.Title, "letter", l.ID)
	s.publish(ctx, actorID(ctx), "family.letter_added", "letter:"+itoa(l.ID), map[string]any{"family_id": f.ID, "letter_id": l.ID, "title": l.Title,
		"kind": l.Kind, "due_on": l.DueOn, "action": l.Action, "for": namesOf(f, l.People), "amount": l.Amount, "currency": l.Currency})
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleUpdateLetter(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetLetter(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "letter")
		return
	}
	if app := linkApp(ctx); app != "" && l.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only change the letters it added itself")
		return
	}
	var in letterIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	was := l.State
	if msg := in.apply(f, l); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if l.State == "done" && was != "done" {
		l.DoneBy, l.DoneAt = f.Me.ID, store.Now()
		s.act(ctx, "letter", "dealt with "+l.Title, "letter", l.ID)
	} else if l.State != "done" {
		l.DoneBy, l.DoneAt = 0, ""
	}
	if err := s.store.SaveLetter(ctx, l); err != nil {
		s.fail(w, err, "save letter")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleDeleteLetter(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetLetter(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "letter")
		return
	}
	if app := linkApp(ctx); app != "" && l.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the letters it added itself")
		return
	}
	if err := s.store.DeleteLetter(ctx, f.ID, l.ID); err != nil {
		s.fail(w, err, "remove letter")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
