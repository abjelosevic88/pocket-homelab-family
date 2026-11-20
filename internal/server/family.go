package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- the family a request is about ----
//
// A person may be in more than one family (two homes, grandparents). The
// family is the one the request names (X-Family header or ?family=), else
// the one they last chose, else their first. Not being in it is a 404, the
// same as it not existing.

const famKey ctxKey = 3

func famFrom(ctx context.Context) *store.Family {
	f, _ := ctx.Value(famKey).(*store.Family)
	return f
}

// meFrom is the caller as a person in the family.
func meFrom(ctx context.Context) *store.Person {
	if f := famFrom(ctx); f != nil {
		return f.Me
	}
	return nil
}

func famID(ctx context.Context) int64 {
	if f := famFrom(ctx); f != nil {
		return f.ID
	}
	return 0
}

// familyOf resolves the family for a person and a wish ("" = their choice).
func (s *Server) familyOf(ctx context.Context, u *store.User, want string) (*store.Family, error) {
	if id, _ := strconv.ParseInt(want, 10, 64); id > 0 {
		return s.store.FamilyFor(ctx, u.ID, id)
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	if st.FamilyID > 0 {
		if f, err := s.store.FamilyFor(ctx, u.ID, st.FamilyID); err == nil {
			return f, nil
		}
	}
	list, err := s.store.MyFamilies(ctx, u.ID, u.Email)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, store.ErrNotFound
	}
	return s.store.FamilyFor(ctx, u.ID, list[0].ID)
}

// needFamily puts the request's family in the context.
func (s *Server) needFamily(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		want := firstNonEmpty(r.Header.Get("X-Family"), r.URL.Query().Get("family"))
		f, err := s.familyOf(r.Context(), u, want)
		if err != nil {
			if want == "" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "make a family first, or ask to be added to one", "code": "no_family"})
				return
			}
			s.fail(w, err, "family")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), famKey, f)))
	})
}

// mayFamily is needFamily for the export and import: a person who is in no
// family yet goes on without one (the handlers cope).
func (s *Server) mayFamily(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		want := firstNonEmpty(r.Header.Get("X-Family"), r.URL.Query().Get("family"))
		f, err := s.familyOf(r.Context(), u, want)
		if err != nil {
			if want == "" && errors.Is(err, store.ErrNotFound) {
				next.ServeHTTP(w, r)
				return
			}
			s.fail(w, err, "family")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), famKey, f)))
	})
}

// canEdit says no to children for planning work (403).
func canEdit(w http.ResponseWriter, r *http.Request) bool {
	if !meFrom(r.Context()).CanEdit() {
		writeErr(w, http.StatusForbidden, "ask a grown-up to change this")
		return false
	}
	return true
}

func canManage(w http.ResponseWriter, r *http.Request) bool {
	if !meFrom(r.Context()).CanManage() {
		writeErr(w, http.StatusForbidden, "only the people who look after the family settings can do that")
		return false
	}
	return true
}

// personIn reports whether an id is someone in the family (0 = nobody, allowed).
func personIn(f *store.Family, id int64) bool {
	if id == 0 {
		return true
	}
	for _, p := range f.People {
		if p.ID == id {
			return true
		}
	}
	return false
}

func peopleIn(f *store.Family, ids []int64) bool {
	for _, id := range ids {
		if !personIn(f, id) {
			return false
		}
	}
	return true
}

func personByID(f *store.Family, id int64) *store.Person {
	for i := range f.People {
		if f.People[i].ID == id {
			return &f.People[i]
		}
	}
	return nil
}

// nameOf is a person's first name ("" when unknown).
func nameOf(f *store.Family, id int64) string {
	if p := personByID(f, id); p != nil {
		return firstWord(p.Name)
	}
	return ""
}

func namesOf(f *store.Family, ids []int64) string {
	var out []string
	for _, id := range ids {
		if n := nameOf(f, id); n != "" {
			out = append(out, n)
		}
	}
	return andList(out)
}

// andList joins words as people say them: "Luka, Mila and Ana".
func andList(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// act writes one line of who did what in the family.
func (s *Server) act(ctx context.Context, kind, text, refKind string, refID int64) {
	f, me := famFrom(ctx), meFrom(ctx)
	if f == nil {
		return
	}
	a := &store.Activity{FamilyID: f.ID, Kind: kind, Text: text, RefKind: refKind, RefID: refID}
	if me != nil {
		a.PersonID = me.ID
	}
	if app := linkApp(ctx); app != "" && !strings.Contains(text, "Pocket") {
		a.Text += " (from Pocket " + strings.ToUpper(app[:1]) + app[1:] + ")"
	}
	s.store.AddActivity(ctx, a)
}

// ---- families ----

func (s *Server) handleFamilies(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	list, err := s.store.MyFamilies(r.Context(), u.ID, u.Email)
	if err != nil {
		s.fail(w, err, "families")
		return
	}
	if list == nil {
		list = []store.Family{}
	}
	st, _ := s.store.GetSettings(r.Context(), u.ID)
	cur := int64(0)
	for _, f := range list {
		if f.ID == st.FamilyID {
			cur = f.ID
		}
	}
	if cur == 0 && len(list) > 0 {
		cur = list[0].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"families": list, "current": cur})
}

