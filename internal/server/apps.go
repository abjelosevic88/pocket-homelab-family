package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- the family in the other Pocket apps ----
//
// Reading (for Today and the week, as the person looking, with their own
// links): their calendar, sport games, trips, the meal plan, the weather,
// the pets' and the home's care due, papers running out, relatives'
// birthdays. Every source is optional: when an app is not there, switched
// off or slow, the page says so in a line and shows the rest.
//
// Making (a button on a run, an event, a letter, a person): a task, a
// calendar event, a note, a payment in Pocket Money, a shared cost in Pocket
// Split, a contact, a message in the family chat. Each thing made is
// remembered (privately) with a link back.

// outItem is one thing from another app on the family's days.
type outItem struct {
	App     string `json:"app"`
	Kind    string `json:"kind"` // event | game | trip | meal | pet | home | birthday | renewal
	Day     string `json:"day"`
	EndDay  string `json:"end_day,omitempty"`
	Time    string `json:"time,omitempty"`
	EndTime string `json:"end_time,omitempty"`
	Title   string `json:"title"`
	Sub     string `json:"sub,omitempty"`
	Place   string `json:"place,omitempty"`
	URL     string `json:"url,omitempty"`
	Overdue bool   `json:"overdue,omitempty"`
	Ref     string `json:"ref,omitempty"`
}

// sourceState says how one app did for this page.
type sourceState struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	State  string `json:"state"` // on | off | error
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
	Count  int    `json:"count"`
}

type outside struct {
	Items    []outItem      `json:"items"`
	Weather  map[string]any `json:"weather,omitempty"`
	Renewals []outItem      `json:"renewals"`
	Sources  []sourceState  `json:"sources"`
}

