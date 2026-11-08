package store

import (
	"encoding/json"
	"strconv"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Family is a household shared by its people.
type Family struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Emoji     string   `json:"emoji"`
	Color     string   `json:"color"`
	CreatedBy int64    `json:"created_by"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	People    []Person `json:"people"`
	Me        *Person  `json:"me,omitempty"` // the caller in this family
}

// Person roles.
const (
	RoleGrownup = "grownup"
	RoleChild   = "child"
	RoleHelper  = "helper" // a grandparent, a sitter: helps, has no say
)

// Person is someone in a family, with or without an account here.
type Person struct {
	ID         int64  `json:"id"`
	FamilyID   int64  `json:"family_id"`
	UserID     int64  `json:"user_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Manager    bool   `json:"manager"`
	Color      string `json:"color"`
	Emoji      string `json:"emoji"`
	Birthday   string `json:"birthday"`
	School     string `json:"school"`
	ClassName  string `json:"class_name"`
	Phone      string `json:"phone"`
	Notes      string `json:"notes"`
	ContactRef string `json:"contact_ref"`
	Position   int    `json:"position"`
	Archived   bool   `json:"archived"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	// Waiting: added by email, not signed in here yet.
	Waiting bool `json:"waiting"`
}

// CanManage is whether this person may change the family and its people.
func (p *Person) CanManage() bool { return p != nil && p.Manager }

// CanEdit is whether this person may plan things (grown-ups and helpers;
// children tick their own chores, add to lists and post).
func (p *Person) CanEdit() bool { return p != nil && p.Role != RoleChild }

// RunPlan is a pick-up, drop-off or lift that repeats every week.
type RunPlan struct {
	ID        int64            `json:"id"`
	FamilyID  int64            `json:"family_id"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Place     string           `json:"place"`
	Weekdays  []int            `json:"weekdays"`
	Time      string           `json:"time"`
	Children  []int64          `json:"children"`
	DriverID  int64            `json:"driver_id"`
	Rota      map[string]int64 `json:"rota"` // weekday ("1".."7") → person
	TermOnly  bool             `json:"term_only"`
	StartsOn  string           `json:"starts_on"`
	EndsOn    string           `json:"ends_on"`
	Notes     string           `json:"notes"`
	Active    bool             `json:"active"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

// DriverOn is who takes the plan on an ISO weekday.
func (p *RunPlan) DriverOn(weekday int) int64 {
	if id, ok := p.Rota[itoa(int64(weekday))]; ok && id > 0 {
		return id
	}
	return p.DriverID
}

// Run states.
const (
	RunPlanned = "planned"
	RunAsking  = "asking" // the driver asked for someone else to take it
	RunDone    = "done"
	RunSkipped = "skipped"
)

// Run is one pick-up, drop-off or lift on one day.
type Run struct {
	ID        int64   `json:"id"`
	FamilyID  int64   `json:"family_id"`
	PlanID    int64   `json:"plan_id"`
	Day       string  `json:"day"`
	Time      string  `json:"time"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	Place     string  `json:"place"`
	Children  []int64 `json:"children"`
	DriverID  int64   `json:"driver_id"`
	State     string  `json:"state"`
	AskedBy   int64   `json:"asked_by"`
	AskedNote string  `json:"asked_note"`
	DoneAt    string  `json:"done_at"`
	DoneBy    int64   `json:"done_by"`
	Notes     string  `json:"notes"`
	Touched   bool    `json:"touched"`
	Source    string  `json:"source"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	// Worked out when read.
	Clash string `json:"clash,omitempty"` // the driver is somewhere else then
	Made  []Made `json:"made,omitempty"`
}

// Chore is a job at home.
type Chore struct {
	ID        int64   `json:"id"`
	FamilyID  int64   `json:"family_id"`
	Title     string  `json:"title"`
	Emoji     string  `json:"emoji"`
	Notes     string  `json:"notes"`
	Repeat    string  `json:"repeat"` // once | daily | weekly | monthly | every
	EveryDays int     `json:"every_days"`
	Weekdays  []int   `json:"weekdays"`
	Assign    string  `json:"assign"` // one | turns | anyone
	People    []int64 `json:"people"`
	Turn      int     `json:"turn"`
	Stars     int     `json:"stars"`
	PartOfDay string  `json:"part_of_day"`
	NextDue   string  `json:"next_due"`
	Active    bool    `json:"active"`
	Source    string  `json:"source"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	// Worked out when read.
	Whose    int64    `json:"whose"` // whose turn now (0 = anyone)
	LastDone *Done    `json:"last_done,omitempty"`
	DoneOn   []string `json:"done_on,omitempty"` // days done this week
}

// Done is one time a chore was done.
type Done struct {
	ID        int64  `json:"id"`
	ChoreID   int64  `json:"chore_id"`
	PersonID  int64  `json:"person_id"`
	Day       string `json:"day"`
	Stars     int    `json:"stars"`
	CreatedAt string `json:"created_at"`
	Title     string `json:"title,omitempty"`
	Emoji     string `json:"emoji,omitempty"`
}