type familyIn struct {
	Name  *string `json:"name"`
	Emoji *string `json:"emoji"`
	Color *string `json:"color"`
	// first family: how the caller appears in it
	MyName  string `json:"my_name"`
	MyColor string `json:"my_color"`
}

func (s *Server) handleCreateFamily(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if linkApp(r.Context()) != "" {
		writeErr(w, http.StatusForbidden, "families are made in Pocket Family itself")
		return
	}
	var in familyIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := &store.Family{Name: "Our family", Emoji: "🏡", Color: "#e8603c"}
	trimP(&f.Name, in.Name)
	trimP(&f.Emoji, in.Emoji)
	trimP(&f.Color, in.Color)
	if f.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the family a name")
		return
	}
	me := &store.Person{Name: firstNonEmpty(strings.TrimSpace(in.MyName), u.Name, strings.Split(u.Email, "@")[0]), Email: u.Email, Role: store.RoleGrownup,
		Color: firstNonEmpty(in.MyColor, palette[0])}
	if err := s.store.CreateFamily(r.Context(), u.ID, f, me); err != nil {
		s.fail(w, err, "create family")
		return
	}
	st, _ := s.store.GetSettings(r.Context(), u.ID)
	st.FamilyID = f.ID
	_ = s.store.SaveUserSettings(r.Context(), u.ID, st)
	out, err := s.store.FamilyFor(r.Context(), u.ID, f.ID)
	if err != nil {
		s.fail(w, err, "family")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetFamily(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, famFrom(r.Context()))
}