func link(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

type gatherJob struct {
	a   pocketApp
	run func(ctx context.Context, sib *siblingConn) ([]outItem, error)
}

// gather reads the other apps for the days from…to, all at once, each within a few seconds.
func (s *Server) gather(ctx context.Context, c *clients, from, to string, want map[string]bool) outside {
	out := outside{Items: []outItem{}, Renewals: []outItem{}, Sources: []sourceState{}}
	days := daysBetween(from, to) + 1
	var mu sync.Mutex
	jobs := []gatherJob{
		{appCalendar, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/events", url.Values{"from": {from}, "to": {store.AddDays(to, 1)}, "tz": {time.Local.String()}})
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v, "events") {
				allDay, _ := m["all_day"].(bool)
				start, end := str(m, "start"), str(m, "end")
				it := outItem{App: "calendar", Kind: "event", Day: day10(start), Title: firstNonEmpty(str(m, "title"), "Busy"), Place: str(m, "location"),
					URL: link(sib, "/event/"+url.PathEscape(idOf(m))), Ref: idOf(m)}
				if str(m, "source") == "family" {
					continue // put there from here: already on the page
				}
				if allDay {
					if e := day10(end); e != "" && store.AddDays(e, -1) > it.Day {
						it.EndDay = store.AddDays(e, -1)
					}
				} else {
					it.Time, it.EndTime = clock(start), clock(end)
				}
				items = append(items, it)
			}
			return items, nil
		}},
		{appLeague, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/games", url.Values{"from": {from}, "to": {store.AddDays(to, 1)}, "limit": {"60"}})
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v, "games") {
				if str(m, "status") == "cancelled" {
					continue
				}
				start := str(m, "starts_at")
				title := firstNonEmpty(str(m, "title"), joinNonEmpty(" · ", str(m, "sport"), str(m, "group_name")))
				end := ""
				if d := int(num(m, "duration_min")); d > 0 && len(start) >= 16 {
					if t, err := time.Parse("2006-01-02T15:04", start[:16]); err == nil {
						end = t.Add(time.Duration(d) * time.Minute).Format("15:04")
					}
				}
				sub := str(m, "group_name")
				if a := str(m, "my_answer"); a != "" {
					sub = joinNonEmpty(" · ", sub, map[string]string{"in": "you're in", "out": "you're out", "maybe": "maybe"}[a])
				}
				items = append(items, outItem{App: "league", Kind: "game", Day: day10(start), Time: clock(start), EndTime: end, Title: strings.TrimSpace(str(m, "emoji") + " " + title),
					Sub: sub, Place: str(m, "venue_name"), URL: link(sib, "/games/"+idOf(m)), Ref: idOf(m)})
			}
			return items, nil
		}},
		{appTravel, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/trips", nil)
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v, "trips") {
				a, b := str(m, "start_date"), firstNonEmpty(str(m, "end_date"), str(m, "start_date"))
				if a == "" || a > to || b < from || m["archived"] == true {
					continue
				}
				it := outItem{App: "travel", Kind: "trip", Day: a, Title: "✈️ " + str(m, "name"), Sub: str(m, "destination"), URL: link(sib, "/trips/"+idOf(m)), Ref: idOf(m)}
				if b != a {
					it.EndDay = b
				}
				items = append(items, it)
			}
			return items, nil
		}},
		{appKitchen, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/plan", url.Values{"from": {from}, "to": {to}})
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v, "entries") {
				slot := str(m, "slot")
				if slot != "dinner" && slot != "lunch" {
					continue
				}
				title := firstNonEmpty(str(m, "title"), str(m, "recipe_title"), "Something")
				u := link(sib, "/plan")
				if rid := str(m, "recipe_id"); rid != "" {
					u = link(sib, "/recipes/"+rid)
				}
				items = append(items, outItem{App: "kitchen", Kind: "meal", Day: str(m, "date"), Title: title, Sub: slot, URL: u, Ref: str(m, "recipe_id")})
			}
			return items, nil
		}},
		{appPets, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			return careDue(ctx, sib, "pets", from, to, days, func(m map[string]any) (string, string) {
				return joinNonEmpty(": ", str(m, "pet_name"), str(m, "title")), "/care/" + idOf(m)
			})
		}},
		{appHome, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			return careDue(ctx, sib, "home", from, to, days, func(m map[string]any) (string, string) {
				if p := str(m, "plant_name"); p != "" {
					return str(m, "title") + " — " + p, "/plants/" + str(m, "plant_id")
				}
				return joinNonEmpty(" — ", str(m, "title"), str(m, "thing_name")), "/upkeep/" + idOf(m)
			})
		}},
		{appContacts, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/birthdays", url.Values{"from": {from}, "days": {strconv.Itoa(days)}})
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v) {
				ct := obj(m, "contact")
				name := firstNonEmpty(str(ct, "fn"), joinNonEmpty(" ", str(ct, "given"), str(ct, "family")))
				d := str(m, "date")
				if d < from || d > to || name == "" {
					continue
				}
				title := "🎂 " + name
				if str(m, "kind") == "anniversary" {
					title = "💍 " + name + " (anniversary)"
				}
				sub := ""
				if age := num(m, "age"); age > 0 {
					sub = "turns " + strconv.Itoa(int(age))
				}
				items = append(items, outItem{App: "contacts", Kind: "birthday", Day: d, Title: title, Sub: sub, URL: link(sib, "/contacts/"+idOf(ct)), Ref: idOf(ct)})
			}
			return items, nil
		}},
		{appRenewals, func(ctx context.Context, sib *siblingConn) ([]outItem, error) {
			v, err := getJSONFrom(ctx, sib, "/api/v1/due", url.Values{"days": {"60"}})
			if err != nil {
				return nil, err
			}
			var items []outItem
			for _, m := range rows(v, "renewals") {
				g := str(m, "group")
				if g != "identity" && g != "health" && g != "insurance" {
					continue
				}
				who := firstNonEmpty(str(m, "contact_name"), str(m, "holder"))
				items = append(items, outItem{App: "renewals", Kind: "renewal", Day: str(m, "expires_on"), Title: joinNonEmpty(" — ", str(m, "title"), who),
					Sub: str(m, "status"), Overdue: str(m, "status") == "expired", URL: link(sib, "/r/"+idOf(m)), Ref: idOf(m)})
			}
			return items, nil
		}},
	}
	var wg sync.WaitGroup
	add := func(st sourceState, items []outItem, renew bool) {
		mu.Lock()
		defer mu.Unlock()
		st.Count = len(items)
		out.Sources = append(out.Sources, st)
		if renew {
			out.Renewals = append(out.Renewals, items...)
		} else {
			out.Items = append(out.Items, items...)
		}
	}
	for _, j := range jobs {
		if want != nil && !want[j.a.slug] {
			continue
		}
		if j.a.slug == "calendar" && !c.st.ShowCalendar {
			continue
		}
		sib, st := s.source(ctx, c, j.a)
		if sib == nil {
			add(st, nil, false)
			continue
		}
		wg.Add(1)
		go func(j gatherJob, sib *siblingConn, st sourceState) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			items, err := j.run(cctx, sib)
			if err != nil {
				st.State, st.Detail = "error", plainErr(j.a, err)
				items = nil
			}
			add(st, items, j.a.slug == "renewals")
		}(j, sib, st)
	}
	if want == nil || want["weather"] {
		if sib, st := s.source(ctx, c, appWeather); sib != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
				defer cancel()
				v, err := getJSONFrom(cctx, sib, "/api/v1/summary", nil)
				m, _ := v.(map[string]any)
				if err != nil {
					st.State, st.Detail = "error", plainErr(appWeather, err)
				} else if m != nil && m["place"] != nil {
					w := map[string]any{"place": str(obj(m, "place"), "name"), "text": str(m, "text"), "today": obj(m, "today"), "tomorrow": obj(m, "tomorrow"), "url": link(sib, "/")}
					if ws := rows(m, "warnings"); len(ws) > 0 {
						w["warnings"] = ws
					}
					mu.Lock()
					out.Weather = w
					mu.Unlock()
					st.Count = 1
				}
				mu.Lock()
				out.Sources = append(out.Sources, st)
				mu.Unlock()
			}()
		} else {
			add(st, nil, false)
		}
	}
	wg.Wait()
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.Time < b.Time
	})
	sort.SliceStable(out.Sources, func(i, j int) bool { return out.Sources[i].Name < out.Sources[j].Name })
	return out
}

