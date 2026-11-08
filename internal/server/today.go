package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- Today, the week, a day, the summary and search ----
//
// One household view: who takes which pick-up, the family's plans next to
// each person's own calendar and games, the chores due, what to take to
// school tomorrow, letters waiting, what's for dinner and the weather.

// agendaItem is one line on a day, from here or from another app.
type agendaItem struct {
	Source  string  `json:"source"` // family | an app slug
	Kind    string  `json:"kind"`   // event | birthday | school | term | game | trip | meal | pet | home …
	ID      int64   `json:"id,omitempty"`
	Day     string  `json:"day"`
	EndDay  string  `json:"end_day,omitempty"`
	Time    string  `json:"time,omitempty"`
	EndTime string  `json:"end_time,omitempty"`
	Title   string  `json:"title"`
	Sub     string  `json:"sub,omitempty"`
	Place   string  `json:"place,omitempty"`
	People  []int64 `json:"people,omitempty"`
	Bring   string  `json:"bring,omitempty"`
	URL     string  `json:"url,omitempty"`
	Overdue bool    `json:"overdue,omitempty"`
}

// bringItem is something to take along on a day.
type bringItem struct {
	Day      string `json:"day"`
	PersonID int64  `json:"person_id,omitempty"`
	What     string `json:"what"`
	For      string `json:"for,omitempty"` // "School", the event's title
}

// dayPlan is everything on the family's days from…to (their own things only).
type dayPlan struct {
	Runs    []store.Run
	Events  []store.Event
	Terms   []store.Term
	School  []store.SchoolDay
	Chores  []store.Chore
	Letters []store.Letter
}

func (s *Server) planFor(ctx context.Context, f *store.Family, from, to string) (*dayPlan, error) {
	p := &dayPlan{}
	var err error
	if p.Runs, err = s.store.ListRuns(ctx, f.ID, from, to); err != nil {
		return nil, err
	}
	if p.Events, err = s.store.EventsBetween(ctx, f.ID, from, to); err != nil {
		return nil, err
	}
	if p.Terms, err = s.store.ListTerms(ctx, f.ID); err != nil {
		return nil, err
	}
	if p.School, err = s.store.SchoolDays(ctx, f.ID); err != nil {
		return nil, err
	}
	if p.Chores, err = s.store.ListChores(ctx, f.ID, false); err != nil {
		return nil, err
	}
	if p.Letters, err = s.store.ListLetters(ctx, f.ID, "todo", 100); err != nil {
		return nil, err
	}
	return p, nil
}

// agenda is the family's own lines for the days from…to (not runs, not chores).
func (s *Server) agenda(f *store.Family, p *dayPlan, from, to string) []agendaItem {
	out := []agendaItem{}
	for _, e := range p.Events {
		out = append(out, agendaItem{Source: "family", Kind: e.Kind, ID: e.ID, Day: e.Day, EndDay: e.EndDay, Time: e.StartTime, EndTime: e.EndTime, Title: e.Title,
			Place: e.Place, People: e.People, Bring: e.Bring})
	}
	// birthdays of the family's own people
	for _, pr := range f.People {
		if pr.Birthday == "" || pr.Archived {
			continue
		}
		md := pr.Birthday[len(pr.Birthday)-5:]
		for y := from[:4]; y <= to[:4]; y = fmt.Sprint(atoi(y) + 1) {
			d := y + "-" + md
			if d < from || d > to {
				continue
			}
			sub := ""
			if len(pr.Birthday) == 10 {
				sub = fmt.Sprintf("turns %d", atoi(y)-atoi(pr.Birthday[:4]))
			}
			out = append(out, agendaItem{Source: "family", Kind: "birthday", ID: pr.ID, Day: d, Title: "🎂 " + firstWord(pr.Name) + "'s birthday", Sub: sub, People: []int64{pr.ID}})
		}
	}
	// school: the first and last day of holidays, closed days
	for _, t := range p.Terms {
		if t.Kind == "term" {
			continue
		}
		if t.StartsOn <= to && t.EndsOn >= from {
			day := t.StartsOn
			if day < from {
				day = from
			}
			sub := "no school"
			if t.EndsOn != t.StartsOn {
				sub = "no school until " + dayWords(t.EndsOn)
			}
			out = append(out, agendaItem{Source: "family", Kind: "term", ID: t.ID, Day: day, EndDay: endIf(t.EndsOn, day), Title: "🏖️ " + t.Name, Sub: sub})
		}
		if t.EndsOn >= from && t.EndsOn < to && t.Kind == "holiday" {
			back := store.AddDays(t.EndsOn, 1)
			out = append(out, agendaItem{Source: "family", Kind: "term", ID: t.ID, Day: back, Title: "🎒 Back to school", Sub: t.Name + " is over"})
		}
	}
	return out
}

