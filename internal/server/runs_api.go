package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- "who's picking up?" ----
//
// Runs are the pick-ups, drop-offs and lifts of one day. Anyone grown-up can
// take one ("I'll do it"), the person down for it can ask for someone else
// to take it (the others hear about it), and tick it off when it is done.
// Children see them but do not change them.

var runKinds = []string{"pickup", "dropoff", "lift"}

// runWords is a run as people say it: "Pick up Mila (School)".
func runWords(f *store.Family, r *store.Run) string {
	verb := map[string]string{"pickup": "Pick up", "dropoff": "Drop off", "lift": "Lift for"}[r.Kind]
	if verb == "" {
		verb = "Run for"
	}
	who := namesOf(f, r.Children)
	out := strings.TrimSpace(verb + " " + who)
	if who == "" {
		out = firstNonEmpty(r.Title, verb)
	}
	if r.Place != "" {
		out += " (" + r.Place + ")"
	} else if r.Title != "" && who != "" {
		out += " (" + r.Title + ")"
	}
	return out
}

// whenWords is "today 15:30", "tomorrow", "Tue 4 Nov 08:00".
func whenWords(day, hhmm string) string {
	out := dayWords(day)
	if hhmm != "" {
		out += " " + hhmm
	}
	return out
}

func dayWords(day string) string {
	today := store.Today()
	switch day {
	case today:
		return "today"
	case store.AddDays(today, 1):
		return "tomorrow"
	case store.AddDays(today, -1):
		return "yesterday"
	}
	t, err := store.ParseDate(day)
	if err != nil {
		return day
	}
	if day > today && day < store.AddDays(today, 7) {
		return t.Format("Monday")
	}
	return t.Format("Mon 2 Jan")
}

func validTime(t string) bool {
	if t == "" {
		return true
	}
	_, err := time.Parse("15:04", t)
	return err == nil
}

func (s *Server) rangeOf(r *http.Request, days int) (string, string, string) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" || !validDate(from) {
		from = store.Today()
	}
	if to == "" || !validDate(to) || to < from {
		to = store.AddDays(from, days-1)
	}
	if to > store.AddDays(from, 92) {
		to = store.AddDays(from, 92)
	}
	return from, to, ""
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	from, to, _ := s.rangeOf(r, 7)
	runs, err := s.store.ListRuns(ctx, f.ID, from, to)
	if err != nil {
		s.fail(w, err, "runs")
		return
	}
	made, _ := s.store.MadeFrom(ctx, actorID(ctx), f.ID, "run", "")
	for i := range runs {
		runs[i].Made = made[runs[i].ID]
	}
	if mine := queryBool(r, "mine"); mine {
		var out []store.Run
		for _, x := range runs {
			if x.DriverID == f.Me.ID {
				out = append(out, x)
			}
		}
		runs = append([]store.Run{}, out...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "runs": runs})
}

type runIn struct {
	Day      *string  `json:"day"`
	Time     *string  `json:"time"`
	Kind     *string  `json:"kind"`
	Title    *string  `json:"title"`
	Place    *string  `json:"place"`
	Children *[]int64 `json:"children"`
	DriverID *int64   `json:"driver_id"`
	Notes    *string  `json:"notes"`
}

