package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- the family's own events ----

var eventKinds = []string{"school", "activity", "appointment", "family", "birthday", "other"}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	if r.URL.Query().Get("from") == "" && r.URL.Query().Get("to") == "" {
		list, err := s.store.ListEvents(ctx, f.ID)
		if err != nil {
			s.fail(w, err, "events")
			return
		}
		writeJSON(w, http.StatusOK, list)
		return
	}
	from, to, _ := s.rangeOf(r, 7)
	list, err := s.store.EventsBetween(ctx, f.ID, from, to)
	if err != nil {
		s.fail(w, err, "events")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	e, err := s.store.GetEvent(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "event")
		return
	}
	e.Made, _ = s.store.MadeFor(ctx, actorID(ctx), "event", e.ID)
	writeJSON(w, http.StatusOK, e)
}

type eventIn struct {
	Title     *string  `json:"title"`
	Day       *string  `json:"day"`
	EndDay    *string  `json:"end_day"`
	StartTime *string  `json:"start_time"`
	EndTime   *string  `json:"end_time"`
	Place     *string  `json:"place"`
	People    *[]int64 `json:"people"`
	Kind      *string  `json:"kind"`
	Repeat    *string  `json:"repeat"`
	Weekdays  *[]int   `json:"weekdays"`
	Until     *string  `json:"until"`
	Bring     *string  `json:"bring"`
	Notes     *string  `json:"notes"`
}

func (in eventIn) apply(f *store.Family, e *store.Event) string {
	trimP(&e.Title, in.Title)
	trimP(&e.Day, in.Day)
	trimP(&e.EndDay, in.EndDay)
	trimP(&e.StartTime, in.StartTime)
	trimP(&e.EndTime, in.EndTime)
	trimP(&e.Place, in.Place)
	trimP(&e.Kind, in.Kind)
	trimP(&e.Repeat, in.Repeat)
	trimP(&e.Until, in.Until)
	trimP(&e.Bring, in.Bring)
	trimP(&e.Notes, in.Notes)
	if in.People != nil {
		e.People = *in.People
	}
	if in.Weekdays != nil {
		e.Weekdays = *in.Weekdays
	}
	if e.EndDay == e.Day {
		e.EndDay = ""
	}
	switch {
	case e.Title == "":
		return "say what it is"
	case e.Day == "" || !validDate(e.Day) || !validDate(e.EndDay) || !validDate(e.Until):
		return "dates are YYYY-MM-DD"
	case e.EndDay != "" && e.EndDay < e.Day:
		return "it cannot end before it starts"
	case !validTime(e.StartTime) || !validTime(e.EndTime):
		return "a time is HH:MM"
	case !oneOf(e.Kind, eventKinds):
		return "unknown kind of event"
	case !oneOf(e.Repeat, []string{"none", "weekly", "yearly"}):
		return "an event happens once, weekly or yearly"
	case !peopleIn(f, e.People):
		return "that person is not in this family"
	}
	return ""
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	var in eventIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e := &store.Event{FamilyID: f.ID, Kind: "family", Repeat: "none", Source: linkApp(ctx), People: []int64{}}
	if msg := in.apply(f, e); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateEvent(ctx, actorID(ctx), e); err != nil {
		s.fail(w, err, "add event")
		return
	}
	s.act(ctx, "event", "added "+e.Title+" ("+dayWords(e.Day)+")", "event", e.ID)
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	e, err := s.store.GetEvent(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "event")
		return
	}
	if app := linkApp(ctx); app != "" && e.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only change the events it made itself")
		return
	}
	var in eventIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(f, e); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateEvent(ctx, e); err != nil {
		s.fail(w, err, "save event")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	e, err := s.store.GetEvent(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "event")
		return
	}
	if app := linkApp(ctx); app != "" && e.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the events it made itself")
		return
	}
	if err := s.store.DeleteEvent(ctx, f.ID, e.ID); err != nil {
		s.fail(w, err, "remove event")
		return
	}
	s.act(ctx, "event", "took "+e.Title+" off the plans", "event", 0)
	w.WriteHeader(http.StatusNoContent)
}

// ---- the board ----