func endIf(end, day string) string {
	if end > day {
		return end
	}
	return ""
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// bring is what to take along on a day: each child's school things, and what events ask for.
func bringOn(f *store.Family, p *dayPlan, day string) []bringItem {
	out := []bringItem{}
	if !store.NoSchool(day, p.Terms) {
		wd := store.Weekday(day)
		for _, sd := range p.School {
			if sd.Weekday == wd && sd.Bring != "" && personIn(f, sd.PersonID) {
				out = append(out, bringItem{Day: day, PersonID: sd.PersonID, What: sd.Bring, For: "School"})
			}
		}
	}
	for _, e := range p.Events {
		if e.Day == day && e.Bring != "" {
			pid := int64(0)
			if len(e.People) == 1 {
				pid = e.People[0]
			}
			out = append(out, bringItem{Day: day, PersonID: pid, What: e.Bring, For: e.Title})
		}
	}
	return out
}

// choresOn are the chores due on a day (with the overdue ones on today).
func choresOn(list []store.Chore, day string) []store.Chore {
	today := store.Today()
	out := []store.Chore{}
	for _, c := range list {
		if !c.Active || c.NextDue == "" {
			continue
		}
		if c.NextDue == day || (day == today && c.NextDue < today) {
			out = append(out, c)
			continue
		}
		// later days of the week: where a repeating chore will fall next
		if day > today && c.NextDue < day && c.Repeat != "once" {
			d := c.NextDue
			cc := c
			for i := 0; i < 40 && d < day; i++ {
				cc.NextDue = d
				d = store.NextAfter(&cc, d)
				if d == "" {
					break
				}
			}
			if d == day {
				out = append(out, c)
			}
		}
	}
	return out
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	today := firstNonEmpty(r.URL.Query().Get("date"), store.Today())
	if !validDate(today) {
		today = store.Today()
	}
	tomorrow := store.AddDays(today, 1)
	p, err := s.planFor(ctx, f, today, store.AddDays(today, 7))
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	c := s.clients(ctx, actorID(ctx))
	var out outside
	if !queryBool(r, "quick") {
		out = s.gather(ctx, c, today, tomorrow, nil)
	} else {
		out = outside{Items: []outItem{}, Renewals: []outItem{}, Sources: []sourceState{}}
	}
	ag := s.agenda(f, p, today, tomorrow)
	for _, it := range out.Items {
		ag = append(ag, agendaItem{Source: it.App, Kind: it.Kind, Day: it.Day, EndDay: it.EndDay, Time: it.Time, EndTime: it.EndTime, Title: it.Title, Sub: it.Sub, Place: it.Place, URL: it.URL, Overdue: it.Overdue})
	}
	sortAgenda(ag)
	var runs, needs []store.Run
	for _, x := range p.Runs {
		if x.Day <= tomorrow {
			runs = append(runs, x)
		}
		if x.State != store.RunDone && x.State != store.RunSkipped && (x.DriverID == 0 || x.State == store.RunAsking) {
			needs = append(needs, x)
		}
	}
	letters := []store.Letter{}
	for _, l := range p.Letters {
		if l.DueOn == "" || l.DueOn <= store.AddDays(today, 7) {
			letters = append(letters, l)
		}
	}
	lists, _ := s.store.ListLists(ctx, f.ID)
	posts, _ := s.store.ListPosts(ctx, f.ID, f.Me.ID, false, 3)
	renew := []outItem{}
	for _, it := range out.Renewals {
		if it.Day <= store.AddDays(today, 30) {
			renew = append(renew, it)
		}
	}
	since := store.AddDays(today, -1) + "T00:00:00Z"
	activity, _ := s.store.ListActivity(ctx, f.ID, since, 12)
	writeJSON(w, http.StatusOK, map[string]any{
		"date": today, "tomorrow": tomorrow,
		"runs": nonNil(runs), "needs": nonNil(needs),
		"agenda": ag, "chores": choresOn(p.Chores, today),
		"bring_today": bringOn(f, p, today), "bring_tomorrow": bringOn(f, p, tomorrow),
		"no_school": store.NoSchool(today, p.Terms), "no_school_tomorrow": store.NoSchool(tomorrow, p.Terms),
		"letters": letters, "lists": lists, "posts": posts, "unseen": s.store.UnseenPosts(ctx, f.ID, f.Me.ID),
		"weather": out.Weather, "renewals": renew, "sources": out.Sources, "activity": activity,
	})
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func sortAgenda(ag []agendaItem) {
	sort.SliceStable(ag, func(i, j int) bool {
		a, b := ag[i], ag[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if (a.Time == "") != (b.Time == "") {
			return a.Time == ""
		}
		return a.Time < b.Time
	})
}

// weekDay is one day of the week page.
type weekDay struct {
	Day      string        `json:"day"`
	NoSchool bool          `json:"no_school"`
	Runs     []store.Run   `json:"runs"`
	Agenda   []agendaItem  `json:"agenda"`
	Chores   []store.Chore `json:"chores"`
	Bring    []bringItem   `json:"bring"`
}

func (s *Server) week(ctx context.Context, f *store.Family, from string, days int, outsideToo bool) ([]weekDay, []sourceState, error) {
	to := store.AddDays(from, days-1)
	p, err := s.planFor(ctx, f, from, to)
	if err != nil {
		return nil, nil, err
	}
	ag := s.agenda(f, p, from, to)
	sources := []sourceState{}
	if outsideToo {
		out := s.gather(ctx, s.clients(ctx, actorID(ctx)), from, to, map[string]bool{"calendar": true, "league": true, "travel": true, "kitchen": true, "contacts": true, "pets": true, "home": true})
		for _, it := range out.Items {
			ag = append(ag, agendaItem{Source: it.App, Kind: it.Kind, Day: it.Day, EndDay: it.EndDay, Time: it.Time, EndTime: it.EndTime, Title: it.Title, Sub: it.Sub, Place: it.Place, URL: it.URL, Overdue: it.Overdue})
		}
		sources = out.Sources
	}
	sortAgenda(ag)
	out := make([]weekDay, 0, days)
	for i := 0; i < days; i++ {
		d := store.AddDays(from, i)
		wd := weekDay{Day: d, NoSchool: store.NoSchool(d, p.Terms), Runs: []store.Run{}, Agenda: []agendaItem{}, Chores: choresOn(p.Chores, d), Bring: bringOn(f, p, d)}
		for _, x := range p.Runs {
			if x.Day == d {
				wd.Runs = append(wd.Runs, x)
			}
		}
		for _, it := range ag {
			end := firstNonEmpty(it.EndDay, it.Day)
			if it.Day == d || (it.Day < d && end >= d) {
				wd.Agenda = append(wd.Agenda, it)
			}
		}
		out = append(out, wd)
	}
	return out, sources, nil
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	from := r.URL.Query().Get("from")
	if from == "" || !validDate(from) {
		from = store.WeekStart(store.Today(), st.WeekStart)
	}
	days := queryInt(r, "days")
	if days < 1 || days > 31 {
		days = 7
	}
	list, sources, err := s.week(ctx, f, from, days, !queryBool(r, "quick"))
	if err != nil {
		s.fail(w, err, "week")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "days": list, "sources": sources})
}

// handleDay is one day for the rest of the box (Pocket Day, Pocket AI): what
// happened and what was planned, the family's own things only.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	day := chiParam(r, "date")
	if !validDate(day) || day == "" {
		writeErr(w, http.StatusBadRequest, "a date is YYYY-MM-DD")
		return
	}
	list, _, err := s.week(ctx, f, day, 1, false)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	d := list[0]
	done, _ := s.store.ChoreLog(ctx, f.ID, day, day)
	acts, _ := s.store.ListActivity(ctx, f.ID, day+"T00:00:00Z", 200)
	var dayActs []store.Activity
	for _, a := range acts {
		if strings.HasPrefix(a.CreatedAt, day) {
			dayActs = append(dayActs, a)
		}
	}
	people := map[int64]string{}
	for _, p := range f.People {
		people[p.ID] = p.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "family": f.Name, "people": people, "no_school": d.NoSchool, "runs": d.Runs, "agenda": d.Agenda,
		"chores_due": d.Chores, "chores_done": done, "bring": d.Bring, "activity": nonNil(dayActs), "url": strings.TrimRight(s.cfg.BaseURL, "/") + "/week?day=" + day})
}