func (in runIn) apply(f *store.Family, x *store.Run) string {
	trimP(&x.Day, in.Day)
	trimP(&x.Time, in.Time)
	trimP(&x.Kind, in.Kind)
	trimP(&x.Title, in.Title)
	trimP(&x.Place, in.Place)
	trimP(&x.Notes, in.Notes)
	if in.Children != nil {
		x.Children = *in.Children
	}
	if in.DriverID != nil {
		x.DriverID = *in.DriverID
	}
	switch {
	case x.Day == "" || !validDate(x.Day):
		return "say which day (YYYY-MM-DD)"
	case !validTime(x.Time):
		return "a time is HH:MM"
	case !oneOf(x.Kind, runKinds):
		return "a run is a pickup, a dropoff or a lift"
	case !peopleIn(f, x.Children) || !personIn(f, x.DriverID):
		return "that person is not in this family"
	case len(x.Children) == 0 && x.Title == "":
		return "say who it is for, or what it is"
	}
	return ""
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	var in runIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	x := &store.Run{FamilyID: f.ID, Kind: "pickup", Source: linkApp(ctx)}
	if msg := in.apply(f, x); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateRun(ctx, actorID(ctx), x); err != nil {
		s.fail(w, err, "add run")
		return
	}
	s.act(ctx, "run", "planned "+lower1(runWords(f, x))+" "+whenWords(x.Day, x.Time), "run", x.ID)
	if x.DriverID > 0 && x.DriverID != f.Me.ID {
		s.tell(ctx, f, []int64{x.DriverID}, "runs", "You're down for a "+kindWord(x.Kind), runWords(f, x)+", "+whenWords(x.Day, x.Time)+" — "+nameOf(f, f.Me.ID)+" put you down.", "/pickups?day="+x.Day)
	}
	if x.DriverID == 0 {
		s.askAround(ctx, f, x, "")
	}
	writeJSON(w, http.StatusCreated, x)
}

func kindWord(k string) string {
	return map[string]string{"pickup": "pick-up", "dropoff": "drop-off", "lift": "lift"}[k]
}

func lower1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func (s *Server) runFor(w http.ResponseWriter, r *http.Request) (*store.Family, *store.Run, bool) {
	f := famFrom(r.Context())
	x, err := s.store.GetRun(r.Context(), f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "run")
		return nil, nil, false
	}
	return f, x, true
}

