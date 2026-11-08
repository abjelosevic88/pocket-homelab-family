package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-family/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- telling people ----
//
// A push goes to each person's own notification address (Pocket Alerts,
// ntfy, Apprise), only for what they asked to hear about (pick-ups, the
// shop, the board) and not in their quiet hours, except a pick-up that
// needs someone today. The evening before, at their hour, each grown-up
// gets "tomorrow for you": their pick-ups, what the children take along,
// chores and letters due.

func quiet(st store.Settings, now time.Time) bool {
	from, to := st.QuietFrom, st.QuietTo
	if from == to {
		return false
	}
	h := now.Hour()
	if from < to {
		return h >= from && h < to
	}
	return h >= from || h < to
}

// tell pushes a message to people in the family (never to the caller).
func (s *Server) tell(ctx context.Context, f *store.Family, people []int64, kind, title, body, path string) {
	me := meFrom(ctx)
	now := time.Now()
	for _, pid := range people {
		p := personByID(f, pid)
		if p == nil || p.UserID == 0 || (me != nil && p.ID == me.ID) {
			continue
		}
		st, err := s.store.GetSettings(ctx, p.UserID)
		if err != nil || st.NotifyURL == "" {
			continue
		}
		switch kind {
		case "runs":
			if !st.NotifyRuns {
				continue
			}
		case "shop":
			if !st.NotifyShop {
				continue
			}
		case "posts":
			if !st.NotifyPosts {
				continue
			}
		}
		urgent := kind == "runs" && strings.Contains(body, "today")
		if quiet(st, now) && !urgent {
			continue
		}
		prio := "info"
		if urgent {
			prio = "high"
		}
		t := title
		if len(familiesOf(ctx, s, p.UserID)) > 1 {
			t = f.Name + ": " + title
		}
		link := strings.TrimRight(s.cfg.BaseURL, "/") + path
		go s.push(context.WithoutCancel(ctx), p.UserID, st, t, body, link, prio)
	}
}

func familiesOf(ctx context.Context, s *Server, uid int64) []store.Family {
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return nil
	}
	list, _ := s.store.MyFamilies(ctx, uid, u.Email)
	return list
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link, priority string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: priority, Tags: []string{"house"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// evening runs every minute; at each person's own hour it sends "tomorrow for you".
func (s *Server) evening(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.NotifyDaily || st.NotifyURL == "" || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.tomorrowFor(ctx, uid, st, true)
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// tomorrowFor puts together (and sends, when asked) one person's "tomorrow for you".
func (s *Server) tomorrowFor(ctx context.Context, uid int64, st store.Settings, send bool) (string, bool) {
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return "", false
	}
	fams, err := s.store.MyFamilies(ctx, uid, u.Email)
	if err != nil || len(fams) == 0 {
		return "", false
	}
	tomorrow := store.AddDays(store.Today(), 1)
	var lines []string
	for _, fl := range fams {
		f, err := s.store.FamilyFor(ctx, uid, fl.ID)
		if err != nil || f.Me == nil {
			continue
		}
		p, err := s.planFor(ctx, f, tomorrow, tomorrow)
		if err != nil {
			continue
		}
		prefix := ""
		if len(fams) > 1 {
			prefix = f.Name + ": "
		}
		for _, x := range p.Runs {
			if x.State == store.RunSkipped || x.State == store.RunDone {
				continue
			}
			switch {
			case x.DriverID == f.Me.ID:
				lines = append(lines, fmt.Sprintf("%s%s %s", prefix, x.Time, runWords(f, &x)))
			case x.DriverID == 0 || x.State == store.RunAsking:
				if f.Me.CanEdit() {
					lines = append(lines, fmt.Sprintf("%s%s %s — nobody yet", prefix, x.Time, runWords(f, &x)))
				}
			}
		}
		if store.NoSchool(tomorrow, p.Terms) {
			lines = append(lines, prefix+"No school tomorrow")
		}
		for _, b := range bringOn(f, p, tomorrow) {
			lines = append(lines, fmt.Sprintf("%s%s takes %s", prefix, firstNonEmpty(nameOf(f, b.PersonID), "Take along:"), b.What))
		}
		for _, a := range s.agenda(f, p, tomorrow, tomorrow) {
			if a.Kind == "term" {
				continue
			}
			lines = append(lines, prefix+strings.TrimSpace(a.Time+" "+a.Title))
		}
		for _, l := range p.Letters {
			if l.DueOn != "" && l.DueOn <= tomorrow {
				lines = append(lines, prefix+"Letter: "+firstNonEmpty(l.Action, l.Title)+" (due "+dayWords(l.DueOn)+")")
			}
		}
		var mine []string
		for _, c := range choresOn(p.Chores, tomorrow) {
			if c.For(f.Me.ID) && c.Assign != "anyone" {
				mine = append(mine, c.Title)
			}
		}
		if len(mine) > 0 {
			lines = append(lines, prefix+"Your chores: "+strings.Join(mine, ", "))
		}
	}
	if len(lines) == 0 {
		return "", false
	}
	if len(lines) > 12 {
		lines = append(lines[:12], fmt.Sprintf("and %d more", len(lines)-12))
	}
	body := strings.Join(lines, "\n")
	if !send {
		return body, false
	}
	return body, s.push(ctx, uid, st, "Tomorrow", body, strings.TrimRight(s.cfg.BaseURL, "/")+"/?day=tomorrow", "info")
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Family", Body: notifyTestBody, Priority: "info", Tags: []string{"house"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow sends "tomorrow for you" now and says what it sent.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	body, sent := s.tomorrowFor(ctx, uid, st, true)
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "body": body})
}

const notifyTestBody = "Notifications work. Pick-ups given to you or asked of you, someone at the shop and new notes on the board arrive here, as you set them up."