// handleSummary is the family at a glance (dashboards, Pocket AI's briefing).
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	today := store.Today()
	p, err := s.planFor(ctx, f, today, store.AddDays(today, 7))
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	var mineToday, needs, todayRuns []store.Run
	for _, x := range p.Runs {
		open := x.State != store.RunDone && x.State != store.RunSkipped
		if x.Day == today && x.State != store.RunSkipped {
			todayRuns = append(todayRuns, x)
		}
		if x.Day == today && x.DriverID == f.Me.ID && open {
			mineToday = append(mineToday, x)
		}
		if open && (x.DriverID == 0 || x.State == store.RunAsking) {
			needs = append(needs, x)
		}
	}
	items, _ := s.store.OpenItems(ctx, f.ID)
	var next *agendaItem
	for _, a := range s.agenda(f, p, today, store.AddDays(today, 7)) {
		if a.Kind != "term" {
			a := a
			if next == nil || a.Day < next.Day || (a.Day == next.Day && a.Time < next.Time) {
				next = &a
			}
		}
	}
	chores := choresOn(p.Chores, today)
	var mine int
	for _, c := range chores {
		if c.For(f.Me.ID) {
			mine++
		}
	}
	words := func(list []store.Run) []map[string]any {
		out := []map[string]any{}
		for _, x := range list {
			out = append(out, map[string]any{"id": x.ID, "day": x.Day, "time": x.Time, "title": runWords(f, &x), "driver": nameOf(f, x.DriverID), "state": x.State})
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"family": f.Name, "me": f.Me.Name, "date": today,
		"runs_today": words(todayRuns), "my_runs_today": words(mineToday), "needs_someone": words(needs),
		"chores_due_today": len(chores), "my_chores_today": mine, "shopping_open": len(items),
		"letters_to_do": len(p.Letters), "unseen_posts": s.store.UnseenPosts(ctx, f.ID, f.Me.ID),
		"bring_tomorrow": bringOn(f, p, store.AddDays(today, 1)), "next": next, "no_school_today": store.NoSchool(today, p.Terms),
		"url": strings.TrimRight(s.cfg.BaseURL, "/") + "/",
	})
}

