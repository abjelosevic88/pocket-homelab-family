package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// makeFamily makes Alice's family with a child and returns its id and the child's.
func makeFamily(t *testing.T, a *app) (fid, child int64) {
	t.Helper()
	f := a.ok(a.alice, "POST", "/api/v1/families", map[string]any{"name": "Alice's family"})
	fid = int64(num(f, "id"))
	c := a.ok(a.alice, "POST", "/api/v1/people", map[string]any{"name": "Mia", "role": "child"})
	return fid, int64(num(c, "id"))
}

func TestNoFamilyYet(t *testing.T) {
	a := newApp(t, nil)
	m := a.expect(404, a.bob, "GET", "/api/v1/today", nil)
	if m["code"] != "no_family" {
		t.Fatalf("code = %v, want no_family", m["code"])
	}
	if l := a.ok(a.bob, "GET", "/api/v1/families", nil); len(l["families"].([]any)) != 0 {
		t.Fatalf("bob has families: %v", l)
	}
}

// Somebody who is not in a family gets 404 for everything in it — an admin too.
func TestFamilyIsolation(t *testing.T) {
	a := newApp(t, nil)
	fid, child := makeFamily(t, a)
	run := a.ok(a.alice, "POST", "/api/v1/runs", map[string]any{"day": store.Today(), "time": "15:00", "kind": "pickup", "children": []int64{child}})
	ev := a.ok(a.alice, "POST", "/api/v1/events", map[string]any{"title": "Dentist", "day": store.Today()})
	ch := a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Dishes", "repeat": "daily", "assign": "one", "people": []int64{child}})
	lt := a.ok(a.alice, "POST", "/api/v1/letters", map[string]any{"title": "Trip"})
	po := a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"body": "Hello"})
	post := po["post"].(map[string]any)
	lists := a.list(a.alice, "/api/v1/lists")
	list := lists[0].(map[string]any)
	item := a.ok(a.alice, "POST", "/api/v1/lists/"+id(list)+"/items", map[string]any{"name": "milk"})
	itemID := id(item["items"].([]any)[0].(map[string]any))

	// Bob is in no family: asking for Alice's by id is a 404
	a.expect(404, a.bob, "GET", "/api/v1/family?family="+itoa(fid), nil)
	a.expect(404, a.bob, "GET", "/api/v1/today?family="+itoa(fid), nil)
	// Bob makes his own family; Alice's things are not in it
	a.ok(a.bob, "POST", "/api/v1/families", map[string]any{"name": "Bob's"})
	for _, p := range []string{"/api/v1/events/" + id(ev), "/api/v1/letters/" + id(lt), "/api/v1/lists/" + id(list) + "/items"} {
		a.expect(404, a.bob, "GET", p, nil)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/runs/"+id(run), map[string]any{"time": "16:00"})
	a.expect(404, a.bob, "POST", "/api/v1/runs/"+id(run)+"/take", nil)
	a.expect(404, a.bob, "POST", "/api/v1/chores/"+id(ch)+"/done", nil)
	a.expect(404, a.bob, "DELETE", "/api/v1/posts/"+id(post), nil)
	a.expect(404, a.bob, "PUT", "/api/v1/items/"+itemID, map[string]any{"done": true})
	a.expect(404, a.bob, "PUT", "/api/v1/people/"+itoa(child), map[string]any{"name": "x"})
	// and Alice (admin) does not see Bob's family
	bobs := a.ok(a.bob, "GET", "/api/v1/family", nil)
	a.expect(404, a.alice, "GET", "/api/v1/family?family="+id(bobs), nil)
	if runs := a.ok(a.bob, "GET", "/api/v1/runs", nil)["runs"].([]any); len(runs) != 0 {
		t.Fatalf("bob sees runs: %v", runs)
	}
}

