package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- Pocket AI for the family ----
//
// Three helpers, each private to whoever asks and never saving on its own:
// read a letter from school (pasted text) into what it asks for, dates and
// things to take; turn a sentence ("Ana picks Mila up from football on
// Thursday at 6") into a pick-up, an event, a chore, a shopping item or a
// note; and say the week in a few lines with what nobody has taken yet.
// The answer is a suggestion the person checks before anything is added.

func (s *Server) withAI(w http.ResponseWriter, r *http.Request, run func(ctx context.Context, complete func(system, prompt string, asJSON bool) (string, error)) (any, error)) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	cl, _, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusConflict, firstNonEmpty(why, "AI is not set up (Settings → AI)"))
		return
	}
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-ctx.Done():
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	out, err := run(cctx, func(system, prompt string, asJSON bool) (string, error) {
		return cl.Complete(cctx, system, prompt, asJSON)
	})
	if err != nil {
		var ue *userErr
		if errors.As(err, &ue) {
			writeErr(w, http.StatusBadRequest, ue.msg)
			return
		}
		writeErr(w, http.StatusBadGateway, "AI: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// familyContext tells the AI who is who.
func familyContext(f *store.Family, st store.Settings) string {
	var b strings.Builder
	now := time.Now()
	fmt.Fprintf(&b, "Today is %s (%s). Times are local, 24-hour.\n", now.Format("2006-01-02"), now.Format("Monday"))
	fmt.Fprintf(&b, "The family \"%s\" (person id: name, role):\n", f.Name)
	for _, p := range f.People {
		if p.Archived {
			continue
		}
		extra := ""
		if p.School != "" {
			extra = ", school: " + p.School
		}
		fmt.Fprintf(&b, "- %d: %s, %s%s\n", p.ID, p.Name, roleWords(p.Role), extra)
	}
	fmt.Fprintf(&b, "The person asking is %d (%s). Money is in %s unless said otherwise.\n", f.Me.ID, f.Me.Name, firstNonEmpty(st.Currency, "EUR"))
	return b.String()
}

// parseObject reads the JSON object in an answer (fences and chatter around it are fine).
func parseObject(s string, v any) error {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	return json.Unmarshal([]byte(s), v)
}

type letterRead struct {
	Title    string    `json:"title"`
	Sender   string    `json:"sender"`
	Summary  string    `json:"summary"`
	Kind     string    `json:"kind"`
	Action   string    `json:"action"`
	DueOn    string    `json:"due_on"`
	Amount   float64   `json:"amount"`
	Currency string    `json:"currency"`
	People   []int64   `json:"people"`
	Events   []aiEvent `json:"events"`
}

type aiEvent struct {
	Title     string `json:"title"`
	Day       string `json:"day"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Place     string `json:"place"`
	Bring     string `json:"bring"`
}

const letterSystem = `You read letters and messages that a school, a kindergarten or a club sends to parents, and say plainly what they ask for.
Answer with ONE JSON object, nothing else:
{"title": short title (max 60 chars), "sender": who sent it ("" if unknown),
 "summary": 1–3 short sentences in plain words, in the language of the letter,
 "kind": "permission" (a slip to sign/return) | "payment" (money to pay) | "bring" (something to take along) | "event" (a date to remember) | "info" (nothing to do),
 "action": what the parents must do, short ("Sign and return the slip", "Pay 15 € for the trip"), "" if nothing,
 "due_on": the deadline as YYYY-MM-DD ("" if none),
 "amount": the money asked for as a number (0 if none), "currency": ISO code ("" if none),
 "people": ids of the children it is about (from the family list; [] if unclear),
 "events": [{"title", "day": YYYY-MM-DD, "start_time": "HH:MM" or "", "end_time": "HH:MM" or "", "place", "bring": what to take that day or ""}]}
Work out dates from the letter and today's date ("next Friday"). Never invent a date, an amount or a child.`

func (s *Server) handleAILetter(w http.ResponseWriter, r *http.Request) {
	f := famFrom(r.Context())
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	text := strings.TrimSpace(in.Text)
	if len([]rune(text)) < 20 {
		writeErr(w, http.StatusBadRequest, "paste the letter's text first")
		return
	}
	st, _ := s.store.GetSettings(r.Context(), actorID(r.Context()))
	s.withAI(w, r, func(ctx context.Context, complete func(string, string, bool) (string, error)) (any, error) {
		out, err := complete(letterSystem, familyContext(f, st)+"\nThe letter:\n\"\"\"\n"+clip(text, 12000)+"\n\"\"\"", true)
		if err != nil {
			return nil, err
		}
		var lr letterRead
		if err := parseObject(out, &lr); err != nil {
			return nil, errors.New("the answer could not be read; try again")
		}
		return cleanLetter(f, lr, st), nil
	})
}

// cleanLetter keeps only what fits: known children, real dates, known kinds.
func cleanLetter(f *store.Family, lr letterRead, st store.Settings) map[string]any {
	var people []int64
	for _, id := range lr.People {
		if p := personByID(f, id); p != nil {
			people = append(people, id)
		}
	}
	if !oneOf(lr.Kind, letterKinds) {
		lr.Kind = "info"
	}
	if !validDate(lr.DueOn) {
		lr.DueOn = ""
	}
	events := []aiEvent{}
	for _, e := range lr.Events {
		if e.Title == "" || e.Day == "" || !validDate(e.Day) {
			continue
		}
		if !validTime(e.StartTime) {
			e.StartTime = ""
		}
		if !validTime(e.EndTime) {
			e.EndTime = ""
		}
		events = append(events, e)
	}
	cur := cleanCurrency(lr.Currency)
	if cur == "" && lr.Amount > 0 {
		cur = st.Currency
	}
	return map[string]any{"title": clip(firstNonEmpty(lr.Title, "Letter"), 80), "sender": clip(lr.Sender, 80), "summary": clip(lr.Summary, 600), "kind": lr.Kind,
		"action": clip(lr.Action, 200), "due_on": lr.DueOn, "amount": int64(lr.Amount*100 + 0.5), "currency": cur, "people": nonNil(people), "events": events}
}

const quickSystem = `You turn one sentence from a parent into one thing for the family's organiser. Answer with ONE JSON object, nothing else:
{"kind": "run" | "event" | "chore" | "item" | "post", "summary": what you understood, in a few plain words, and
 for "run" (someone picks up, drops off or drives a child): "run": {"kind": "pickup"|"dropoff"|"lift", "day": YYYY-MM-DD, "time": "HH:MM", "children": [ids], "driver_id": id or 0, "place": "", "title": "", "weekly": true if it repeats every week, "weekdays": [1-7, Monday=1] when weekly}
 for "event" (an appointment, a party, a school thing): "event": {"title", "day": YYYY-MM-DD, "start_time", "end_time", "place", "people": [ids], "kind": "school"|"activity"|"appointment"|"family"|"other", "bring": ""}
 for "chore" (a job at home): "chore": {"title", "emoji", "repeat": "once"|"daily"|"weekly"|"monthly", "weekdays": [1-7], "people": [ids], "assign": "one"|"turns"|"anyone", "stars": 0-5}
 for "item" (things to buy): "items": ["2 milk", "bread"]
 for "post" (a note for everyone): "post": {"body"}}
Use the family's person ids. Work out dates from today. Leave out what was not said; never invent people.`

func (s *Server) handleAIQuick(w http.ResponseWriter, r *http.Request) {
	f := famFrom(r.Context())
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, http.StatusBadRequest, "say what to add")
		return
	}
	st, _ := s.store.GetSettings(r.Context(), actorID(r.Context()))
	s.withAI(w, r, func(ctx context.Context, complete func(string, string, bool) (string, error)) (any, error) {
		out, err := complete(quickSystem, familyContext(f, st)+"\nThe sentence: "+clip(strings.TrimSpace(in.Text), 1000), true)
		if err != nil {
			return nil, err
		}
		var v map[string]any
		if err := parseObject(out, &v); err != nil {
			return nil, errors.New("the answer could not be read; try again")
		}
		if !oneOf(str(v, "kind"), []string{"run", "event", "chore", "item", "post"}) {
			return nil, &userErr{"that did not sound like a pick-up, a plan, a chore, shopping or a note — try saying it differently"}
		}
		return v, nil
	})
}

func (s *Server) handleAIWeek(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	text := s.weekText(ctx, f)
	key := fmt.Sprintf("aiweek:%d:%d:%x", f.ID, actorID(ctx), hashString(text))
	if v, _, ok := s.store.CacheGet(ctx, key); ok && !queryBool(r, "fresh") {
		writeJSON(w, http.StatusOK, map[string]any{"text": v, "cached": true})
		return
	}
	s.withAI(w, r, func(ctx context.Context, complete func(string, string, bool) (string, error)) (any, error) {
		out, err := complete(`You help a busy family see their week at a glance. In at most 6 short lines of plain text (no markdown headings, no greetings):
say the busiest days, any pick-up or drop-off that nobody has taken yet ("nobody yet"), anyone due in two places at once, days without school, and what to remember to take along.
Address the person asking as "you". Be concrete with days and times; do not repeat everything.`, familyContext(f, st)+"\nThe coming week:\n"+text, false)
		if err != nil {
			return nil, err
		}
		out = strings.TrimSpace(out)
		_ = s.store.CachePut(context.WithoutCancel(ctx), key, out)
		return map[string]any{"text": out}, nil
	})
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
