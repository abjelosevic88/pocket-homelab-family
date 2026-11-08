package store

import (
	"context"
	"strings"
)

// ---- shopping lists ----

// ListLists lists a family's shopping lists with how much is still to get.
func (s *Store) ListLists(ctx context.Context, fid int64) ([]List, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id, l.family_id, l.name, l.emoji, l.position, COALESCE(l.shopper_id, 0), l.shopper_since, l.created_at,
		(SELECT COUNT(1) FROM items i WHERE i.list_id = l.id AND i.done = 0) FROM lists l WHERE l.family_id = ? ORDER BY l.position, l.id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []List{}
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.FamilyID, &l.Name, &l.Emoji, &l.Position, &l.ShopperID, &l.ShopperSince, &l.CreatedAt, &l.Open); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetList is one list.
func (s *Store) GetList(ctx context.Context, fid, id int64) (*List, error) {
	l := &List{}
	err := s.DB.QueryRowContext(ctx, `SELECT l.id, l.family_id, l.name, l.emoji, l.position, COALESCE(l.shopper_id, 0), l.shopper_since, l.created_at,
		(SELECT COUNT(1) FROM items i WHERE i.list_id = l.id AND i.done = 0) FROM lists l WHERE l.family_id = ? AND l.id = ?`, fid, id).
		Scan(&l.ID, &l.FamilyID, &l.Name, &l.Emoji, &l.Position, &l.ShopperID, &l.ShopperSince, &l.CreatedAt, &l.Open)
	return l, notFound(err)
}

// CreateList adds a list at the end.
func (s *Store) CreateList(ctx context.Context, l *List) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO lists (family_id, name, emoji, position) VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM lists WHERE family_id = ?))`,
		l.FamilyID, l.Name, l.Emoji, l.FamilyID)
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	return nil
}

// UpdateList saves a list's name, emoji, order and who is at the shop.
func (s *Store) UpdateList(ctx context.Context, l *List) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE lists SET name = ?, emoji = ?, position = ?, shopper_id = ?, shopper_since = ? WHERE family_id = ? AND id = ?`,
		l.Name, l.Emoji, l.Position, nullI(&l.ShopperID), l.ShopperSince, l.FamilyID, l.ID)
	return err
}

// DeleteList removes a list and its items.
func (s *Store) DeleteList(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM lists WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const itemCols = `id, family_id, list_id, name, qty, note, aisle, COALESCE(added_by, 0), done, COALESCE(done_by, 0), done_at, position, source, created_at`

func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	it := &Item{}
	var done int
	if err := row.Scan(&it.ID, &it.FamilyID, &it.ListID, &it.Name, &it.Qty, &it.Note, &it.Aisle, &it.AddedBy, &done, &it.DoneBy, &it.DoneAt, &it.Position, &it.Source, &it.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	it.Done = done == 1
	return it, nil
}

// ListItems lists a list's items: to get first (by aisle), then the ones got lately.
func (s *Store) ListItems(ctx context.Context, fid, listID int64) ([]Item, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items WHERE family_id = ? AND list_id = ? AND (done = 0 OR done_at >= ?)
		ORDER BY done, CASE WHEN done = 1 THEN done_at END DESC, aisle, position, id`, fid, listID, AddDays(Today(), -14))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

// OpenItems lists every item still to get in a family (for summaries).
func (s *Store) OpenItems(ctx context.Context, fid int64) ([]Item, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+itemCols+` FROM items WHERE family_id = ? AND done = 0 ORDER BY list_id, aisle, position, id`, fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

// GetItem is one item.
func (s *Store) GetItem(ctx context.Context, fid, id int64) (*Item, error) {
	return scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE family_id = ? AND id = ?`, fid, id))
}

// AddItem puts a thing on a list; the same thing still open is not added
// twice (the existing one comes back, its quantity updated when one is given).
func (s *Store) AddItem(ctx context.Context, it *Item) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM items WHERE list_id = ? AND done = 0 AND lower(name) = ? LIMIT 1`, it.ListID, strings.ToLower(it.Name)).Scan(&id)
	if err == nil {
		if it.Qty != "" {
			_, _ = s.DB.ExecContext(ctx, `UPDATE items SET qty = ? WHERE id = ?`, it.Qty, id)
		}
		cur, err := s.GetItem(ctx, it.FamilyID, id)
		if err != nil {
			return false, err
		}
		*it = *cur
		return false, nil
	}
	if it.Aisle == "" {
		it.Aisle = GuessAisle(it.Name)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO items (family_id, list_id, name, qty, note, aisle, added_by, position, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM items WHERE list_id = ?), ?)`,
		it.FamilyID, it.ListID, it.Name, it.Qty, it.Note, it.Aisle, nullI(&it.AddedBy), it.ListID, it.Source)
	if err != nil {
		return false, err
	}
	it.ID, _ = res.LastInsertId()
	it.CreatedAt = Now()
	return true, nil
}