// Adding someone by email links their account; they then share everything.
func TestInviteByEmail(t *testing.T) {
	a := newApp(t, nil)
	fid, child := makeFamily(t, a)
	bob := a.ok(a.alice, "POST", "/api/v1/people", map[string]any{"name": "Bob", "email": "BOB@example.com", "role": "grownup"})
	if int64(num(bob, "user_id")) != a.ids["Bob"] {
		t.Fatalf("bob not linked: %v", bob)
	}
	a.expect(409, a.alice, "POST", "/api/v1/people", map[string]any{"name": "Bob again", "email": "bob@example.com", "role": "grownup"})
	fams := a.ok(a.bob, "GET", "/api/v1/families", nil)
	if int64(num(fams, "current")) != fid {
		t.Fatalf("bob's current family = %v", fams["current"])
	}
	// Alice asks for someone to take a pick-up; Bob takes it
	run := a.ok(a.alice, "POST", "/api/v1/runs", map[string]any{"day": store.Today(), "time": "15:00", "kind": "pickup", "children": []int64{child}, "driver_id": a.personOf(fid, "Alice")})
	r := a.ok(a.alice, "POST", "/api/v1/runs/"+id(run)+"/ask", map[string]any{"note": "meeting"})
	if r["state"] != "asking" {
		t.Fatalf("state = %v", r["state"])
	}
	r = a.ok(a.bob, "POST", "/api/v1/runs/"+id(run)+"/take", nil)
	if r["state"] != "planned" || int64(num(r, "driver_id")) != int64(num(bob, "id")) {
		t.Fatalf("after take: %v", r)
	}
	// Bob is not a manager: he cannot add people, but can rename himself
	a.expect(403, a.bob, "POST", "/api/v1/people", map[string]any{"name": "X"})
	a.ok(a.bob, "PUT", "/api/v1/people/"+id(bob), map[string]any{"name": "Bobby"})
	a.expect(403, a.bob, "PUT", "/api/v1/people/"+itoa(child), map[string]any{"name": "Y"})
	// the activity says who did what
	acts := a.list(a.alice, "/api/v1/activity")
	if len(acts) == 0 || !strings.Contains(acts[0].(map[string]any)["text"].(string), "will pick up Mia") {
		t.Fatalf("activity: %v", acts)
	}
}

func (a *app) personOf(fid int64, name string) int64 {
	people, _ := a.st.ListPeople(context.Background(), fid, true)
	for _, p := range people {
		if strings.HasPrefix(p.Name, name) {
			return p.ID
		}
	}
	return 0
}

// A child with an account ticks their own chores and adds to the list, but does not plan.
func TestChildCanTickNotPlan(t *testing.T) {
	a := newApp(t, nil)
	fid, _ := makeFamily(t, a)
	kid := a.ok(a.alice, "POST", "/api/v1/people", map[string]any{"name": "Bob", "email": "bob@example.com", "role": "child"})
	kidID := int64(num(kid, "id"))
	a.expect(403, a.bob, "POST", "/api/v1/events", map[string]any{"title": "Party", "day": store.Today()})
	a.expect(403, a.bob, "POST", "/api/v1/runs", map[string]any{"day": store.Today(), "time": "15:00", "kind": "pickup", "title": "x"})
	ch := a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Tidy", "repeat": "daily", "assign": "one", "people": []int64{kidID}, "stars": 2})
	a.expect(403, a.bob, "POST", "/api/v1/chores/"+id(ch)+"/done", map[string]any{"person_id": a.personOf(fid, "Alice")})
	d := a.ok(a.bob, "POST", "/api/v1/chores/"+id(ch)+"/done", nil)
	if int64(num(d["done"].(map[string]any), "person_id")) != kidID {
		t.Fatalf("done by %v", d)
	}
	stars := a.ok(a.alice, "GET", "/api/v1/stars", nil)
	found := false
	for _, p := range stars["people"].([]any) {
		m := p.(map[string]any)
		if int64(num(m, "person_id")) == kidID && num(m, "stars") == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("stars: %v", stars)
	}
	a.ok(a.bob, "POST", "/api/v1/items", map[string]any{"name": "chocolate"})
	a.ok(a.bob, "POST", "/api/v1/posts", map[string]any{"body": "Can we have pizza?"})
}

func TestChoreTurnsAndUndo(t *testing.T) {
	a := newApp(t, nil)
	_, child := makeFamily(t, a)
	other := int64(num(a.ok(a.alice, "POST", "/api/v1/people", map[string]any{"name": "Leo", "role": "child"}), "id"))
	ch := a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Dishes", "repeat": "daily", "assign": "turns", "people": []int64{child, other}})
	if int64(num(ch, "whose")) != child || ch["next_due"] != store.Today() {
		t.Fatalf("new chore: %v", ch)
	}
	d := a.ok(a.alice, "POST", "/api/v1/chores/"+id(ch)+"/done", map[string]any{"person_id": child})
	c := d["chore"].(map[string]any)
	if int64(num(c, "whose")) != other || c["next_due"] != store.AddDays(store.Today(), 1) {
		t.Fatalf("after done: %v", c)
	}
	c = a.ok(a.alice, "POST", "/api/v1/chores/"+id(ch)+"/undo", nil)
	if int64(num(c, "whose")) != child || c["next_due"] != store.Today() {
		t.Fatalf("after undo: %v", c)
	}
	// a one-off goes away once done
	once := a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Fix the shelf", "repeat": "once", "assign": "anyone"})
	d = a.ok(a.alice, "POST", "/api/v1/chores/"+id(once)+"/done", nil)
	if d["chore"].(map[string]any)["active"] != false {
		t.Fatalf("one-off still active: %v", d)
	}
}