// handleSearch finds things in the family by words.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	q := store.Fold(strings.TrimSpace(r.URL.Query().Get("q")))
	type hit struct {
		Type     string `json:"type"`
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		URL      string `json:"url"`
	}
	out := []hit{}
	if q == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	has := func(parts ...string) bool { return strings.Contains(store.Fold(strings.Join(parts, " ")), q) }
	for _, p := range f.People {
		if has(p.Name, p.School, p.ClassName, p.Notes) {
			out = append(out, hit{"person", p.ID, p.Name, roleWords(p.Role), "/family"})
		}
	}
	if list, err := s.store.ListEvents(ctx, f.ID); err == nil {
		for _, e := range list {
			if has(e.Title, e.Place, e.Notes, e.Bring) {
				out = append(out, hit{"event", e.ID, e.Title, dayWords(e.Day), "/week?day=" + e.Day})
			}
		}
	}
	if list, err := s.store.ListLetters(ctx, f.ID, "", 300); err == nil {
		for _, l := range list {
			if has(l.Title, l.Sender, l.Body, l.Action, l.Summary) {
				out = append(out, hit{"letter", l.ID, l.Title, l.Sender, "/school/letters/" + itoa(l.ID)})
			}
		}
	}
	if list, err := s.store.ListChores(ctx, f.ID, true); err == nil {
		for _, c := range list {
			if has(c.Title, c.Notes) {
				out = append(out, hit{"chore", c.ID, strings.TrimSpace(c.Emoji + " " + c.Title), "chore", "/chores"})
			}
		}
	}
	if list, err := s.store.OpenItems(ctx, f.ID); err == nil {
		for _, it := range list {
			if has(it.Name, it.Note) {
				out = append(out, hit{"item", it.ID, it.Name, "on the shopping list", "/shopping/" + itoa(it.ListID)})
			}
		}
	}
	if list, err := s.store.ListInfo(ctx, f.ID); err == nil {
		for _, in := range list {
			if has(in.Label, in.Value) {
				out = append(out, hit{"info", in.ID, in.Label, clip(in.Value, 80), "/family#info"})
			}
		}
	}
	if list, err := s.store.ListPosts(ctx, f.ID, f.Me.ID, true, 200); err == nil {
		for _, p := range list {
			if has(p.Body) {
				out = append(out, hit{"post", p.ID, clip(firstLine(p.Body), 80), "on the board", "/board"})
			}
		}
	}
	if len(out) > 60 {
		out = out[:60]
	}
	writeJSON(w, http.StatusOK, out)
}