// careDue reads a /due list (Pocket Pets, Pocket Home): overdue (shown on the
// first day of the range when that is today or earlier) and due in the range.
func careDue(ctx context.Context, sib *siblingConn, app, from, to string, days int, words func(map[string]any) (string, string)) ([]outItem, error) {
	v, err := getJSONFrom(ctx, sib, "/api/v1/due", url.Values{"days": {strconv.Itoa(days + 1)}})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	if m == nil {
		return nil, errors.New("an answer Family does not understand")
	}
	today := store.Today()
	kind := "pet"
	if app == "home" {
		kind = "home"
	}
	var items []outItem
	seen := map[string]bool{}
	for _, key := range []string{"overdue", "due", "soon", "upcoming"} {
		for _, x := range rows(m, key) {
			id := idOf(x)
			d := str(x, "due_on")
			if seen[id] || d == "" {
				continue
			}
			overdue := d < today
			day := d
			if overdue {
				if from > today {
					continue
				}
				day = from
			}
			if day < from || day > to {
				continue
			}
			seen[id] = true
			title, path := words(x)
			sub := ""
			if overdue {
				sub = "was due " + dayWords(d)
			}
			items = append(items, outItem{App: app, Kind: kind, Day: day, Title: title, Sub: sub, Overdue: overdue, URL: link(sib, path), Ref: id})
		}
	}
	return items, nil
}

// source is a connection to an app for reading, or nil with why not.
func (s *Server) source(ctx context.Context, c *clients, a pocketApp) (*siblingConn, sourceState) {
	st := sourceState{Slug: a.slug, Name: a.name, State: "on"}
	as := c.status(ctx, a)
	if as.Via == "" || len(as.On) == 0 {
		st.State, st.Detail, st.Fix = "off", as.Detail, as.Fix
		if as.State == "missing" {
			st.Detail = "not on this box"
		}
		return nil, st
	}
	sib, err := c.conn(ctx, a, readScopes(a)...)
	if err != nil {
		st.State, st.Detail = "off", err.Error()
		return nil, st
	}
	return sib, st
}

// readScopes are the scopes of an app used for reading.
func readScopes(a pocketApp) []string {
	var out []string
	for _, sc := range a.scopes {
		if strings.HasSuffix(sc, ":read") {
			out = append(out, sc)
		}
	}
	if len(out) == 0 {
		return a.scopes
	}
	return out
}

func plainErr(a pocketApp, err error) string {
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "Client.Timeout") {
		return a.name + " took too long to answer"
	}
	return a.name + ": " + clip(msg, 200)
}

// clock is the HH:MM of an RFC 3339 time or "YYYY-MM-DDTHH:MM".
func clock(v string) string {
	if len(v) >= 16 && v[10] == 'T' {
		return v[11:16]
	}
	return ""
}