func TestNextAfter(t *testing.T) {
	c := &store.Chore{Repeat: "weekly", Weekdays: []int{2, 5}}        // Tue, Fri
	if got := store.NextAfter(c, "2026-11-03"); got != "2026-11-06" { // Tue → Fri
		t.Fatalf("weekly Tue/Fri after Tue = %s", got)
	}
	if got := store.NextAfter(c, "2026-11-06"); got != "2026-11-10" { // Fri → Tue
		t.Fatalf("weekly Tue/Fri after Fri = %s", got)
	}
	m := &store.Chore{Repeat: "monthly", NextDue: "2026-01-31"}
	if got := store.NextAfter(m, "2026-01-31"); got != "2026-03-03" && got != "2026-02-28" && got != "2026-03-02" {
		t.Fatalf("monthly = %s", got)
	}
	e := &store.Chore{Repeat: "every", EveryDays: 4}
	if got := store.NextAfter(e, "2026-11-01"); got != "2026-11-05" {
		t.Fatalf("every 4 = %s", got)
	}
}

// A term-time plan has no runs in the holidays; a hand-changed run survives a plan edit.
func TestPlansAndHolidays(t *testing.T) {
	a := newApp(t, nil)
	_, child := makeFamily(t, a)
	from := store.Today()
	a.ok(a.alice, "POST", "/api/v1/terms", map[string]any{"name": "Holiday", "kind": "holiday", "starts_on": store.AddDays(from, 7), "ends_on": store.AddDays(from, 13)})
	p := a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"kind": "pickup", "place": "School", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "time": "13:30", "children": []int64{child}, "term_only": true})
	runs := a.ok(a.alice, "GET", "/api/v1/runs?from="+from+"&to="+store.AddDays(from, 20), nil)["runs"].([]any)
	if len(runs) != 21-7 {
		t.Fatalf("runs = %d, want 14 (7 holiday days skipped)", len(runs))
	}
	first := runs[0].(map[string]any)
	a.ok(a.alice, "PUT", "/api/v1/runs/"+id(first), map[string]any{"time": "14:00"})
	a.ok(a.alice, "PUT", "/api/v1/plans/"+id(p), map[string]any{"time": "13:45"})
	runs = a.ok(a.alice, "GET", "/api/v1/runs?from="+from+"&to="+from, nil)["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["time"] != "14:00" {
		t.Fatalf("hand-changed run was rewritten: %v", runs)
	}
	runs = a.ok(a.alice, "GET", "/api/v1/runs?from="+store.AddDays(from, 1)+"&to="+store.AddDays(from, 1), nil)["runs"].([]any)
	if runs[0].(map[string]any)["time"] != "13:45" {
		t.Fatalf("plan edit not applied: %v", runs)
	}
	// nobody down: Today lists it under needs
	today := a.ok(a.alice, "GET", "/api/v1/today?quick=1", nil)
	if len(today["needs"].([]any)) == 0 {
		t.Fatalf("no runs need someone: %v", today["needs"])
	}
}

func TestWeeklyEventOccurrences(t *testing.T) {
	e := store.Event{Title: "Football", Day: "2026-11-02", Repeat: "weekly", Weekdays: []int{2, 4}}
	got := store.Occurrences(e, "2026-11-01", "2026-11-14")
	if len(got) != 4 || got[0].Day != "2026-11-03" || got[3].Day != "2026-11-12" {
		t.Fatalf("occurrences = %v", got)
	}
	y := store.Event{Title: "Anniversary", Day: "2012-06-21", Repeat: "yearly"}
	if got := store.Occurrences(y, "2026-06-01", "2026-06-30"); len(got) != 1 || got[0].Day != "2026-06-21" {
		t.Fatalf("yearly = %v", got)
	}
}

func TestShoppingOnceAndQuantities(t *testing.T) {
	a := newApp(t, nil)
	makeFamily(t, a)
	r := a.ok(a.alice, "POST", "/api/v1/items", map[string]any{"name": "2 milk\nbread\nEggs x10"})
	if num(r, "added") != 3 {
		t.Fatalf("added = %v", r)
	}
	r = a.ok(a.alice, "POST", "/api/v1/items", map[string]any{"name": "Milk"})
	if num(r, "added") != 0 {
		t.Fatalf("milk added twice: %v", r)
	}
	items := r["items"].([]any)
	if items[0].(map[string]any)["qty"] != "2" {
		t.Fatalf("qty lost: %v", items)
	}
	for in, want := range map[string][2]string{"2 milk": {"milk", "2"}, "Eggs x10": {"Eggs", "10"}, "rice, 1 kg": {"rice", "1 kg"}, "bread": {"bread", ""}} {
		if n, q := splitQty(in); n != want[0] || q != want[1] {
			t.Fatalf("splitQty(%q) = %q %q", in, n, q)
		}
	}
	if store.GuessAisle("Bananas") != "Fruit & veg" || store.GuessAisle("mlijeko") != "Dairy & eggs" {
		t.Fatal("aisles")
	}
}