func (s *Server) handleUpdateRun(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f, x, ok := s.runFor(w, r)
	if !ok {
		return
	}
	if app := linkApp(ctx); app != "" && x.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only change the runs it made itself")
		return
	}
	var in runIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	before := x.DriverID
	if msg := in.apply(f, x); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if x.DriverID != before && x.State == store.RunAsking {
		x.State, x.AskedBy, x.AskedNote = store.RunPlanned, 0, ""
	}
	if err := s.store.SaveRun(ctx, x); err != nil {
		s.fail(w, err, "save run")
		return
	}
	if x.DriverID != before && x.DriverID > 0 && x.DriverID != f.Me.ID {
		s.tell(ctx, f, []int64{x.DriverID}, "runs", "You're down for a "+kindWord(x.Kind), runWords(f, x)+", "+whenWords(x.Day, x.Time)+" — "+nameOf(f, f.Me.ID)+" put you down.", "/pickups?day="+x.Day)
	}
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f, x, ok := s.runFor(w, r)
	if !ok {
		return
	}
	if app := linkApp(ctx); app != "" && x.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the runs it made itself")
		return
	}
	if err := s.store.DeleteRun(ctx, f.ID, x.ID); err != nil {
		s.fail(w, err, "remove run")
		return
	}
	s.act(ctx, "run", "called off "+lower1(runWords(f, x))+" "+whenWords(x.Day, x.Time), "run", x.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleRunAction: take | ask | done | undo | skip | assign.
func (s *Server) handleRunAction(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f, x, ok := s.runFor(w, r)
	if !ok {
		return
	}
	var in struct {
		Note     string `json:"note"`
		PersonID int64  `json:"person_id"`
	}
	_ = decode(r, &in)
	me := f.Me
	what, when := runWords(f, x), whenWords(x.Day, x.Time)
	action := chiParam(r, "action")
	switch action {
	case "take":
		prev, asker := x.DriverID, x.AskedBy
		x.DriverID, x.State, x.AskedBy, x.AskedNote = me.ID, store.RunPlanned, 0, ""
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "take")
			return
		}
		s.act(ctx, "run", "will "+lower1(what)+" "+when, "run", x.ID)
		var tell []int64
		for _, id := range []int64{prev, asker} {
			if id > 0 && id != me.ID && !store.HasID(tell, id) {
				tell = append(tell, id)
			}
		}
		s.tell(ctx, f, tell, "runs", nameOf(f, me.ID)+" will do it", what+", "+when+".", "/pickups?day="+x.Day)
		s.publish(ctx, actorID(ctx), "family.run_taken", fmt.Sprintf("run:%d", x.ID), runPayload(f, x))
	case "ask":
		if x.State == store.RunDone {
			writeErr(w, http.StatusBadRequest, "that one is already done")
			return
		}
		x.State, x.AskedBy, x.AskedNote = store.RunAsking, me.ID, clip(strings.TrimSpace(in.Note), 300)
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "ask")
			return
		}
		s.act(ctx, "run", "asked who can "+lower1(what)+" "+when, "run", x.ID)
		s.askAround(ctx, f, x, x.AskedNote)
	case "assign":
		if in.PersonID == 0 || !personIn(f, in.PersonID) {
			writeErr(w, http.StatusBadRequest, "say who takes it")
			return
		}
		x.DriverID, x.State, x.AskedBy, x.AskedNote = in.PersonID, store.RunPlanned, 0, ""
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "assign")
			return
		}
		s.act(ctx, "run", "put "+nameOf(f, in.PersonID)+" down to "+lower1(what)+" "+when, "run", x.ID)
		if in.PersonID != me.ID {
			s.tell(ctx, f, []int64{in.PersonID}, "runs", "You're down for a "+kindWord(x.Kind), what+", "+when+" — "+nameOf(f, me.ID)+" put you down.", "/pickups?day="+x.Day)
		}
	case "done":
		x.State, x.DoneAt, x.DoneBy = store.RunDone, store.Now(), me.ID
		if x.DriverID == 0 {
			x.DriverID = me.ID
		}
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "done")
			return
		}
		s.act(ctx, "run", "did "+lower1(what), "run", x.ID)
		s.publish(ctx, actorID(ctx), "family.run_done", fmt.Sprintf("run:%d", x.ID), runPayload(f, x))
	case "undo":
		x.State, x.DoneAt, x.DoneBy, x.AskedBy, x.AskedNote = store.RunPlanned, "", 0, 0, ""
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "undo")
			return
		}
	case "skip":
		x.State = store.RunSkipped
		if err := s.store.SaveRun(ctx, x); err != nil {
			s.fail(w, err, "skip")
			return
		}
		s.act(ctx, "run", "said "+lower1(what)+" "+when+" is not needed", "run", x.ID)
		if x.DriverID > 0 && x.DriverID != me.ID {
			s.tell(ctx, f, []int64{x.DriverID}, "runs", "Not needed: "+what, when+" — "+nameOf(f, me.ID)+" called it off.", "/pickups?day="+x.Day)
		}
	default:
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, x)
}

// askAround tells the family's other grown-ups and helpers that a run needs someone.
func (s *Server) askAround(ctx context.Context, f *store.Family, x *store.Run, note string) {
	var ids []int64
	for _, p := range accounts(f) {
		if p.Role != store.RoleChild && p.ID != f.Me.ID {
			ids = append(ids, p.ID)
		}
	}
	body := runWords(f, x) + ", " + whenWords(x.Day, x.Time) + "."
	if note != "" {
		body += " " + nameOf(f, f.Me.ID) + ": \"" + note + "\""
	}
	s.tell(ctx, f, ids, "runs", "Who can do it?", body+" Tap to take it.", "/pickups?day="+x.Day)
	s.publish(ctx, actorID(ctx), "family.run_needs_cover", fmt.Sprintf("run:%d", x.ID), runPayload(f, x))
}