func daysBetween(a, b string) int {
	ta, err1 := store.ParseDate(a)
	tb, err2 := store.ParseDate(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

// ---- making things in the other apps ----

type makeIn struct {
	Owner   string `json:"owner"` // run | event | letter | chore | person | sheet | week
	OwnerID int64  `json:"owner_id"`
	Target  string `json:"target"` // task | event | note | payment | expense | contact | chat
	Title   string `json:"title"`
	Notes   string `json:"notes"`
	Due     string `json:"due"`
	// payment / expense
	AccountID int64  `json:"account_id"`
	GroupID   int64  `json:"group_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	// chat: "me", an email, a conversation id or a group name
	To string `json:"to"`
}

var makeApps = map[string]pocketApp{"task": appTasks, "event": appCalendar, "note": appNotes, "payment": appMoney, "expense": appSplit, "contact": appContacts, "chat": appChat}

// subject is what is being made from, in words.
type subject struct {
	title, text, day, time, endTime, place, link string
	allDay                                       bool
	amount                                       int64
	currency                                     string
}

func (s *Server) subjectOf(ctx context.Context, f *store.Family, owner string, id int64) (*subject, error) {
	base := strings.TrimRight(s.cfg.BaseURL, "/")
	sub := &subject{}
	switch owner {
	case "run":
		x, err := s.store.GetRun(ctx, f.ID, id)
		if err != nil {
			return nil, err
		}
		sub.title, sub.day, sub.time, sub.place = runWords(f, x), x.Day, x.Time, x.Place
		sub.text = joinNonEmpty("\n", "Who: "+firstNonEmpty(nameOf(f, x.DriverID), "nobody yet"), x.Notes)
		sub.link = base + "/pickups?day=" + x.Day
	case "event":
		e, err := s.store.GetEvent(ctx, f.ID, id)
		if err != nil {
			return nil, err
		}
		sub.title, sub.day, sub.time, sub.endTime, sub.place = e.Title, e.Day, e.StartTime, e.EndTime, e.Place
		sub.allDay = e.StartTime == ""
		sub.text = joinNonEmpty("\n", whoLine(f, e.People), bringLine(e.Bring), e.Notes)
		sub.link = base + "/week?day=" + e.Day
	case "letter":
		l, err := s.store.GetLetter(ctx, f.ID, id)
		if err != nil {
			return nil, err
		}
		sub.title, sub.day, sub.allDay = l.Title, firstNonEmpty(l.DueOn, store.Today()), true
		sub.text = joinNonEmpty("\n\n", l.Action, l.Summary, l.Body)
		sub.amount, sub.currency = l.Amount, l.Currency
		sub.link = base + "/school/letters/" + itoa(l.ID)
	case "chore":
		c, err := s.store.GetChore(ctx, f.ID, id)
		if err != nil {
			return nil, err
		}
		sub.title, sub.day, sub.text = strings.TrimSpace(c.Emoji+" "+c.Title), c.NextDue, c.Notes
		sub.link = base + "/chores"
	case "person":
		p := personByID(f, id)
		if p == nil {
			return nil, store.ErrNotFound
		}
		sub.title, sub.text = p.Name, p.Notes
		sub.link = base + "/family"
	case "sheet":
		sub.title, sub.text, sub.link = f.Name+" — for the sitter", s.sheetText(ctx, f), base+"/family/sheet"
	case "week":
		sub.title, sub.link = f.Name+" — the week from "+dayWords(store.Today()), base+"/week"
	default:
		return nil, &userErr{"say what it is made from"}
	}
	return sub, nil
}

func whoLine(f *store.Family, ids []int64) string {
	if n := namesOf(f, ids); n != "" {
		return "Who: " + n
	}
	return ""
}

func bringLine(b string) string {
	if b == "" {
		return ""
	}
	return "Bring: " + b
}

type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	if linkApp(ctx) != "" {
		writeErr(w, http.StatusForbidden, "only from Pocket Family itself")
		return
	}
	var in makeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	m, err := s.make(ctx, f, in)
	if err != nil {
		var ue *userErr
		switch {
		case errors.As(err, &ue):
			writeErr(w, http.StatusBadRequest, ue.msg)
		case isNotConnected(err):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, store.ErrNotFound):
			writeErr(w, http.StatusNotFound, "not found")
		default:
			writeErr(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) make(ctx context.Context, f *store.Family, in makeIn) (*store.Made, error) {
	a, ok := makeApps[in.Target]
	if !ok {
		return nil, &userErr{"make a task, an event, a note, a payment, a shared cost, a contact or a chat message"}
	}
	sub, err := s.subjectOf(ctx, f, in.Owner, in.OwnerID)
	if err != nil {
		return nil, err
	}
	c := s.clients(ctx, actorID(ctx))
	title := firstNonEmpty(strings.TrimSpace(in.Title), sub.title)
	origin := "From Pocket Family — " + sub.link
	m := &store.Made{FamilyID: f.ID, Owner: in.Owner, OwnerID: in.OwnerID, Target: in.Target, App: a.slug, Title: title}
	var res map[string]any
	switch in.Target {
	case "task":
		sib, err := c.conn(ctx, a, "tasks:write")
		if err != nil {
			return nil, err
		}
		body := map[string]any{"title": title, "notes": joinNonEmpty("\n\n", in.Notes, sub.text, origin)}
		if d := firstNonEmpty(in.Due, sub.day); d != "" && validDate(d) {
			body["due"] = d
		}
		if res, err = postJSONTo(ctx, sib, "/api/v1/tasks", body); err != nil {
			return nil, err
		}
		m.URL = link(sib, "/today?task="+idOf(res))
	case "event":
		sib, err := c.conn(ctx, a, "calendar:write")
		if err != nil {
			return nil, err
		}
		if sub.day == "" {
			return nil, &userErr{"this has no day to put in a calendar"}
		}
		body := map[string]any{"title": title, "description": joinNonEmpty("\n\n", sub.text, origin), "location": sub.place, "tz": time.Local.String()}
		if sub.allDay || sub.time == "" {
			body["all_day"], body["start"] = true, sub.day
		} else {
			body["start"] = sub.day + "T" + sub.time
			if sub.endTime != "" && sub.endTime > sub.time {
				body["end"] = sub.day + "T" + sub.endTime
			} else if in.Owner == "run" {
				body["end"] = sub.day + "T" + addMinutes(sub.time, 20)
			}
		}
		if res, err = postJSONTo(ctx, sib, "/api/v1/events", body); err != nil {
			return nil, err
		}
		m.URL = link(sib, "/event/"+url.PathEscape(idOf(res)))
	case "note":
		sib, err := c.conn(ctx, a, "notes:write")
		if err != nil {
			return nil, err
		}
		content := sub.text
		if in.Owner == "week" {
			content = s.weekText(ctx, f)
		}
		body := map[string]any{"title": title, "content": joinNonEmpty("\n\n", in.Notes, content, "_"+origin+"_"), "category": "Family"}
		if res, err = postJSONTo(ctx, sib, "/api/v1/notes", body); err != nil {
			return nil, err
		}
		m.URL = link(sib, "/note/"+idOf(res))
	case "payment":
		sib, err := c.conn(ctx, a, "transactions:write")
		if err != nil {
			return nil, err
		}
		amount := firstPositive(in.Amount, sub.amount)
		if amount <= 0 {
			return nil, &userErr{"say how much"}
		}
		if in.AccountID == 0 {
			return nil, &userErr{"pick the account it was paid from"}
		}
		body := map[string]any{"account_id": in.AccountID, "amount": amount, "kind": "expense", "date": store.Today(), "description": title,
			"notes": joinNonEmpty("\n", in.Notes, origin), "external_id": fmt.Sprintf("family-%s-%d", in.Owner, in.OwnerID), "tags": []string{"family"}}
		if cur := cleanCurrency(firstNonEmpty(in.Currency, sub.currency)); cur != "" {
			body["currency"] = cur
		}
		if res, err = postJSONTo(ctx, sib, "/api/v1/transactions", body); err != nil {
			return nil, err
		}
		m.URL = link(sib, "/transactions/"+idOf(res))
	case "expense":
		sib, err := c.conn(ctx, a, "groups:read", "expenses:write")
		if err != nil {
			return nil, err
		}
		amount := firstPositive(in.Amount, sub.amount)
		if amount <= 0 || in.GroupID == 0 {
			return nil, &userErr{"say how much, and pick the Split group"}
		}
		gv, err := getJSONFrom(ctx, sib, fmt.Sprintf("/api/v1/groups/%d", in.GroupID), nil)
		if err != nil {
			return nil, err
		}
		g, _ := gv.(map[string]any)
		me := int64(num(g, "my_member_id"))
		var shares []map[string]any
		for _, mem := range rows(g, "members") {
			shares = append(shares, map[string]any{"member_id": int64(num(mem, "id")), "share": 1})
		}
		if me == 0 || len(shares) == 0 {
			return nil, &userErr{"you are not in that Split group"}
		}
		body := map[string]any{"title": title, "amount": amount, "paid_by": me, "date": store.Today(), "split_mode": "even", "shares": shares, "notes": joinNonEmpty("\n", in.Notes, origin)}
		if cur := cleanCurrency(firstNonEmpty(in.Currency, sub.currency)); cur != "" {
			body["currency"] = cur
		}
		if res, err = postJSONTo(ctx, sib, fmt.Sprintf("/api/v1/groups/%d/expenses", in.GroupID), body); err != nil {
			return nil, err
		}
		m.URL = link(sib, fmt.Sprintf("/groups/%d/expenses/%s", in.GroupID, idOf(res)))
	case "contact":
		if in.Owner != "person" {
			return nil, &userErr{"contacts are made for people in the family"}
		}
		p := personByID(f, in.OwnerID)
		sib, err := c.conn(ctx, a, "contacts:write")
		if err != nil {
			return nil, err
		}
		card := map[string]any{"fn": p.Name, "note": joinNonEmpty("\n", p.Notes, origin), "categories": []string{"Family"}}
		if p.Phone != "" {
			card["phones"] = []map[string]string{{"label": "mobile", "value": p.Phone}}
		}
		if p.Email != "" {
			card["emails"] = []map[string]string{{"label": "home", "value": p.Email}}
		}
		if p.Birthday != "" {
			card["birthday"] = p.Birthday
		}
		if res, err = postJSONTo(ctx, sib, "/api/v1/contacts", map[string]any{"card": card}); err != nil {
			return nil, err
		}
		m.URL = link(sib, "/contacts/"+idOf(res))
		p.ContactRef = idOf(res)
		_ = s.store.UpdatePerson(ctx, p)
	case "chat":
		text := firstNonEmpty(strings.TrimSpace(in.Notes), title+" — "+sub.link)
		msg, err := s.toChat(ctx, f, firstNonEmpty(in.To, "me"), text, in.Owner, in.OwnerID, nil)
		if err != nil {
			return nil, err
		}
		res = msg
		m.Title = "Sent to Pocket Chat"
		if cid := str(msg, "conversation_id"); cid != "" {
			if sib, _ := c.conn(ctx, a, "chat:write"); sib != nil {
				m.URL = link(sib, "/c/"+cid)
			}
		}
	}
	m.Ref = idOf(res)
	if err := s.store.AddMade(ctx, actorID(ctx), m); err != nil {
		return nil, err
	}
	return m, nil
}

func firstPositive(v ...int64) int64 {
	for _, x := range v {
		if x > 0 {
			return x
		}
	}
	return 0
}

func addMinutes(hhmm string, n int) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return hhmm
	}
	return t.Add(time.Duration(n) * time.Minute).Format("15:04")
}

// toChat posts a message into the person's Pocket Chat (their own link).
func (s *Server) toChat(ctx context.Context, f *store.Family, to, text, owner string, ownerID int64, card map[string]any) (map[string]any, error) {
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appChat, "chat:write")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"to": to, "text": clip(text, 3000), "client_id": fmt.Sprintf("family-%s-%d-%d", owner, ownerID, time.Now().Unix()/60)}
	if card != nil {
		body["card"] = card
	}
	return postJSONTo(ctx, sib, "/api/v1/send", body)
}

// handleMakeOptions says which kinds of things can be made right now (and why not).
func (s *Server) handleMakeOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := map[string]any{}
	for target, a := range makeApps {
		as := c.status(ctx, a)
		out[target] = map[string]any{"app": a.slug, "name": a.name, "state": as.State, "on": as.Via != "" && len(as.On) > 0, "detail": as.Detail, "fix": as.Fix}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMakeAccounts lists the person's Pocket Money accounts (for paying a letter).
func (s *Server) handleMakeAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appMoney, "accounts:read")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/accounts", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "accounts") {
		if m["archived"] == true || str(m, "role") == "viewer" {
			continue
		}
		out = append(out, map[string]any{"id": int64(num(m, "id")), "name": str(m, "name"), "currency": str(m, "currency"), "everyday": m["everyday"] == true})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMakeGroups lists the person's Pocket Split groups (for sharing a cost).
func (s *Server) handleMakeGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appSplit, "groups:read")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/groups", url.Values{"archived": {"0"}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "groups") {
		var names []string
		for _, mem := range rows(m, "members") {
			names = append(names, str(mem, "name"))
		}
		out = append(out, map[string]any{"id": int64(num(m, "id")), "name": str(m, "name"), "currency": str(m, "currency"), "members": strings.Join(names, ", ")})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleChats lists the person's Pocket Chat conversations (to tell one).
func (s *Server) handleChats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appChat, "chat:read")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/conversations", nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "conversations") {
		if m["archived"] == true {
			continue
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "name"), str(m, "title")), "kind": str(m, "kind"), "emoji": str(m, "emoji")})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePickContacts searches the person's Pocket Contacts (for the family info).
func (s *Server) handlePickContacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appContacts, "contacts:read")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var v any
	if q == "" {
		v, err = getJSONFrom(ctx, sib, "/api/v1/contacts", url.Values{"limit": {"40"}, "recent": {"1"}})
	} else {
		v, err = getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"30"}})
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []map[string]any{}
	for _, m := range rows(v, "contacts") {
		phones, _ := m["phones"].([]any)
		phone := ""
		if len(phones) > 0 {
			phone, _ = phones[0].(string)
		}
		out = append(out, map[string]any{"id": idOf(m), "name": firstNonEmpty(str(m, "fn"), joinNonEmpty(" ", str(m, "given"), str(m, "family")), str(m, "org")),
			"phone": phone, "org": str(m, "org"), "title": str(m, "title"), "birthday": str(m, "birthday"), "url": link(sib, "/contacts/"+idOf(m))})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleFromPlan puts the ingredients of the meals planned in Pocket Kitchen on a list.
func (s *Server) handleFromPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetList(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	_ = decode(r, &in)
	if in.From == "" || !validDate(in.From) {
		in.From = store.Today()
	}
	if in.To == "" || !validDate(in.To) || in.To < in.From {
		in.To = store.AddDays(in.From, 6)
	}
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, appKitchen, "plan:read", "recipes:read")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/plan", url.Values{"from": {in.From}, "to": {in.To}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	seen := map[string]bool{}
	var names, meals []string
	for _, e := range rows(v, "entries") {
		rid := str(e, "recipe_id")
		if rid == "" || seen[rid] || str(e, "cooked_at") != "" {
			continue
		}
		seen[rid] = true
		rv, err := getJSONFrom(ctx, sib, "/api/v1/recipes/"+rid, nil)
		if err != nil {
			continue
		}
		rm, _ := rv.(map[string]any)
		rec := obj(rm, "recipe")
		meals = append(meals, str(rec, "title"))
		for _, ing := range rows(map[string]any{"items": rec["ingredients"]}) {
			if ing["optional"] == true || str(ing, "name") == "" {
				continue
			}
			q := ""
			if n := num(ing, "qty"); n > 0 {
				q = strings.TrimSpace(strconv.FormatFloat(n, 'f', -1, 64) + " " + str(ing, "unit"))
			}
			if q != "" {
				names = append(names, str(ing, "name")+", "+q)
			} else {
				names = append(names, str(ing, "name"))
			}
		}
	}
	if len(names) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []store.Item{}, "added": 0, "meals": meals})
		return
	}
	items, added, err := s.addItems(r, l.ID, names, "", "for "+clip(strings.Join(meals, ", "), 200))
	if err != nil {
		s.fail(w, err, "add")
		return
	}
	if added > 0 {
		s.act(ctx, "item", fmt.Sprintf("put %d things from the meal plan on %s", added, l.Name), "list", l.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "added": added, "meals": meals})
}

// handleApps is how the caller reaches each app (the Connected apps page).
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleMadeList lists what the caller made from one thing (?owner=letter&owner_id=3).
func (s *Server) handleMadeList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.URL.Query().Get("owner_id"), 10, 64)
	list, err := s.store.MadeFor(ctx, actorID(ctx), r.URL.Query().Get("owner"), id)
	if err != nil {
		s.fail(w, err, "made")
		return
	}
	writeJSON(w, http.StatusOK, list)
}