// Exporting and importing the same file twice adds things only once.
func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	_, child := makeFamily(t, a)
	a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Dishes", "repeat": "daily", "assign": "one", "people": []int64{child}})
	a.ok(a.alice, "POST", "/api/v1/events", map[string]any{"title": "Dentist", "day": store.Today(), "people": []int64{child}})
	a.ok(a.alice, "POST", "/api/v1/plans", map[string]any{"kind": "pickup", "place": "School", "weekdays": []int{1, 2}, "time": "13:30", "children": []int64{child}})
	a.ok(a.alice, "POST", "/api/v1/info", map[string]any{"section": "health", "label": "Allergy", "value": "Peanuts", "person_id": child})
	a.ok(a.alice, "POST", "/api/v1/items", map[string]any{"name": "milk"})
	doc := a.ok(a.alice, "GET", "/api/v1/export.json", nil)
	// into Bob's own, empty family
	a.ok(a.bob, "POST", "/api/v1/families", map[string]any{"name": "Bob's"})
	first := a.ok(a.bob, "POST", "/api/v1/import", doc)
	if num(first, "chores") != 1 || num(first, "events") != 1 || num(first, "plans") != 1 || num(first, "info") != 1 || num(first, "items") != 1 || num(first, "people") < 1 {
		t.Fatalf("first import: %v", first)
	}
	second := a.ok(a.bob, "POST", "/api/v1/import", doc)
	for k, v := range second {
		if num(map[string]any{"v": v}, "v") != 0 {
			t.Fatalf("second import added %s: %v", k, second)
		}
	}
	info := a.list(a.bob, "/api/v1/info")
	if len(info) != 1 || int64(num(info[0].(map[string]any), "person_id")) == 0 {
		t.Fatalf("info person not mapped: %v", info)
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "BAM", "notify_shop": false, "title": "Our box"})
	b := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if b["currency"] != "EUR" || b["notify_shop"] != true {
		t.Fatalf("bob got alice's settings: %v", b)
	}
	if b["title"] != "Our box" {
		t.Fatalf("box title not shared: %v", b["title"])
	}
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"title": "Bob's box"})
	if m := a.ok(a.alice, "GET", "/api/v1/settings", nil); m["title"] != "Our box" {
		t.Fatalf("a member changed the box title: %v", m["title"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "euros"})
}

func TestLinkScopes(t *testing.T) {
	req := func(method, path string) bool {
		return linkAllows([]string{"family:read"}, httptest.NewRequest(method, path, nil))
	}
	if !req("GET", "/api/v1/today") || !req("GET", "/api/v1/day/2026-11-08") || !req("GET", "/api/v1/letters/3") {
		t.Fatal("family:read should read")
	}
	if req("POST", "/api/v1/items") || req("GET", "/api/v1/settings") || req("POST", "/api/v1/make") || req("GET", "/api/v1/export.json") {
		t.Fatal("family:read reaches too far")
	}
	w := func(method, path string) bool {
		return linkAllows([]string{"family:write"}, httptest.NewRequest(method, path, nil))
	}
	if !w("POST", "/api/v1/items") || !w("POST", "/api/v1/runs/4/take") || !w("POST", "/api/v1/chores/2/done") || !w("DELETE", "/api/v1/events/9") {
		t.Fatal("family:write should write")
	}
	if w("POST", "/api/v1/people") || w("DELETE", "/api/v1/family") || w("POST", "/api/v1/import") || w("PUT", "/api/v1/settings") {
		t.Fatal("family:write reaches too far")
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	today := a.ok(a.alice, "GET", "/api/v1/today?quick=1", nil)
	if len(today["chores"].([]any)) == 0 || len(today["letters"].([]any)) == 0 {
		t.Fatalf("demo today is empty: %v", today)
	}
	week := a.ok(a.alice, "GET", "/api/v1/week?quick=1", nil)
	if len(week["days"].([]any)) != 7 {
		t.Fatalf("week: %v", week)
	}
	if s := a.ok(a.alice, "GET", "/api/v1/summary", nil); len(s["needs_someone"].([]any)) == 0 {
		t.Fatalf("summary: %v", s)
	}
}
