package server

import (
	"context"
	"math/rand"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- demo data (DEMO_DATA=true): one busy family ----
//
// The owner and their partner Ana, Luka (11, with his own account) and Mila
// (7), grandma Fata who helps with pick-ups: school runs on a weekly rota,
// football and music school, chores in turns with stars, two shopping
// lists, letters from school, the school year with its holidays, notes on
// the board and the family info. Three weeks of history.

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if _, _, done := s.store.CacheGet(ctx, "demo:seeded"); done {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	owner, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	account := func(email, name string) (*store.User, error) {
		u, err := s.store.GetUserByEmail(ctx, email)
		if err == nil {
			return u, nil
		}
		u = &store.User{Email: email, Name: name, Role: "member"}
		return u, s.store.CreateUser(ctx, u)
	}
	ana, err := account("ana@demo.pocket", "Ana Kovač")
	if err != nil {
		return err
	}
	lukaU, err := account("luka@demo.pocket", "Luka Kovač")
	if err != nil {
		return err
	}
	today := store.Today()
	year := atoi(today[:4])
	f := &store.Family{Name: "The Kovač family", Emoji: "🏡", Color: "#e8603c"}
	me := &store.Person{Name: firstNonEmpty(owner.Name, "Emir Kovač"), Email: owner.Email, Role: store.RoleGrownup, Color: "#2b7a9e", Emoji: "🧔", Phone: "+387 61 234 567",
		Birthday: itoa(int64(year-41)) + "-03-14"}
	if err := s.store.CreateFamily(ctx, uid, f, me); err != nil {
		return err
	}
	add := func(p *store.Person) int64 {
		p.FamilyID = f.ID
		_ = s.store.CreatePerson(ctx, p)
		return p.ID
	}
	pAna := add(&store.Person{Name: "Ana Kovač", Email: ana.Email, Role: store.RoleGrownup, Manager: true, Color: "#d6336c", Emoji: "👩", Phone: "+387 62 345 678",
		Birthday: itoa(int64(year-39)) + "-" + store.AddDays(today, 23)[5:]})
	pLuka := add(&store.Person{Name: "Luka", Email: lukaU.Email, Role: store.RoleChild, Color: "#0c8a6a", Emoji: "⚽", Birthday: itoa(int64(year-11)) + "-05-02",
		School: "OŠ Mula Mustafa Bašeskija", ClassName: "6b", Notes: "Football Tue/Thu, music school Wed"})
	pMila := add(&store.Person{Name: "Mila", Role: store.RoleChild, Color: "#7c3aed", Emoji: "🦄", Birthday: itoa(int64(year-7)) + "-" + store.AddDays(today, 5)[5:],
		School: "OŠ Mula Mustafa Bašeskija", ClassName: "2a", Notes: "Allergic to peanuts — EpiPen in her school bag"})
	pFata := add(&store.Person{Name: "Grandma Fata", Role: store.RoleHelper, Color: "#b0571d", Emoji: "👵", Phone: "+387 33 456 789", Notes: "Picks Mila up Mon–Wed"})
	st, _ := s.store.GetSettings(ctx, uid)
	st.FamilyID = f.ID
	_ = s.store.SaveUserSettings(ctx, uid, st)
	for _, u := range []int64{ana.ID, lukaU.ID} {
		ust, _ := s.store.GetSettings(ctx, u)
		ust.FamilyID = f.ID
		_ = s.store.SaveUserSettings(ctx, u, ust)
	}
	meID := me.ID
	school := "School (OŠ Mula Mustafa)"

	// the school year
	t := func(name, kind, a, b string) {
		_ = s.store.SaveTerm(ctx, &store.Term{FamilyID: f.ID, Name: name, Kind: kind, StartsOn: a, EndsOn: b})
	}
	t("Autumn term", "term", itoa(int64(year))+"-09-01", itoa(int64(year))+"-12-30")
	t("Winter holiday", "holiday", itoa(int64(year))+"-12-31", itoa(int64(year+1))+"-01-18")
	t("Teacher training day", "closed", store.AddDays(today, 12), store.AddDays(today, 12))
	t("Spring term", "term", itoa(int64(year+1))+"-01-19", itoa(int64(year+1))+"-06-19")
	for _, w := range []struct {
		pid  int64
		ends string
		b    map[int]string
	}{
		{pMila, "13:15", map[int]string{2: "PE kit", 4: "Library book"}},
		{pLuka, "14:30", map[int]string{1: "Swimming things", 3: "Art apron", 5: "PE kit"}},
	} {
		var days []store.SchoolDay
		for d := 1; d <= 5; d++ {
			days = append(days, store.SchoolDay{Weekday: d, Starts: "08:00", Ends: w.ends, Bring: w.b[d]})
		}
		_ = s.store.SetSchoolWeek(ctx, f.ID, w.pid, days)
	}

	// weekly runs
	plans := []*store.RunPlan{
		{Kind: "dropoff", Title: "", Place: school, Weekdays: []int{1, 2, 3, 4, 5}, Time: "07:45", Children: []int64{pMila, pLuka}, DriverID: meID,
			Rota: map[string]int64{"2": pAna, "4": pAna}, TermOnly: true},
		{Kind: "pickup", Place: school, Weekdays: []int{1, 2, 3, 4, 5}, Time: "13:30", Children: []int64{pMila}, DriverID: pFata,
			Rota: map[string]int64{"4": pAna, "5": meID}, TermOnly: true},
		{Kind: "pickup", Place: "Football (FK Željo, Grbavica)", Weekdays: []int{2, 4}, Time: "18:30", Children: []int64{pLuka}, DriverID: meID},
		{Kind: "dropoff", Place: "Music school", Weekdays: []int{3}, Time: "16:00", Children: []int64{pLuka}, DriverID: pAna, TermOnly: true},
	}
	from := store.AddDays(today, -21)
	for _, p := range plans {
		p.FamilyID, p.Active, p.StartsOn = f.ID, true, from
		if err := s.store.CreatePlan(ctx, uid, p); err != nil {
			return err
		}
	}
	// three weeks back: let the plans make their runs, then mark the past ones done
	if err := s.ensureDemoRuns(ctx, f.ID, plans, from, store.AddDays(today, 14)); err != nil {
		return err
	}
	runs, _ := s.store.ListRuns(ctx, f.ID, from, store.AddDays(today, 14))
	asked := false
	for i := range runs {
		x := &runs[i]
		switch {
		case x.Day < today:
			x.State, x.DoneBy, x.DoneAt = store.RunDone, x.DriverID, x.Day+"T"+x.Time+":00Z"
			_ = s.store.SaveRun(ctx, x)
		case !asked && x.Day > today && x.Kind == "pickup" && x.Time == "18:30":
			// the owner can't make one football pick-up: they asked
			x.State, x.AskedBy, x.AskedNote = store.RunAsking, meID, "I'm at a work dinner that evening"
			_ = s.store.SaveRun(ctx, x)
			asked = true
		}
	}
	sat := today
	for store.Weekday(sat) != 6 {
		sat = store.AddDays(sat, 1)
	}
	_ = s.store.CreateRun(ctx, uid, &store.Run{FamilyID: f.ID, Day: today, Time: "17:30", Kind: "pickup", Children: []int64{pMila}, Place: "Lejla's house (Koševo 14)", DriverID: pAna, Notes: "Mila is at Lejla's after the swimming pool"})
	_ = s.store.CreateRun(ctx, uid, &store.Run{FamilyID: f.ID, Day: today, Time: "19:15", Kind: "pickup", Children: []int64{pLuka}, Place: "Cinema City (Alta)"})
	_ = s.store.CreateRun(ctx, uid, &store.Run{FamilyID: f.ID, Day: sat, Time: "15:00", Kind: "lift", Children: []int64{pLuka}, Place: "Amar's birthday party (Bowling, SCC)", Notes: "Bring the present from the hall cupboard"})
	_ = s.store.CreateRun(ctx, uid, &store.Run{FamilyID: f.ID, Day: sat, Time: "18:00", Kind: "pickup", Children: []int64{pLuka}, Place: "Amar's birthday party (Bowling, SCC)", DriverID: pAna})

	// chores
	chores := []*store.Chore{
		{Title: "Dishes after dinner", Emoji: "🍽️", Repeat: "daily", Assign: "turns", People: []int64{pLuka, pMila}, Stars: 2, PartOfDay: "evening"},
		{Title: "Feed Mrvica", Emoji: "🐈", Repeat: "daily", Assign: "one", People: []int64{pMila}, Stars: 1, PartOfDay: "morning", Notes: "Half a pouch, fresh water"},
		{Title: "Take out the bins", Emoji: "🗑️", Repeat: "weekly", Weekdays: []int{2, 5}, Assign: "one", People: []int64{meID}, PartOfDay: "evening"},
		{Title: "Tidy your room", Emoji: "🧸", Repeat: "weekly", Weekdays: []int{6}, Assign: "one", People: []int64{pLuka, pMila}, Stars: 3},
		{Title: "Hoover the living room", Emoji: "🧹", Repeat: "weekly", Weekdays: []int{6}, Assign: "turns", People: []int64{pAna, meID}},
		{Title: "Water the plants", Emoji: "🪴", Repeat: "every", EveryDays: 4, Assign: "turns", People: []int64{pLuka, pMila}, Stars: 1},
		{Title: "Change the bed sheets", Emoji: "🛏️", Repeat: "every", EveryDays: 14, Assign: "anyone"},
		{Title: "Homework checked", Emoji: "📚", Repeat: "weekly", Weekdays: []int{1, 2, 3, 4}, Assign: "anyone", PartOfDay: "afternoon"},
	}
	for _, c := range chores {
		c.FamilyID = f.ID
		c.NextDue = store.FirstDue(c, from)
		if err := s.store.CreateChore(ctx, uid, c); err != nil {
			return err
		}
		// tick it off on most days it was due, by whoever's turn it was
		for i := 0; i < 60; i++ {
			cur, err := s.store.GetChore(ctx, f.ID, c.ID)
			if err != nil || !cur.Active || cur.NextDue >= today {
				break
			}
			if rng.Intn(10) < 8 {
				who := cur.Whose
				if who == 0 {
					who = []int64{meID, pAna}[rng.Intn(2)]
				}
				if _, err := s.store.DoChore(ctx, uid, cur, who, cur.NextDue); err != nil {
					return err
				}
			} else {
				cur.NextDue = store.NextAfter(cur, cur.NextDue)
				if cur.Assign == "turns" && len(cur.People) > 0 {
					cur.Turn = (cur.Turn + 1) % len(cur.People)
				}
				_ = s.store.UpdateChore(ctx, cur)
			}
		}
	}
	// a chore ticked off today already
	if list, err := s.store.ListChores(ctx, f.ID, false); err == nil {
		for i := range list {
			if list[i].Title == "Feed Mrvica" && list[i].NextDue <= today {
				_, _ = s.store.DoChore(ctx, uid, &list[i], pMila, today)
			}
		}
	}

	// the family's plans
	nextWd := func(wd int) string {
		d := store.AddDays(today, 1)
		for store.Weekday(d) != wd {
			d = store.AddDays(d, 1)
		}
		return d
	}
	events := []*store.Event{
		{Title: "Football training", Day: from, StartTime: "17:00", EndTime: "18:30", Place: "FK Željo, Grbavica", People: []int64{pLuka}, Kind: "activity", Repeat: "weekly", Weekdays: []int{2, 4}, Bring: "Football boots, shin pads, water"},
		{Title: "Music school — guitar", Day: from, StartTime: "16:15", EndTime: "17:00", Place: "Music school", People: []int64{pLuka}, Kind: "activity", Repeat: "weekly", Weekdays: []int{3}, Bring: "Guitar and notes"},
		{Title: "Swimming pool with Lejla", Day: today, StartTime: "14:00", EndTime: "16:30", Place: "Olympic pool Otoka", People: []int64{pMila}, Kind: "activity", Bring: "Swimsuit, towel, goggles"},
		{Title: "Luka at the cinema with friends", Day: today, StartTime: "17:00", EndTime: "19:00", Place: "Cinema City (Alta)", People: []int64{pLuka}, Kind: "family"},
		{Title: "Mila at the dentist", Day: store.AddDays(today, 3), StartTime: "10:30", EndTime: "11:00", Place: "Dr. Begić, Ferhadija 12", People: []int64{pMila}, Kind: "appointment"},
		{Title: "Parents' evening", Day: nextWd(3), StartTime: "18:00", EndTime: "19:30", Place: school, People: []int64{meID, pAna}, Kind: "school"},
		{Title: "Grandma's lunch", Day: nextWd(7), StartTime: "13:00", Place: "Grandma Fata's", People: []int64{meID, pAna, pLuka, pMila}, Kind: "family", Bring: "The cake from the fridge"},
		{Title: "Mila's birthday party", Day: store.AddDays(today, 6), StartTime: "16:00", EndTime: "18:00", Place: "Home", People: []int64{pMila}, Kind: "family", Notes: "8 friends coming — Ana has the list"},
		{Title: "Anniversary", Day: itoa(int64(year-14)) + "-06-21", Kind: "family", Repeat: "yearly", People: []int64{meID, pAna}},
	}
	for _, e := range events {
		e.FamilyID = f.ID
		if e.People == nil {
			e.People = []int64{}
		}
		if err := s.store.CreateEvent(ctx, uid, e); err != nil {
			return err
		}
	}

	// letters from school
	trip := &store.Letter{FamilyID: f.ID, Title: "School trip to Jahorina", Sender: "Ms. Mehić, class 2a", ReceivedOn: store.AddDays(today, -2), People: []int64{pMila},
		Kind: "payment", Action: "Sign the slip and pay 25 € by Friday", DueOn: store.AddDays(today, 4), Amount: 2500, Currency: "EUR", State: "todo",
		Summary: "Class 2a goes to Jahorina for a day in the snow. Sign the permission slip and pay 25 € (bus and lunch) by Friday.",
		Body:    "Dear parents,\n\nOn " + dayWords(store.AddDays(today, 9)) + " class 2a is going on a day trip to Jahorina. We leave school at 8:00 and come back around 16:00. Children need warm clothes, gloves, a hat and a packed snack.\n\nPlease sign the attached slip and pay 25 € for the bus and lunch by Friday.\n\nThank you,\nMs. Mehić"}
	_ = s.store.CreateLetter(ctx, uid, trip)
	_ = s.store.CreateEvent(ctx, uid, &store.Event{FamilyID: f.ID, Title: "School trip to Jahorina", Day: store.AddDays(today, 9), StartTime: "08:00", EndTime: "16:00",
		Place: "Jahorina (bus from school)", People: []int64{pMila}, Kind: "school", Bring: "Warm clothes, gloves, hat, a snack", LetterID: trip.ID})
	for _, l := range []*store.Letter{
		{Title: "Football club fee for November", Sender: "FK Željo youth", ReceivedOn: store.AddDays(today, -5), People: []int64{pLuka}, Kind: "payment",
			Action: "Pay 30 € club fee", DueOn: store.AddDays(today, 10), Amount: 3000, Currency: "EUR", State: "todo", Summary: "The monthly club fee for November is 30 €."},
		{Title: "Photo day", Sender: "School office", ReceivedOn: store.AddDays(today, -16), People: []int64{pLuka, pMila}, Kind: "bring", Action: "Smart clothes on Thursday",
			DueOn: store.AddDays(today, -10), State: "done", Summary: "School photos are taken on Thursday; children should wear something smart."},
		{Title: "Head lice in class 2a", Sender: "School nurse", ReceivedOn: store.AddDays(today, -8), People: []int64{pMila}, Kind: "info", State: "none",
			Summary: "A few cases of head lice in class 2a. Please check your child's hair this week."},
	} {
		l.FamilyID = f.ID
		_ = s.store.CreateLetter(ctx, uid, l)
		if l.State == "done" {
			l.DoneBy, l.DoneAt = pAna, store.Now()
			_ = s.store.SaveLetter(ctx, l)
		}
	}

	// shopping
	lists, _ := s.store.ListLists(ctx, f.ID)
	groceries := lists[0].ID
	pharmacy := &store.List{FamilyID: f.ID, Name: "Pharmacy", Emoji: "💊"}
	_ = s.store.CreateList(ctx, pharmacy)
	for i, it := range []struct {
		name, qty string
		by        int64
		done      bool
	}{
		{"Milk", "2 l", pAna, false}, {"Bread", "", meID, false}, {"Eggs", "10", pAna, false}, {"Bananas", "", pMila, false}, {"Minced beef", "500 g", pAna, false},
		{"Rice", "1 kg", meID, false}, {"Yoghurt", "4", pLuka, false}, {"Dish soap", "", pAna, false}, {"Apples", "1 kg", meID, true}, {"Cheese", "", pAna, true},
		{"Kajmak", "", meID, false}, {"Juice for the party", "3", pAna, false}, {"Balloons", "a bag", pMila, false},
	} {
		x := &store.Item{FamilyID: f.ID, ListID: groceries, Name: it.name, Qty: it.qty, AddedBy: it.by}
		_, _ = s.store.AddItem(ctx, x)
		if it.done {
			x.Done, x.DoneBy, x.DoneAt = true, pAna, store.AddDays(today, -1)+"T17:2"+itoa(int64(i%10))+":00Z"
			_ = s.store.SaveItem(ctx, x)
		}
	}
	for _, n := range []string{"Children's paracetamol", "Plasters"} {
		_, _ = s.store.AddItem(ctx, &store.Item{FamilyID: f.ID, ListID: pharmacy.ID, Name: n, AddedBy: pAna})
	}

	// the board
	for _, p := range []struct {
		by     int64
		body   string
		pinned bool
		seen   []int64
	}{
		{pAna, "Grandma comes for lunch on Sunday 🍲 — please be home by 13:00", true, []int64{meID, pLuka}},
		{meID, "The car is in for a service on Thursday — the school run that day is on foot or by bus 🚌", false, []int64{pAna}},
		{pLuka, "Who ate the last yoghurt?? 😄", false, nil},
	} {
		x := &store.Post{FamilyID: f.ID, PersonID: p.by, Body: p.body, Pinned: p.pinned}
		_ = s.store.CreatePost(ctx, x)
		for _, who := range p.seen {
			_, _ = s.store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO post_seen (post_id, person_id) VALUES (?, ?)`, x.ID, who)
		}
	}

	// the family info
	for _, in := range []store.Info{
		{Section: "emergency", Label: "Emergency", Value: "Call 112 (ambulance 124, police 122, fire 123)", Phone: "112"},
		{Section: "emergency", Label: "Grandma Fata", Value: "Lives 5 minutes away, has a key", Phone: "+387 33 456 789", PersonID: pFata},
		{Section: "emergency", Label: "Neighbour (Selma, flat 4)", Value: "Has a spare key", Phone: "+387 61 987 654"},
		{Section: "health", Label: "Allergy", Value: "Peanuts — EpiPen in her school bag and in the kitchen drawer", PersonID: pMila},
		{Section: "health", Label: "Family doctor", Value: "Dr. Hodžić, Dom zdravlja Centar", Phone: "+387 33 292 000"},
		{Section: "health", Label: "Dentist", Value: "Dr. Begić, Ferhadija 12", Phone: "+387 33 200 100"},
		{Section: "school", Label: "School office", Value: "OŠ Mula Mustafa Bašeskija", Phone: "+387 33 444 555"},
		{Section: "school", Label: "Class teacher", Value: "Ms. Mehić (2a)", PersonID: pMila},
		{Section: "home", Label: "Wi-Fi", Value: "KovacHome — sunflower42"},
		{Section: "home", Label: "Water shut-off", Value: "Under the kitchen sink, the blue tap"},
		{Section: "home", Label: "Fuse box", Value: "In the hall cupboard, top shelf", OnSheet: false},
		{Section: "sitter", Label: "Bedtime", Value: "Mila 20:00, Luka 21:30 (21:00 on school nights)"},
		{Section: "sitter", Label: "Screens", Value: "One hour after homework; no phones at dinner"},
		{Section: "sitter", Label: "Mrvica the cat", Value: "Fed in the morning; don't let her out at night"},
	} {
		in := in
		in.FamilyID = f.ID
		if in.Label != "Fuse box" {
			in.OnSheet = true
		}
		_ = s.store.SaveInfo(ctx, uid, &in)
	}

	// who did what lately
	for _, a := range []store.Activity{
		{PersonID: pAna, Kind: "item", Text: "put Juice for the party on Groceries"},
		{PersonID: pLuka, Kind: "chore", Text: "did Dishes after dinner"},
		{PersonID: meID, Kind: "run", Text: "asked who can pick up Luka (Football) on a work dinner evening"},
		{PersonID: pAna, Kind: "letter", Text: "added the letter School trip to Jahorina"},
	} {
		a := a
		a.FamilyID = f.ID
		s.store.AddActivity(ctx, &a)
	}
	return s.store.CachePut(ctx, "demo:seeded", "1")
}

// ensureDemoRuns makes the runs of the demo's plans for the past too (the
// store only looks a week back).
func (s *Server) ensureDemoRuns(ctx context.Context, fid int64, plans []*store.RunPlan, from, to string) error {
	terms, _ := s.store.ListTerms(ctx, fid)
	for _, p := range plans {
		for d := from; d <= to; d = store.AddDays(d, 1) {
			if !p.FallsOn(d, terms) {
				continue
			}
			if _, err := s.store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO runs (family_id, plan_id, day, time, kind, title, place, children, driver_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				fid, p.ID, d, p.Time, p.Kind, p.Title, p.Place, idsText(p.Children), nullID(p.DriverOn(store.Weekday(d)))); err != nil {
				return err
			}
		}
	}
	return nil
}

func idsText(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = itoa(id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
