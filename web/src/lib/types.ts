export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'calendar' | 'tasks' | 'contacts' | 'kitchen' | 'money' | 'split' | 'notes' | 'weather' | 'league' | 'travel' | 'pets' | 'home' | 'renewals' | 'chat'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  family_id: number; show_stars: boolean; show_calendar: boolean; currency: string
  notify_runs: boolean; notify_shop: boolean; notify_posts: boolean; notify_daily: boolean; notify_hour: number; quiet_from: number; quiet_to: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type Role = 'grownup' | 'child' | 'helper'

export interface Person {
  id: number; family_id: number; user_id: number; email: string; name: string; role: Role; manager: boolean; color: string; emoji: string
  birthday: string; school: string; class_name: string; phone: string; notes: string; contact_ref: string; position: number; archived: boolean
  created_at: string; updated_at: string; waiting: boolean
}

export interface Family { id: number; name: string; emoji: string; color: string; created_by: number; created_at: string; updated_at: string; people: Person[]; me?: Person }

export interface Made { id: number; family_id: number; owner: string; owner_id: number; target: MakeTarget; app: string; ref: string; title: string; url: string; created_at: string }

export type RunKind = 'pickup' | 'dropoff' | 'lift'
export type RunState = 'planned' | 'asking' | 'done' | 'skipped'

export interface Run {
  id: number; family_id: number; plan_id: number; day: string; time: string; kind: RunKind; title: string; place: string; children: number[]
  driver_id: number; state: RunState; asked_by: number; asked_note: string; done_at: string; done_by: number; notes: string; touched: boolean
  source: string; created_at: string; updated_at: string; clash?: string; made?: Made[]
}

export interface RunPlan {
  id: number; family_id: number; kind: RunKind; title: string; place: string; weekdays: number[]; time: string; children: number[]; driver_id: number
  rota: Record<string, number>; term_only: boolean; starts_on: string; ends_on: string; notes: string; active: boolean; created_at: string; updated_at: string
}

export interface Done { id: number; chore_id: number; person_id: number; day: string; stars: number; created_at: string; title?: string; emoji?: string }

export interface Chore {
  id: number; family_id: number; title: string; emoji: string; notes: string; repeat: 'once' | 'daily' | 'weekly' | 'monthly' | 'every'; every_days: number
  weekdays: number[]; assign: 'one' | 'turns' | 'anyone'; people: number[]; turn: number; stars: number; part_of_day: '' | 'morning' | 'afternoon' | 'evening'
  next_due: string; active: boolean; source: string; created_at: string; updated_at: string; whose: number; last_done?: Done; done_on?: string[]
}

export type EventKind = 'school' | 'activity' | 'appointment' | 'family' | 'birthday' | 'other'

export interface FamEvent {
  id: number; family_id: number; title: string; day: string; end_day: string; start_time: string; end_time: string; place: string; people: number[]
  kind: EventKind; repeat: 'none' | 'weekly' | 'yearly'; weekdays: number[]; until: string; bring: string; notes: string; letter_id: number; source: string
  created_at: string; updated_at: string; made?: Made[]
}

export interface List { id: number; family_id: number; name: string; emoji: string; position: number; shopper_id: number; shopper_since: string; created_at: string; open: number }
export interface Item { id: number; family_id: number; list_id: number; name: string; qty: string; note: string; aisle: string; added_by: number; done: boolean; done_by: number; done_at: string; position: number; source: string; created_at: string }

export interface Term { id: number; family_id: number; name: string; kind: 'term' | 'holiday' | 'closed'; starts_on: string; ends_on: string; created_at: string }
export interface SchoolDay { id?: number; person_id: number; weekday: number; starts: string; ends: string; bring: string }

export type LetterKind = 'info' | 'permission' | 'payment' | 'bring' | 'event'
export interface Letter {
  id: number; family_id: number; title: string; sender: string; body: string; summary: string; received_on: string; people: number[]; kind: LetterKind
  action: string; due_on: string; amount: number; currency: string; state: 'todo' | 'done' | 'none'; done_by: number; done_at: string; source: string
  created_at: string; updated_at: string; events?: FamEvent[]; made?: Made[]
}

export interface Post { id: number; family_id: number; person_id: number; body: string; pinned: boolean; until: string; source: string; created_at: string; updated_at: string; seen_by: number[]; seen: boolean }

export type InfoSection = 'emergency' | 'health' | 'school' | 'home' | 'sitter' | 'other'
export interface Info { id: number; family_id: number; person_id: number; section: InfoSection; label: string; value: string; phone: string; contact_ref: string; on_sheet: boolean; position: number; created_at: string; updated_at: string }

export interface Activity { id: number; family_id: number; person_id: number; kind: string; text: string; ref_kind: string; ref_id: number; created_at: string }

export interface AgendaItem {
  source: string; kind: string; id?: number; day: string; end_day?: string; time?: string; end_time?: string; title: string; sub?: string; place?: string
  people?: number[]; bring?: string; url?: string; overdue?: boolean
}
export interface BringItem { day: string; person_id?: number; what: string; for?: string }
export interface SourceState { slug: string; name: string; state: 'on' | 'off' | 'error'; detail?: string; fix?: string; count: number }
export interface OutItem { app: string; kind: string; day: string; title: string; sub?: string; url?: string; overdue?: boolean }

export interface WeatherDay { t_max?: number; t_min?: number; precip_prob?: number; precip?: number; condition?: string; text?: string; code?: number }
export interface Weather { place: string; text: string; today: WeatherDay; tomorrow: WeatherDay; url: string; warnings?: { title: string; level: string }[] }

export interface Today {
  date: string; tomorrow: string; runs: Run[]; needs: Run[]; agenda: AgendaItem[]; chores: Chore[]; bring_today: BringItem[]; bring_tomorrow: BringItem[]
  no_school: boolean; no_school_tomorrow: boolean; letters: Letter[]; lists: List[]; posts: Post[]; unseen: number; weather?: Weather
  renewals: OutItem[]; sources: SourceState[]; activity: Activity[]
}

export interface WeekDay { day: string; no_school: boolean; runs: Run[]; agenda: AgendaItem[]; chores: Chore[]; bring: BringItem[] }

export type MakeTarget = 'task' | 'event' | 'note' | 'payment' | 'expense' | 'contact' | 'chat'
export interface MakeOption { app: AppSlug; name: string; state: string; on: boolean; detail: string; fix: string }
export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: string; detail: string; fix: string; on: string[]; open_url: string }
