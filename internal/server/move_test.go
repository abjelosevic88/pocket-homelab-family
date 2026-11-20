package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-family/internal/config"
	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// moveLinks starts Family behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "family:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"family","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) send(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) moveImport(tok string, data []byte) map[string]any {
	a.t.Helper()
	code, out := a.send(tok, "POST", "/api/v1/import", "application/json", data)
	if code != 200 {
		a.t.Fatalf("import → %d %s", code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	a.ok(a.bob, "POST", "/api/v1/families", map[string]any{"name": "Bob's"})
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/today", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/families", 403},
		{imp, "POST", "/api/v1/items", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := a.send(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.send(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 1 || len(p.Imports) != 1 || p.Exports[0].Kind != core.KindPocket || p.Imports[0].ContentType != "application/json" {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's family to Bob, who is in none yet: the family is made
// once, everything is added once, and Alice's family stays as it was.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	_, child := makeFamily(t, a)
	a.ok(a.alice, "POST", "/api/v1/chores", map[string]any{"title": "Dishes", "repeat": "daily", "assign": "one", "people": []int64{child}})
	a.ok(a.alice, "POST", "/api/v1/events", map[string]any{"title": "Dentist", "day": store.Today(), "people": []int64{child}})
	a.ok(a.alice, "POST", "/api/v1/items", map[string]any{"name": "milk"})

	// Bob is in no family: his export is empty, not an error
	code, bob := a.send("pcx_bob_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || strings.Contains(string(bob), "Dentist") || strings.Contains(string(bob), "Alice") {
		t.Fatalf("bob's export: %d %s", code, bob)
	}
	code, export := a.send("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "Dentist") {
		t.Fatalf("alice's export: %d %s", code, export)
	}

	first := a.moveImport("pcx_bob_import", export)
	if num(first, "families") != 1 || num(first, "chores") != 1 || num(first, "events") != 1 || num(first, "items") != 1 {
		t.Fatalf("first import: %v", first)
	}
	second := a.moveImport("pcx_bob_import", export)
	for k := range second {
		if num(second, k) != 0 {
			t.Fatalf("second import added %s: %v", k, second)
		}
	}
	if fams := a.ok(a.bob, "GET", "/api/v1/families", nil)["families"].([]any); len(fams) != 1 || fams[0].(map[string]any)["name"] != "Alice's family" {
		t.Fatalf("bob's families: %v", fams)
	}
	if fams := a.ok(a.alice, "GET", "/api/v1/families", nil)["families"].([]any); len(fams) != 1 {
		t.Fatalf("alice's families: %v", fams)
	}
	if ev := a.list(a.bob, "/api/v1/events"); len(ev) != 1 {
		t.Fatalf("bob's events: %v", ev)
	}
	if ev := a.list(a.alice, "/api/v1/events"); len(ev) != 1 {
		t.Fatalf("alice's events changed: %v", ev)
	}
	// Bob's export is now his own family, a copy, not Alice's
	_, bob = a.send("pcx_bob_export", "GET", "/api/v1/export.json", "", nil)
	if !strings.Contains(string(bob), "Dentist") {
		t.Fatalf("bob's export after the move: %s", bob)
	}
}