// SaveItem writes an item.
func (s *Store) SaveItem(ctx context.Context, it *Item) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET list_id = ?, name = ?, qty = ?, note = ?, aisle = ?, done = ?, done_by = ?, done_at = ?, position = ? WHERE family_id = ? AND id = ?`,
		it.ListID, it.Name, it.Qty, it.Note, it.Aisle, boolInt(it.Done), nullI(&it.DoneBy), it.DoneAt, it.Position, it.FamilyID, it.ID)
	return err
}

// DeleteItem removes an item.
func (s *Store) DeleteItem(ctx context.Context, fid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE family_id = ? AND id = ?`, fid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearDone removes the items got on a list.
func (s *Store) ClearDone(ctx context.Context, fid, listID int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE family_id = ? AND list_id = ? AND done = 1`, fid, listID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecentNames are things the family put on lists before (for quick adding).
func (s *Store) RecentNames(ctx context.Context, fid int64, limit int) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name FROM items WHERE family_id = ? GROUP BY lower(name) ORDER BY COUNT(1) DESC, MAX(created_at) DESC LIMIT ?`, fid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// aisles groups common shop words so a list reads in the order of a shop.
var aisles = []struct {
	name  string
	words []string
}{
	{"Fruit & veg", []string{"apple", "banana", "orange", "lemon", "lime", "grape", "pear", "peach", "berry", "berries", "melon", "tomato", "potato", "onion", "garlic", "carrot", "pepper", "salad", "lettuce", "cucumber", "spinach", "broccoli", "zucchini", "courgette", "cabbage", "mushroom", "avocado", "herbs", "parsley", "fruit", "veg",
		"jabuk", "banan", "narandž", "naranc", "limun", "grožđ", "kruška", "breskv", "paradajz", "krompir", "luk", "bijeli luk", "mrkv", "paprik", "salat", "krastavac", "špinat", "kupus", "gljiv", "voće", "povrće"}},
	{"Bread & bakery", []string{"bread", "roll", "bun", "baguette", "croissant", "toast", "cake", "hljeb", "kruh", "pecivo", "kifl", "burek"}},
	{"Dairy & eggs", []string{"milk", "cheese", "yoghurt", "yogurt", "butter", "cream", "egg", "kefir", "mlijeko", "mleko", "sir", "jogurt", "maslac", "pavlak", "vrhnje", "jaja", "kajmak"}},
	{"Meat & fish", []string{"chicken", "beef", "pork", "mince", "minced", "sausage", "ham", "bacon", "fish", "salmon", "tuna", "turkey", "piletina", "piletin", "junetina", "mljeveno", "kobasic", "šunka", "riba", "losos", "tuna", "ćuretina", "meso", "sudžuk"}},
	{"Pantry", []string{"rice", "pasta", "spaghetti", "flour", "sugar", "salt", "oil", "olive", "vinegar", "beans", "lentil", "sauce", "passata", "ketchup", "mayo", "honey", "jam", "cereal", "oats", "coffee", "tea", "spice", "riža", "riz", "tjestenin", "brašno", "šećer", "so", "ulje", "sirće", "grah", "pasulj", "sos", "med", "džem", "kafa", "kava", "čaj", "zob"}},
	{"Frozen", []string{"frozen", "ice cream", "peas", "smrznut", "sladoled", "grašak"}},
	{"Drinks", []string{"water", "juice", "beer", "wine", "soda", "cola", "voda", "sok", "pivo", "vino", "mineralna"}},
	{"Snacks", []string{"chocolate", "crisps", "chips", "biscuit", "cookie", "nuts", "sweets", "čokolad", "čips", "keks", "smoki", "orasi", "bomboni"}},
	{"Household", []string{"detergent", "soap", "washing", "dish", "sponge", "toilet paper", "paper towel", "bin bag", "bags", "foil", "bleach", "deterdžent", "sapun", "spužv", "toaletni", "ubrus", "kese", "folija", "varikina"}},
	{"Baby & care", []string{"nappies", "diapers", "wipes", "shampoo", "toothpaste", "toothbrush", "deodorant", "pelene", "maramice", "šampon", "pasta za zube", "četkica", "dezodorans"}},
}

// GuessAisle names the part of a shop a thing is usually in ("" = other).
func GuessAisle(name string) string {
	n := " " + strings.ToLower(name) + " "
	for _, a := range aisles {
		for _, w := range a.words {
			if strings.Contains(n, " "+w) {
				return a.name
			}
		}
	}
	return ""
}