func runPayload(f *store.Family, x *store.Run) map[string]any {
	return map[string]any{"family_id": f.ID, "family": f.Name, "run_id": x.ID, "day": x.Day, "time": x.Time, "kind": x.Kind, "title": runWords(f, x),
		"place": x.Place, "children": namesOf(f, x.Children), "driver": nameOf(f, x.DriverID), "state": x.State}
}

// ---- weekly plans ----

func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPlans(r.Context(), famID(r.Context()))
	if err != nil {
		s.fail(w, err, "plans")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type planIn struct {
	Kind     *string           `json:"kind"`
	Title    *string           `json:"title"`
	Place    *string           `json:"place"`
	Weekdays *[]int            `json:"weekdays"`
	Time     *string           `json:"time"`
	Children *[]int64          `json:"children"`
	DriverID *int64            `json:"driver_id"`
	Rota     *map[string]int64 `json:"rota"`
	TermOnly *bool             `json:"term_only"`
	StartsOn *string           `json:"starts_on"`
	EndsOn   *string           `json:"ends_on"`
	Notes    *string           `json:"notes"`
	Active   *bool             `json:"active"`
}

func (in planIn) apply(f *store.Family, p *store.RunPlan) string {
	trimP(&p.Kind, in.Kind)
	trimP(&p.Title, in.Title)
	trimP(&p.Place, in.Place)
	trimP(&p.Time, in.Time)
	trimP(&p.StartsOn, in.StartsOn)
	trimP(&p.EndsOn, in.EndsOn)
	trimP(&p.Notes, in.Notes)
	if in.Weekdays != nil {
		p.Weekdays = *in.Weekdays
	}
	if in.Children != nil {
		p.Children = *in.Children
	}
	if in.DriverID != nil {
		p.DriverID = *in.DriverID
	}
	if in.Rota != nil {
		p.Rota = *in.Rota
	}
	if in.TermOnly != nil {
		p.TermOnly = *in.TermOnly
	}
	if in.Active != nil {
		p.Active = *in.Active
	}
	for _, id := range p.Rota {
		if !personIn(f, id) {
			return "that person is not in this family"
		}
	}
	switch {
	case !oneOf(p.Kind, runKinds):
		return "a run is a pickup, a dropoff or a lift"
	case len(p.Weekdays) == 0:
		return "pick the days of the week"
	case p.Time == "" || !validTime(p.Time):
		return "a time is HH:MM"
	case !validDate(p.StartsOn) || !validDate(p.EndsOn):
		return "dates are YYYY-MM-DD"
	case !peopleIn(f, p.Children) || !personIn(f, p.DriverID):
		return "that person is not in this family"
	case len(p.Children) == 0 && p.Title == "":
		return "say who it is for, or what it is"
	}
	return ""
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	var in planIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.RunPlan{FamilyID: f.ID, Kind: "pickup", Active: true, Rota: map[string]int64{}}
	if msg := in.apply(f, p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreatePlan(ctx, actorID(ctx), p); err != nil {
		s.fail(w, err, "add plan")
		return
	}
	s.act(ctx, "plan", "set up a weekly "+kindWord(p.Kind)+": "+planWords(f, p), "plan", p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func planWords(f *store.Family, p *store.RunPlan) string {
	x := &store.Run{Kind: p.Kind, Title: p.Title, Place: p.Place, Children: p.Children}
	return runWords(f, x) + " at " + p.Time
}

func (s *Server) handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	p, err := s.store.GetPlan(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	var in planIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(f, p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdatePlan(ctx, p); err != nil {
		s.fail(w, err, "save plan")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	f := famFrom(ctx)
	p, err := s.store.GetPlan(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	if err := s.store.DeletePlan(ctx, f.ID, p.ID); err != nil {
		s.fail(w, err, "remove plan")
		return
	}
	s.act(ctx, "plan", "stopped the weekly "+kindWord(p.Kind)+": "+planWords(f, p), "plan", 0)
	w.WriteHeader(http.StatusNoContent)
}
