package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- export and import ----
//
// The export is one family as JSON, people referred to by name so it can
// go into another box or another family. Importing adds what is not there
// yet (matched by name, title and day), so the same file twice adds nothing
// twice.

type exportDoc struct {
	App        string            `json:"app"`
	Version    string            `json:"version"`
	ExportedAt string            `json:"exported_at"`
	Family     exportFamily      `json:"family"`
	People     []store.Person    `json:"people"`
	Plans      []exportPlan      `json:"plans"`
	Runs       []exportRun       `json:"runs"`
	Chores     []exportChore     `json:"chores"`
	Events     []exportEvent     `json:"events"`
	Lists      []exportList      `json:"lists"`
	Terms      []store.Term      `json:"terms"`
	School     []exportSchoolDay `json:"school_days"`
	Letters    []exportLetter    `json:"letters"`
	Posts      []exportPost      `json:"posts"`
	Info       []exportInfo      `json:"info"`
}

type exportFamily struct {
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
}

type exportPlan struct {
	store.RunPlan
	ChildNames []string          `json:"child_names"`
	DriverName string            `json:"driver_name"`
	RotaNames  map[string]string `json:"rota_names"`
}

type exportRun struct {
	store.Run
	ChildNames []string `json:"child_names"`
	DriverName string   `json:"driver_name"`
}

type exportChore struct {
	store.Chore
	PeopleNames []string `json:"people_names"`
}

type exportEvent struct {
	store.Event
	PeopleNames []string `json:"people_names"`
}

type exportList struct {
	Name  string       `json:"name"`
	Emoji string       `json:"emoji"`
	Items []store.Item `json:"items"`
}

type exportSchoolDay struct {
	store.SchoolDay
	PersonName string `json:"person_name"`
}

type exportLetter struct {
	store.Letter
	PeopleNames []string `json:"people_names"`
}

type exportPost struct {
	store.Post
	AuthorName string `json:"author_name"`
}

type exportInfo struct {
	store.Info
	PersonName string `json:"person_name"`
}

func names(f *store.Family, ids []int64) []string {
	out := []string{}
	for _, id := range ids {
		if p := personByID(f, id); p != nil {
			out = append(out, p.Name)
		}
	}
	return out
}

func fullName(f *store.Family, id int64) string {
	if p := personByID(f, id); p != nil {
		return p.Name
	}
	return ""
}

