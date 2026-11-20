package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Family as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "family:read", Name: "See your family", Description: "Read your family's day and week: who takes which pick-up, plans, chores, the shopping lists, letters from school, the board and the family info (only the families you are in) — for your journal, your briefing, search and Pocket AI."},
	{Scope: "family:write", Name: "Add to your family", Description: "Put things on the shopping list, add a plan, a pick-up, a chore, a letter or a note on the board, take a pick-up or tick off a chore for you (a reminder from Pocket Flows, a sentence you said to Pocket Voice or Pocket AI). It may change or remove only what that app added."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "family.json", Kind: core.KindPocket, Title: "Your family: its people, pick-ups and plans, chores, events, shopping lists, school terms, letters, the board and the family info"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", MaxBytes: 32 << 20, Title: "Adds the family from another box (or makes it, if you are in none yet); what is already there is left alone"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(families|family|today|week|summary|search|runs|plans|chores|stars|events|lists|items/recent|school|letters|posts|info|activity|day/\d{4}-\d{2}-\d{2})$`), "family:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(events/\d+|letters/\d+|lists/\d+/items)$`), "family:read"},
	{"POST", regexp.MustCompile(`^/api/v1/(items|lists/\d+/items|events|runs|chores|posts|letters)$`), "family:write"},
	{"POST", regexp.MustCompile(`^/api/v1/(runs/\d+/(take|ask|done|undo|skip|assign)|chores/\d+/(done|undo))$`), "family:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/(events|runs|chores|posts|letters|items)/\d+$`), "family:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "family" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
