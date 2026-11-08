package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-family/internal/store"
)

// ---- shopping ----
//
// Shared lists everyone adds to (a child too). "I'm at the shop" tells the
// others, so they can add what they need while someone is there.

func (s *Server) handleLists(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListLists(r.Context(), famID(r.Context()))
	if err != nil {
		s.fail(w, err, "lists")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	l := &store.List{FamilyID: famID(r.Context()), Name: strings.TrimSpace(in.Name), Emoji: strings.TrimSpace(in.Emoji)}
	if l.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the list a name")
		return
	}
	if err := s.store.CreateList(r.Context(), l); err != nil {
		s.fail(w, err, "add list")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetList(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Emoji    *string `json:"emoji"`
		Position *int    `json:"position"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !canEdit(w, r) {
		return
	}
	trimP(&l.Name, in.Name)
	trimP(&l.Emoji, in.Emoji)
	if in.Position != nil {
		l.Position = *in.Position
	}
	if l.Name == "" {
		writeErr(w, http.StatusBadRequest, "give the list a name")
		return
	}
	if err := s.store.UpdateList(ctx, l); err != nil {
		s.fail(w, err, "save list")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	if !canEdit(w, r) {
		return
	}
	ctx := r.Context()
	lists, _ := s.store.ListLists(ctx, famID(ctx))
	if len(lists) <= 1 {
		writeErr(w, http.StatusBadRequest, "keep at least one list")
		return
	}
	if err := s.store.DeleteList(ctx, famID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "remove list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleShopping: POST /lists/{id}/shopping {"on": true} — I'm at the shop / done.
func (s *Server) handleShopping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetList(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	var in struct {
		On bool `json:"on"`
	}
	_ = decode(r, &in)
	if in.On {
		l.ShopperID, l.ShopperSince = f.Me.ID, store.Now()
	} else {
		l.ShopperID, l.ShopperSince = 0, ""
	}
	if err := s.store.UpdateList(ctx, l); err != nil {
		s.fail(w, err, "save list")
		return
	}
	if in.On {
		var others []int64
		for _, p := range accounts(f) {
			if p.ID != f.Me.ID {
				others = append(others, p.ID)
			}
		}
		s.act(ctx, "shop", "went shopping ("+l.Name+")", "list", l.ID)
		s.tell(ctx, f, others, "shop", nameOf(f, f.Me.ID)+" is at the shop", "Add what you need to "+l.Name+" now.", "/shopping/"+itoa(l.ID))
		s.publish(ctx, actorID(ctx), "family.shopping_trip", "list:"+itoa(l.ID), map[string]any{"family_id": f.ID, "list_id": l.ID, "list": l.Name, "by": nameOf(f, f.Me.ID), "open": l.Open})
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetList(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	items, err := s.store.ListItems(ctx, f.ID, l.ID)
	if err != nil {
		s.fail(w, err, "items")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": l, "items": items})
}

type itemIn struct {
	ListID *int64  `json:"list_id"`
	Name   *string `json:"name"`
	Qty    *string `json:"qty"`
	Note   *string `json:"note"`
	Aisle  *string `json:"aisle"`
	Done   *bool   `json:"done"`
}

// addItems adds names (one per line, "2 x milk" or "milk, 2 l" both work) to a list.
func (s *Server) addItems(r *http.Request, listID int64, names []string, qty, note string) ([]store.Item, int, error) {
	ctx := r.Context()
	f := famFrom(ctx)
	out := []store.Item{}
	added := 0
	for _, raw := range names {
		name, q := splitQty(raw)
		if name == "" {
			continue
		}
		if qty != "" && len(names) == 1 {
			q = qty
		}
		it := &store.Item{FamilyID: f.ID, ListID: listID, Name: clip(name, 200), Qty: clip(q, 40), Note: clip(note, 300), AddedBy: f.Me.ID, Source: linkApp(ctx)}
		isNew, err := s.store.AddItem(ctx, it)
		if err != nil {
			return nil, 0, err
		}
		if isNew {
			added++
		}
		out = append(out, *it)
	}
	return out, added, nil
}

// splitQty takes "2 milk", "milk x2", "milk, 2 l" apart.
func splitQty(raw string) (string, string) {
	s := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "-•*"))
	if s == "" {
		return "", ""
	}
	if i := strings.LastIndex(s, ","); i > 0 {
		q := strings.TrimSpace(s[i+1:])
		if q != "" && q[0] >= '0' && q[0] <= '9' && len(q) <= 12 {
			return strings.TrimSpace(s[:i]), q
		}
	}
	f := strings.Fields(s)
	if len(f) > 1 {
		first := strings.TrimSuffix(strings.ToLower(f[0]), "x")
		if first != "" && first[0] >= '0' && first[0] <= '9' && len(first) <= 5 {
			return strings.Join(f[1:], " "), first
		}
		last := strings.TrimPrefix(strings.ToLower(f[len(f)-1]), "x")
		if strings.HasPrefix(strings.ToLower(f[len(f)-1]), "x") && last != "" && last[0] >= '0' && last[0] <= '9' {
			return strings.Join(f[:len(f)-1], " "), last
		}
	}
	return s, ""
}

func (s *Server) handleAddItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	l, err := s.store.GetList(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "list")
		return
	}
	var in struct {
		Name  string   `json:"name"`
		Names []string `json:"names"`
		Qty   string   `json:"qty"`
		Note  string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	names := in.Names
	if strings.TrimSpace(in.Name) != "" {
		names = append(names, strings.Split(in.Name, "\n")...)
	}
	if len(names) == 0 {
		writeErr(w, http.StatusBadRequest, "say what to get")
		return
	}
	if len(names) > 200 {
		names = names[:200]
	}
	items, added, err := s.addItems(r, l.ID, names, strings.TrimSpace(in.Qty), strings.TrimSpace(in.Note))
	if err != nil {
		s.fail(w, err, "add")
		return
	}
	if added > 0 {
		what := items[0].Name
		if added > 1 {
			what = itoa(int64(added)) + " things"
		}
		s.act(ctx, "item", "put "+what+" on "+l.Name, "list", l.ID)
		// someone at the shop hears what was just added
		if l.ShopperID > 0 && l.ShopperID != f.Me.ID {
			s.tell(ctx, f, []int64{l.ShopperID}, "shop", nameOf(f, f.Me.ID)+" added to "+l.Name, clip(joinNames(items), 300), "/shopping/"+itoa(l.ID))
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": items, "added": added})
}

func joinNames(items []store.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, strings.TrimSpace(it.Qty+" "+it.Name))
	}
	return strings.Join(out, ", ")
}

func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	it, err := s.store.GetItem(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "item")
		return
	}
	var in itemIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	trimP(&it.Name, in.Name)
	trimP(&it.Qty, in.Qty)
	trimP(&it.Note, in.Note)
	trimP(&it.Aisle, in.Aisle)
	if in.ListID != nil && *in.ListID != it.ListID {
		if _, err := s.store.GetList(ctx, f.ID, *in.ListID); err != nil {
			s.fail(w, err, "list")
			return
		}
		it.ListID = *in.ListID
	}
	if in.Done != nil && *in.Done != it.Done {
		it.Done = *in.Done
		if it.Done {
			it.DoneBy, it.DoneAt = f.Me.ID, store.Now()
		} else {
			it.DoneBy, it.DoneAt = 0, ""
		}
	}
	if it.Name == "" {
		writeErr(w, http.StatusBadRequest, "say what to get")
		return
	}
	if err := s.store.SaveItem(ctx, it); err != nil {
		s.fail(w, err, "save item")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	it, err := s.store.GetItem(ctx, f.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "item")
		return
	}
	if app := linkApp(ctx); app != "" && it.Source != app {
		writeErr(w, http.StatusForbidden, "an app may only remove the things it put on a list itself")
		return
	}
	if err := s.store.DeleteItem(ctx, f.ID, it.ID); err != nil {
		s.fail(w, err, "remove item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearDone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.store.ClearDone(ctx, famID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "clear")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"removed": n})
}

func (s *Server) handleRecentItems(w http.ResponseWriter, r *http.Request) {
	names, err := s.store.RecentNames(r.Context(), famID(r.Context()), 40)
	if err != nil {
		s.fail(w, err, "recent")
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// handleQuickItem: POST /items {"name","list_id"?} — onto the first list
// when none is named (Pocket AI, Pocket Voice, another app).
func (s *Server) handleQuickItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := famFrom(ctx)
	var in struct {
		ListID int64    `json:"list_id"`
		List   string   `json:"list"`
		Name   string   `json:"name"`
		Names  []string `json:"names"`
		Qty    string   `json:"qty"`
		Note   string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	lists, err := s.store.ListLists(ctx, f.ID)
	if err != nil || len(lists) == 0 {
		writeErr(w, http.StatusBadRequest, "there is no shopping list yet")
		return
	}
	l := lists[0]
	for _, x := range lists {
		if x.ID == in.ListID || (in.List != "" && strings.EqualFold(x.Name, strings.TrimSpace(in.List))) {
			l = x
		}
	}
	names := in.Names
	if strings.TrimSpace(in.Name) != "" {
		names = append(names, strings.Split(in.Name, "\n")...)
	}
	if len(names) == 0 {
		writeErr(w, http.StatusBadRequest, "say what to get")
		return
	}
	items, added, err := s.addItems(r, l.ID, names, strings.TrimSpace(in.Qty), strings.TrimSpace(in.Note))
	if err != nil {
		s.fail(w, err, "add")
		return
	}
	if added > 0 {
		s.act(ctx, "item", "put "+joinNames(items)+" on "+l.Name, "list", l.ID)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"list": l, "items": items, "added": added})
}