func (s *Server) export(ctx context.Context, f *store.Family) (exportDoc, error) {
	doc := exportDoc{App: "pocket-family", Version: Version, ExportedAt: store.Now(), Family: exportFamily{f.Name, f.Emoji, f.Color}, People: f.People}
	plans, err := s.store.ListPlans(ctx, f.ID)
	if err != nil {
		return doc, err
	}
	for _, p := range plans {
		rota := map[string]string{}
		for k, v := range p.Rota {
			rota[k] = fullName(f, v)
		}
		doc.Plans = append(doc.Plans, exportPlan{p, names(f, p.Children), fullName(f, p.DriverID), rota})
	}
	runs, err := s.store.ListRuns(ctx, f.ID, store.AddDays(store.Today(), -7), store.AddDays(store.Today(), 60))
	if err != nil {
		return doc, err
	}
	for _, x := range runs {
		if x.PlanID == 0 {
			doc.Runs = append(doc.Runs, exportRun{x, names(f, x.Children), fullName(f, x.DriverID)})
		}
	}
	chores, _ := s.store.ListChores(ctx, f.ID, true)
	for _, c := range chores {
		doc.Chores = append(doc.Chores, exportChore{c, names(f, c.People)})
	}
	events, _ := s.store.ListEvents(ctx, f.ID)
	for _, e := range events {
		doc.Events = append(doc.Events, exportEvent{e, names(f, e.People)})
	}
	lists, _ := s.store.ListLists(ctx, f.ID)
	for _, l := range lists {
		items, _ := s.store.ListItems(ctx, f.ID, l.ID)
		var open []store.Item
		for _, it := range items {
			if !it.Done {
				open = append(open, it)
			}
		}
		doc.Lists = append(doc.Lists, exportList{l.Name, l.Emoji, nonNil(open)})
	}
	doc.Terms, _ = s.store.ListTerms(ctx, f.ID)
	days, _ := s.store.SchoolDays(ctx, f.ID)
	for _, d := range days {
		doc.School = append(doc.School, exportSchoolDay{d, fullName(f, d.PersonID)})
	}
	letters, _ := s.store.ListLetters(ctx, f.ID, "", 10000)
	for _, l := range letters {
		doc.Letters = append(doc.Letters, exportLetter{l, names(f, l.People)})
	}
	posts, _ := s.store.ListPosts(ctx, f.ID, f.Me.ID, true, 10000)
	for _, p := range posts {
		doc.Posts = append(doc.Posts, exportPost{p, fullName(f, p.PersonID)})
	}
	info, _ := s.store.ListInfo(ctx, f.ID)
	for _, in := range info {
		doc.Info = append(doc.Info, exportInfo{in, fullName(f, in.PersonID)})
	}
	return doc, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	f := famFrom(r.Context())
	doc, err := s.export(r.Context(), f)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	slug := strings.ToLower(strings.Join(strings.Fields(f.Name), "-"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-family-%s-%s.json"`, slug, time.Now().Format("20060102")))
	writeJSON(w, http.StatusOK, doc)
}

// handleImport adds a Pocket Family export to the current family.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !canManage(w, r) {
		return
	}
	ctx := r.Context()
	var doc exportDoc
	if err := decode(r, &doc); err != nil || doc.App != "pocket-family" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Family export")
		return
	}
	n, err := s.importDoc(ctx, famFrom(ctx), doc)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) importDoc(ctx context.Context, f *store.Family, doc exportDoc) (map[string]int, error) {
	uid := actorID(ctx)
	added := map[string]int{}
	// people by name (the caller stays who they are)
	byName := map[string]int64{}
	for _, p := range f.People {
		byName[store.Fold(p.Name)] = p.ID
	}
	for _, p := range doc.People {
		if p.Name == "" || byName[store.Fold(p.Name)] > 0 {
			continue
		}
		np := &store.Person{FamilyID: f.ID, Name: p.Name, Role: p.Role, Color: p.Color, Emoji: p.Emoji, Birthday: p.Birthday, School: p.School, ClassName: p.ClassName,
			Phone: p.Phone, Notes: p.Notes}
		if !oneOf(np.Role, []string{store.RoleGrownup, store.RoleChild, store.RoleHelper}) {
			np.Role = store.RoleChild
		}
		if err := s.store.CreatePerson(ctx, np); err != nil {
			return nil, err
		}
		byName[store.Fold(np.Name)] = np.ID
		added["people"]++
	}
	id := func(name string) int64 { return byName[store.Fold(name)] }
	ids := func(list []string) []int64 {
		out := []int64{}
		for _, n := range list {
			if x := id(n); x > 0 {
				out = append(out, x)
			}
		}
		return out
	}
	key := func(parts ...string) string { return store.Fold(strings.Join(parts, "|")) }

	plans, _ := s.store.ListPlans(ctx, f.ID)
	have := map[string]bool{}
	for _, p := range plans {
		have[key(p.Kind, p.Time, p.Place, p.Title)] = true
	}
	for _, p := range doc.Plans {
		if have[key(p.Kind, p.Time, p.Place, p.Title)] {
			continue
		}
		np := p.RunPlan
		np.ID, np.FamilyID, np.Children, np.DriverID = 0, f.ID, ids(p.ChildNames), id(p.DriverName)
		np.Rota = map[string]int64{}
		for k, v := range p.RotaNames {
			if x := id(v); x > 0 {
				np.Rota[k] = x
			}
		}
		if err := s.store.CreatePlan(ctx, uid, &np); err != nil {
			return nil, err
		}
		have[key(p.Kind, p.Time, p.Place, p.Title)] = true
		added["plans"]++
	}
	runs, _ := s.store.ListRuns(ctx, f.ID, store.AddDays(store.Today(), -7), store.AddDays(store.Today(), 70))
	have = map[string]bool{}
	for _, x := range runs {
		have[key(x.Day, x.Time, x.Kind, x.Place, x.Title)] = true
	}
	for _, x := range doc.Runs {
		k := key(x.Day, x.Time, x.Kind, x.Place, x.Title)
		if have[k] || x.Day == "" {
			continue
		}
		nx := x.Run
		nx.ID, nx.FamilyID, nx.PlanID, nx.Children, nx.DriverID, nx.Source = 0, f.ID, 0, ids(x.ChildNames), id(x.DriverName), ""
		if err := s.store.CreateRun(ctx, uid, &nx); err != nil {
			return nil, err
		}
		have[k] = true
		added["runs"]++
	}
	chores, _ := s.store.ListChores(ctx, f.ID, true)
	have = map[string]bool{}
	for _, c := range chores {
		have[key(c.Title)] = true
	}
	for _, c := range doc.Chores {
		if have[key(c.Title)] || c.Title == "" {
			continue
		}
		nc := c.Chore
		nc.ID, nc.FamilyID, nc.People, nc.Source = 0, f.ID, ids(c.PeopleNames), ""
		if nc.Turn >= len(nc.People) {
			nc.Turn = 0
		}
		if err := s.store.CreateChore(ctx, uid, &nc); err != nil {
			return nil, err
		}
		have[key(c.Title)] = true
		added["chores"]++
	}
	events, _ := s.store.ListEvents(ctx, f.ID)
	have = map[string]bool{}
	for _, e := range events {
		have[key(e.Title, e.Day)] = true
	}
	for _, e := range doc.Events {
		if have[key(e.Title, e.Day)] || e.Title == "" || e.Day == "" {
			continue
		}
		ne := e.Event
		ne.ID, ne.FamilyID, ne.People, ne.LetterID, ne.Source = 0, f.ID, ids(e.PeopleNames), 0, ""
		if err := s.store.CreateEvent(ctx, uid, &ne); err != nil {
			return nil, err
		}
		have[key(e.Title, e.Day)] = true
		added["events"]++
	}
	lists, _ := s.store.ListLists(ctx, f.ID)
	listID := map[string]int64{}
	for _, l := range lists {
		listID[key(l.Name)] = l.ID
	}
	for _, l := range doc.Lists {
		lid := listID[key(l.Name)]
		if lid == 0 {
			nl := &store.List{FamilyID: f.ID, Name: l.Name, Emoji: l.Emoji}
			if err := s.store.CreateList(ctx, nl); err != nil {
				return nil, err
			}
			lid, listID[key(l.Name)] = nl.ID, nl.ID
			added["lists"]++
		}
		for _, it := range l.Items {
			ni := &store.Item{FamilyID: f.ID, ListID: lid, Name: it.Name, Qty: it.Qty, Note: it.Note, Aisle: it.Aisle}
			isNew, err := s.store.AddItem(ctx, ni)
			if err != nil {
				return nil, err
			}
			if isNew {
				added["items"]++
			}
		}
	}
	terms, _ := s.store.ListTerms(ctx, f.ID)
	have = map[string]bool{}
	for _, t := range terms {
		have[key(t.Name, t.StartsOn)] = true
	}
	for _, t := range doc.Terms {
		if have[key(t.Name, t.StartsOn)] || t.StartsOn == "" {
			continue
		}
		nt := t
		nt.ID, nt.FamilyID = 0, f.ID
		if err := s.store.SaveTerm(ctx, &nt); err != nil {
			return nil, err
		}
		added["terms"]++
	}
	cur, _ := s.store.SchoolDays(ctx, f.ID)
	hasWeek := map[int64]bool{}
	for _, d := range cur {
		hasWeek[d.PersonID] = true
	}
	weeks := map[int64][]store.SchoolDay{}
	for _, d := range doc.School {
		if pid := id(d.PersonName); pid > 0 && !hasWeek[pid] {
			weeks[pid] = append(weeks[pid], d.SchoolDay)
		}
	}
	for pid, days := range weeks {
		if err := s.store.SetSchoolWeek(ctx, f.ID, pid, days); err != nil {
			return nil, err
		}
		added["school_weeks"]++
	}
	letters, _ := s.store.ListLetters(ctx, f.ID, "", 10000)
	have = map[string]bool{}
	for _, l := range letters {
		have[key(l.Title, l.ReceivedOn)] = true
	}
	for _, l := range doc.Letters {
		if have[key(l.Title, l.ReceivedOn)] || l.Title == "" {
			continue
		}
		nl := l.Letter
		nl.ID, nl.FamilyID, nl.People, nl.DoneBy, nl.Source = 0, f.ID, ids(l.PeopleNames), 0, ""
		if err := s.store.CreateLetter(ctx, uid, &nl); err != nil {
			return nil, err
		}
		have[key(l.Title, l.ReceivedOn)] = true
		added["letters"]++
	}
	posts, _ := s.store.ListPosts(ctx, f.ID, f.Me.ID, true, 10000)
	have = map[string]bool{}
	for _, p := range posts {
		have[key(p.Body)] = true
	}
	for _, p := range doc.Posts {
		if have[key(p.Body)] || p.Body == "" {
			continue
		}
		np := &store.Post{FamilyID: f.ID, PersonID: id(p.AuthorName), Body: p.Body, Pinned: p.Pinned, Until: p.Until}
		if err := s.store.CreatePost(ctx, np); err != nil {
			return nil, err
		}
		have[key(p.Body)] = true
		added["posts"]++
	}
	info, _ := s.store.ListInfo(ctx, f.ID)
	have = map[string]bool{}
	for _, in := range info {
		have[key(in.Section, in.Label, fullName(f, in.PersonID))] = true
	}
	for _, in := range doc.Info {
		k := key(in.Section, in.Label, in.PersonName)
		if have[k] || in.Label == "" {
			continue
		}
		ni := in.Info
		ni.ID, ni.FamilyID, ni.PersonID = 0, f.ID, id(in.PersonName)
		if !oneOf(ni.Section, infoSections) {
			ni.Section = "other"
		}
		if err := s.store.SaveInfo(ctx, uid, &ni); err != nil {
			return nil, err
		}
		have[k] = true
		added["info"]++
	}
	return added, nil
}