func (s *Server) handlePosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	list, err := s.store.ListPosts(ctx, f.ID, f.Me.ID, queryBool(r, "all"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "board")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type postIn struct {
	Body   *string `json:"body"`
	Pinned *bool   `json:"pinned"`
	Until  *string `json:"until"`
	// also tell the family chat (Pocket Chat), when the person has one there
	Chat string `json:"chat"`
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.Post{FamilyID: f.ID, PersonID: f.Me.ID, Source: linkApp(ctx)}
	trimP(&p.Body, in.Body)
	trimP(&p.Until, in.Until)
	if in.Pinned != nil {
		p.Pinned = *in.Pinned
	}
	if p.Body == "" {
		writeErr(w, http.StatusBadRequest, "write something")
		return
	}
	if !validDate(p.Until) {
		writeErr(w, http.StatusBadRequest, "a date is YYYY-MM-DD")
		return
	}
	p.Body = clip(p.Body, 2000)
	if err := s.store.CreatePost(ctx, p); err != nil {
		s.fail(w, err, "post")
		return
	}
	s.act(ctx, "post", "wrote on the board: "+clip(firstLine(p.Body), 80), "post", p.ID)
	var others []int64
	for _, x := range accounts(f) {
		if x.ID != f.Me.ID {
			others = append(others, x.ID)
		}
	}
	s.tell(ctx, f, others, "posts", nameOf(f, f.Me.ID)+" on the family board", clip(p.Body, 300), "/board")
	s.publish(ctx, actorID(ctx), "family.post_added", "post:"+itoa(p.ID), map[string]any{"family_id": f.ID, "post_id": p.ID, "text": clip(p.Body, 500), "by": nameOf(f, f.Me.ID)})
	out := map[string]any{"post": p}
	if strings.TrimSpace(in.Chat) != "" {
		if m, err := s.toChat(ctx, f, in.Chat, nameOf(f, f.Me.ID)+" on the family board: "+p.Body, "post", p.ID, nil); err != nil {
			out["chat_error"] = err.Error()
		} else {
			out["chat"] = m
		}
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	p, err := s.store.GetPost(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	if p.PersonID != f.Me.ID && !f.Me.CanEdit() {
		writeErr(w, http.StatusForbidden, "you can change your own notes")
		return
	}
	if app := linkApp(ctx); app != "" && p.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only change the notes it wrote itself")
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	trimP(&p.Body, in.Body)
	trimP(&p.Until, in.Until)
	if in.Pinned != nil {
		p.Pinned = *in.Pinned
	}
	if p.Body == "" || !validDate(p.Until) {
		writeErr(w, http.StatusBadRequest, "write something (and a date as YYYY-MM-DD)")
		return
	}
	if err := s.store.SavePost(ctx, p); err != nil {
		s.fail(w, err, "save post")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	p, err := s.store.GetPost(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	if p.PersonID != f.Me.ID && !f.Me.CanEdit() {
		writeErr(w, http.StatusForbidden, "you can remove your own notes")
		return
	}
	if app := linkApp(ctx); app != "" && p.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the notes it wrote itself")
		return
	}
	if err := s.store.DeletePost(ctx, f.ID, p.ID); err != nil {
		s.fail(w, err, "remove post")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSeePosts(w http.ResponseWriter, r *http.Request) {
	f := famFrom(r.Context())
	if err := s.store.SeePosts(r.Context(), f.ID, f.Me.ID); err != nil {
		s.fail(w, err, "seen")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- family info ----

var infoSections = []string{"emergency", "health", "school", "home", "sitter", "other"}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListInfo(r.Context(), famID(r.Context()))
	if err != nil {
		s.fail(w, err, "info")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type infoIn struct {
	PersonID   *int64  `json:"person_id"`
	Section    *string `json:"section"`
	Label      *string `json:"label"`
	Value      *string `json:"value"`
	Phone      *string `json:"phone"`
	ContactRef *string `json:"contact_ref"`
	OnSheet    *bool   `json:"on_sheet"`
	Position   *int    `json:"position"`
}

func (s *Server) handleSaveInfo(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	in := &store.Info{FamilyID: f.ID, Section: "other", OnSheet: true}
	if id := idParam(r, "id"); id > 0 {
		cur, err := s.store.GetInfo(ctx, f.ID, id)
		if err != nil {
			s.fail(w, err, "info")
			return
		}
		in = cur
	}
	var b infoIn
	if err := decode(r, &b); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	trimP(&in.Section, b.Section)
	trimP(&in.Label, b.Label)
	trimP(&in.Value, b.Value)
	trimP(&in.Phone, b.Phone)
	trimP(&in.ContactRef, b.ContactRef)
	if b.PersonID != nil {
		in.PersonID = *b.PersonID
	}
	if b.OnSheet != nil {
		in.OnSheet = *b.OnSheet
	}
	if b.Position != nil {
		in.Position = *b.Position
	}
	switch {
	case in.Label == "":
		writeErr(w, http.StatusBadRequest, "say what it is (\"Doctor\", \"Allergies\", \"Wi-Fi\")")
		return
	case !oneOf(in.Section, infoSections):
		writeErr(w, http.StatusBadRequest, "unknown section")
		return
	case !personIn(f, in.PersonID):
		writeErr(w, http.StatusBadRequest, "that person is not in this family")
		return
	}
	created := in.ID == 0
	if err := s.store.SaveInfo(ctx, actorID(ctx), in); err != nil {
		s.fail(w, err, "save info")
		return
	}
	if created {
		s.act(ctx, "info", "added "+in.Label+" to the family info", "info", in.ID)
		writeJSON(w, http.StatusCreated, in)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleDeleteInfo(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	if err := s.store.DeleteInfo(r.Context(), famID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "remove info")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