// Event is one of the family's own plans.
type Event struct {
	ID        int64   `json:"id"`
	FamilyID  int64   `json:"family_id"`
	Title     string  `json:"title"`
	Day       string  `json:"day"`
	EndDay    string  `json:"end_day"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
	Place     string  `json:"place"`
	People    []int64 `json:"people"`
	Kind      string  `json:"kind"`
	Repeat    string  `json:"repeat"`
	Weekdays  []int   `json:"weekdays"`
	Until     string  `json:"until"`
	Bring     string  `json:"bring"`
	Notes     string  `json:"notes"`
	LetterID  int64   `json:"letter_id"`
	Source    string  `json:"source"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Made      []Made  `json:"made,omitempty"`
}

// List is a shared shopping list.
type List struct {
	ID           int64  `json:"id"`
	FamilyID     int64  `json:"family_id"`
	Name         string `json:"name"`
	Emoji        string `json:"emoji"`
	Position     int    `json:"position"`
	ShopperID    int64  `json:"shopper_id"`
	ShopperSince string `json:"shopper_since"`
	CreatedAt    string `json:"created_at"`
	Open         int    `json:"open"` // items still to get
}

// Item is one thing on a list.
type Item struct {
	ID        int64  `json:"id"`
	FamilyID  int64  `json:"family_id"`
	ListID    int64  `json:"list_id"`
	Name      string `json:"name"`
	Qty       string `json:"qty"`
	Note      string `json:"note"`
	Aisle     string `json:"aisle"`
	AddedBy   int64  `json:"added_by"`
	Done      bool   `json:"done"`
	DoneBy    int64  `json:"done_by"`
	DoneAt    string `json:"done_at"`
	Position  int    `json:"position"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// Term is a school term, a holiday or a closed day.
type Term struct {
	ID        int64  `json:"id"`
	FamilyID  int64  `json:"family_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // term | holiday | closed
	StartsOn  string `json:"starts_on"`
	EndsOn    string `json:"ends_on"`
	CreatedAt string `json:"created_at"`
}

// SchoolDay is a child's hours and things to take on one weekday.
type SchoolDay struct {
	ID       int64  `json:"id"`
	PersonID int64  `json:"person_id"`
	Weekday  int    `json:"weekday"`
	Starts   string `json:"starts"`
	Ends     string `json:"ends"`
	Bring    string `json:"bring"`
}

// Letter is a letter or message from school or a club.
type Letter struct {
	ID         int64   `json:"id"`
	FamilyID   int64   `json:"family_id"`
	Title      string  `json:"title"`
	Sender     string  `json:"sender"`
	Body       string  `json:"body"`
	Summary    string  `json:"summary"`
	ReceivedOn string  `json:"received_on"`
	People     []int64 `json:"people"`
	Kind       string  `json:"kind"`
	Action     string  `json:"action"`
	DueOn      string  `json:"due_on"`
	Amount     int64   `json:"amount"`
	Currency   string  `json:"currency"`
	State      string  `json:"state"` // todo | done | none
	DoneBy     int64   `json:"done_by"`
	DoneAt     string  `json:"done_at"`
	Source     string  `json:"source"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	Events     []Event `json:"events,omitempty"`
	Made       []Made  `json:"made,omitempty"`
}

// Post is a note on the family board.
type Post struct {
	ID        int64   `json:"id"`
	FamilyID  int64   `json:"family_id"`
	PersonID  int64   `json:"person_id"`
	Body      string  `json:"body"`
	Pinned    bool    `json:"pinned"`
	Until     string  `json:"until"`
	Source    string  `json:"source"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	SeenBy    []int64 `json:"seen_by"`
	Seen      bool    `json:"seen"` // the caller has seen it
}

// Info is one thing everyone should know.
type Info struct {
	ID         int64  `json:"id"`
	FamilyID   int64  `json:"family_id"`
	PersonID   int64  `json:"person_id"`
	Section    string `json:"section"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Phone      string `json:"phone"`
	ContactRef string `json:"contact_ref"`
	OnSheet    bool   `json:"on_sheet"`
	Position   int    `json:"position"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Activity is one line of who did what.
type Activity struct {
	ID        int64  `json:"id"`
	FamilyID  int64  `json:"family_id"`
	PersonID  int64  `json:"person_id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	RefKind   string `json:"ref_kind"`
	RefID     int64  `json:"ref_id"`
	CreatedAt string `json:"created_at"`
}

// Made is something a person made from a family thing in their other apps.
type Made struct {
	ID        int64  `json:"id"`
	FamilyID  int64  `json:"family_id"`
	Owner     string `json:"owner"`
	OwnerID   int64  `json:"owner_id"`
	Target    string `json:"target"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// The family shown (a person can be in more than one).
	FamilyID int64 `json:"family_id"`

	// Today and the week.
	ShowStars    bool   `json:"show_stars"`    // stars for children's chores
	ShowCalendar bool   `json:"show_calendar"` // my own Pocket Calendar events on Today and the week
	Currency     string `json:"currency"`      // for letters that ask for money

	// Notifications to my own address (Pocket Alerts, ntfy, Apprise).
	NotifyRuns   bool   `json:"notify_runs"`  // pick-ups given to me, asked of me, taken for me
	NotifyShop   bool   `json:"notify_shop"`  // someone is at the shop
	NotifyPosts  bool   `json:"notify_posts"` // new notes on the board
	NotifyDaily  bool   `json:"notify_daily"` // the evening before: tomorrow for me
	NotifyHour   int    `json:"notify_hour"`
	QuietFrom    int    `json:"quiet_from"` // hour; quiet_from == quiet_to = no quiet hours
	QuietTo      int    `json:"quiet_to"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