func roleWords(r string) string {
	return map[string]string{store.RoleGrownup: "grown-up", store.RoleChild: "child", store.RoleHelper: "helps out"}[r]
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// weekText is the coming week in plain words (for a note).
func (s *Server) weekText(ctx context.Context, f *store.Family) string {
	list, _, err := s.week(ctx, f, store.Today(), 7, false)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, d := range list {
		t, _ := store.ParseDate(d.Day)
		fmt.Fprintf(&b, "## %s\n", t.Format("Monday 2 January"))
		if d.NoSchool {
			b.WriteString("- No school\n")
		}
		for _, x := range d.Runs {
			if x.State == store.RunSkipped {
				continue
			}
			fmt.Fprintf(&b, "- %s %s — %s\n", x.Time, runWords(f, &x), firstNonEmpty(nameOf(f, x.DriverID), "**nobody yet**"))
		}
		for _, a := range d.Agenda {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(a.Time+" "+a.Title+placeWords(a.Place)))
		}
		for _, br := range d.Bring {
			fmt.Fprintf(&b, "- Bring (%s): %s\n", firstNonEmpty(nameOf(f, br.PersonID), br.For), br.What)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func placeWords(p string) string {
	if p == "" {
		return ""
	}
	return " (" + p + ")"
}

// sheetText is the sitter sheet in markdown.
func (s *Server) sheetText(ctx context.Context, f *store.Family) string {
	info, _ := s.store.ListInfo(ctx, f.ID)
	var b strings.Builder
	b.WriteString("## People\n")
	for _, p := range f.People {
		if p.Archived {
			continue
		}
		fmt.Fprintf(&b, "- **%s** (%s)%s%s\n", p.Name, roleWords(p.Role), prefixed(" · ", p.Phone), prefixed(" · ", p.Notes))
	}
	sec := ""
	for _, in := range info {
		if !in.OnSheet {
			continue
		}
		if in.Section != sec {
			sec = in.Section
			fmt.Fprintf(&b, "\n## %s\n", sectionWords(sec))
		}
		who := ""
		if in.PersonID > 0 {
			who = " (" + nameOf(f, in.PersonID) + ")"
		}
		fmt.Fprintf(&b, "- **%s**%s: %s%s\n", in.Label, who, in.Value, prefixed(" · ", in.Phone))
	}
	return b.String()
}

func prefixed(p, v string) string {
	if v == "" {
		return ""
	}
	return p + v
}

func sectionWords(s string) string {
	return map[string]string{"emergency": "In an emergency", "health": "Health and allergies", "school": "School", "home": "At home", "sitter": "For the sitter", "other": "Good to know"}[s]
}