func (s *Server) handleUpdateFamily(w http.ResponseWriter, r *http.Request) {
	if !canManage(w, r) {
		return
	}
	var in familyIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := famFrom(r.Context())
	trimP(&f.Name, in.Name)
	trimP(&f.Emoji, in.Emoji)
	trimP(&f.Color, in.Color)
	if f.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the family a name")
		return
	}
	if err := s.store.UpdateFamily(r.Context(), f); err != nil {
		s.fail(w, err, "update family")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFamily(w http.ResponseWriter, r *http.Request) {
	if !canManage(w, r) || linkApp(r.Context()) != "" {
		if linkApp(r.Context()) != "" {
			writeErr(w, http.StatusForbidden, "a family is deleted in Pocket Family itself")
		}
		return
	}
	if err := s.store.DeleteFamily(r.Context(), famID(r.Context())); err != nil {
		s.fail(w, err, "delete family")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSwitchFamily remembers which family the caller looks at.
func (s *Server) handleSwitchFamily(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u := userFrom(r.Context())
	f, err := s.store.FamilyFor(r.Context(), u.ID, in.ID)
	if err != nil {
		s.fail(w, err, "family")
		return
	}
	st, _ := s.store.GetSettings(r.Context(), u.ID)
	st.FamilyID = f.ID
	if err := s.store.SaveUserSettings(r.Context(), u.ID, st); err != nil {
		s.fail(w, err, "save")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// handleLeaveFamily lets the caller's account go from the family (the
// person stays, for the history; a manager re-adds them by email).
func (s *Server) handleLeaveFamily(w http.ResponseWriter, r *http.Request) {
	f, me := famFrom(r.Context()), meFrom(r.Context())
	if me.Manager && s.store.CountManagers(r.Context(), f.ID) <= 1 && len(accounts(f)) > 1 {
		writeErr(w, http.StatusBadRequest, "let someone else look after the family first (Family → People)")
		return
	}
	if err := s.store.UnlinkPerson(r.Context(), f.ID, me.ID); err != nil {
		s.fail(w, err, "leave")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// accounts are the people with an account here.
func accounts(f *store.Family) []store.Person {
	var out []store.Person
	for _, p := range f.People {
		if p.UserID > 0 && !p.Archived {
			out = append(out, p)
		}
	}
	return out
}

// ---- people ----

// palette gives new people a colour of their own.
var palette = []string{"#e8603c", "#2b7a9e", "#7c3aed", "#0c8a6a", "#d6336c", "#b0571d", "#4263eb", "#567f0c"}

type personBody struct {
	Email      *string `json:"email"`
	Name       *string `json:"name"`
	Role       *string `json:"role"`
	Manager    *bool   `json:"manager"`
	Color      *string `json:"color"`
	Emoji      *string `json:"emoji"`
	Birthday   *string `json:"birthday"`
	School     *string `json:"school"`
	ClassName  *string `json:"class_name"`
	Phone      *string `json:"phone"`
	Notes      *string `json:"notes"`
	ContactRef *string `json:"contact_ref"`
	Position   *int    `json:"position"`
	Archived   *bool   `json:"archived"`
}

func (in personBody) apply(p *store.Person) string {
	trimP(&p.Email, in.Email)
	trimP(&p.Name, in.Name)
	trimP(&p.Role, in.Role)
	trimP(&p.Color, in.Color)
	trimP(&p.Emoji, in.Emoji)
	trimP(&p.Birthday, in.Birthday)
	trimP(&p.School, in.School)
	trimP(&p.ClassName, in.ClassName)
	trimP(&p.Phone, in.Phone)
	trimP(&p.Notes, in.Notes)
	trimP(&p.ContactRef, in.ContactRef)
	if in.Manager != nil {
		p.Manager = *in.Manager
	}
	if in.Position != nil {
		p.Position = *in.Position
	}
	if in.Archived != nil {
		p.Archived = *in.Archived
	}
	if p.Name == "" {
		return "give them a name"
	}
	if !oneOf(p.Role, []string{store.RoleGrownup, store.RoleChild, store.RoleHelper}) {
		return "someone is a grown-up, a child or a helper"
	}
	if p.Birthday != "" && !validBirthday(p.Birthday) {
		return "a birthday is YYYY-MM-DD (or --MM-DD without the year)"
	}
	if p.Email != "" && !strings.Contains(p.Email, "@") {
		return "that email does not look right"
	}
	if p.Role != store.RoleGrownup {
		p.Manager = false
	}
	return ""
}

func validBirthday(b string) bool {
	if strings.HasPrefix(b, "--") && len(b) == 7 {
		return validDate("2000" + b[1:])
	}
	return len(b) == 10 && validDate(b)
}

func (s *Server) handleCreatePerson(w http.ResponseWriter, r *http.Request) {
	if !canManage(w, r) {
		return
	}
	var in personBody
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f := famFrom(r.Context())
	p := &store.Person{FamilyID: f.ID, Role: store.RoleChild, Color: palette[len(f.People)%len(palette)]}
	if msg := in.apply(p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreatePerson(r.Context(), p); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "that account is already in this family")
			return
		}
		s.fail(w, err, "add person")
		return
	}
	p, _ = s.store.GetPerson(r.Context(), f.ID, p.ID)
	s.act(r.Context(), "person", "added "+p.Name+" to the family", "person", p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	f, me := famFrom(r.Context()), meFrom(r.Context())
	p, err := s.store.GetPerson(r.Context(), f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "person")
		return
	}
	// everyone may change their own name, colour, emoji and phone; the rest is for managers
	var in personBody
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !me.Manager {
		if p.ID != me.ID {
			writeErr(w, http.StatusForbidden, "only the people who look after the family settings can change someone else")
			return
		}
		in = personBody{Name: in.Name, Color: in.Color, Emoji: in.Emoji, Phone: in.Phone, Birthday: in.Birthday}
	}
	wasManager := p.Manager
	if msg := in.apply(p); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if wasManager && (!p.Manager || p.Archived) && s.store.CountManagers(r.Context(), f.ID) <= 1 {
		writeErr(w, http.StatusBadRequest, "someone has to look after the family settings")
		return
	}
	if err := s.store.UpdatePerson(r.Context(), p); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "that account is already in this family")
			return
		}
		s.fail(w, err, "save person")
		return
	}
	p, _ = s.store.GetPerson(r.Context(), f.ID, p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePerson(w http.ResponseWriter, r *http.Request) {
	if !canManage(w, r) {
		return
	}
	f, me := famFrom(r.Context()), meFrom(r.Context())
	id := idParam(r, "id")
	if id == me.ID {
		writeErr(w, http.StatusBadRequest, "to leave the family, use Leave this family")
		return
	}
	p, err := s.store.GetPerson(r.Context(), f.ID, id)
	if err != nil {
		s.fail(w, err, "person")
		return
	}
	if err := s.store.DeletePerson(r.Context(), f.ID, id); err != nil {
		s.fail(w, err, "remove person")
		return
	}
	s.act(r.Context(), "person", "took "+p.Name+" off the family", "person", 0)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListActivity(r.Context(), famID(r.Context()), "", queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
