// The dialogs that add and change things: a pick-up, a weekly plan, a chore,
// a plan on the calendar, a letter from school, a line of family info, a
// person, a school holiday — and "say it in your words" (Pocket AI).
import React, { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { todayISO } from '../lib/format'
import { EVENT_KINDS, INFO_SECTIONS, PALETTE, WEEKDAYS, announceChange, canEdit, dayWords, useFamily } from '../lib/family'
import { MakeMenu } from './MakeMenu'
import type { Chore, FamEvent, Info, Letter, Person, Run, RunPlan, Term } from '../lib/types'
import { Icon } from './icons'
import { PeoplePicker, PersonSelect, children, drivers } from './People'
import { Select } from './Select'
import { Confirm, Field, Modal, Segmented, Switch, errMsg, useToast } from './ui'

type Saved<T> = (x: T) => void

function useSave() {
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const run = async <T,>(fn: () => Promise<T>, ok: string, done: (x: T) => void) => {
    setBusy(true)
    try { const x = await fn(); toast(ok); announceChange(); done(x) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return { busy, run }
}

function Foot({ onClose, busy, label, onDelete }: { onClose: () => void; busy: boolean; label: string; onDelete?: () => void }) {
  return (
    <>
      {onDelete && <button type="button" className="btn ghost danger-text" onClick={onDelete}>Remove</button>}
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="submit" form="dlg-form" className="btn primary" disabled={busy}>{label}</button>
    </>
  )
}

export function WeekdayPicker({ value, onChange }: { value: number[]; onChange: (v: number[]) => void }) {
  const toggle = (d: number) => onChange(value.includes(d) ? value.filter(x => x !== d) : [...value, d].sort())
  return (
    <div className="wdpick" role="group" aria-label="Days of the week">
      {WEEKDAYS.map((w, i) => <button key={w} type="button" className={`wdpick-btn ${value.includes(i + 1) ? 'on' : ''}`} aria-pressed={value.includes(i + 1)} onClick={() => toggle(i + 1)}>{w}</button>)}
      <button type="button" className="wdpick-quick" onClick={() => onChange([1, 2, 3, 4, 5])}>Mon–Fri</button>
    </div>
  )
}

const KIND_OPTS = [{ value: 'pickup' as const, label: 'Pick-up' }, { value: 'dropoff' as const, label: 'Drop-off' }, { value: 'lift' as const, label: 'Lift' }]

// ---------- a single pick-up ----------

export function RunModal({ run, day, onClose, onSaved }: { run?: Partial<Run>; day?: string; onClose: () => void; onSaved?: Saved<Run> }) {
  const [f, setF] = useState<Partial<Run>>({ kind: 'pickup', day: day || todayISO(), time: '', children: [], driver_id: 0, place: '', title: '', notes: '', ...run })
  const [del, setDel] = useState(false)
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const body = { day: f.day, time: f.time, kind: f.kind, title: f.title, place: f.place, children: f.children, driver_id: f.driver_id, notes: f.notes }
    save(() => f.id ? api.put<Run>(`${V1}/runs/${f.id}`, body) : api.post<Run>(`${V1}/runs`, body), f.id ? 'Saved' : 'Added', x => { onSaved?.(x); onClose() })
  }
  return (
    <Modal title={f.id ? 'Change this run' : 'A pick-up, drop-off or lift'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Segmented value={f.kind!} onChange={v => setF({ ...f, kind: v })} options={KIND_OPTS} />
        <Field label="For"><PeoplePicker value={f.children || []} onChange={v => setF({ ...f, children: v })} filter={children} label="Children" /></Field>
        <div className="two-col tight">
          <Field label="Day"><input type="date" value={f.day} onChange={e => setF({ ...f, day: e.target.value })} required /></Field>
          <Field label="Time"><input type="time" value={f.time} onChange={e => setF({ ...f, time: e.target.value })} /></Field>
        </div>
        <Field label="Where"><input value={f.place} onChange={e => setF({ ...f, place: e.target.value })} placeholder="School, football, a friend's house" /></Field>
        <Field label="Who takes it" help="Nobody yet: the others are asked who can.">
          <PersonSelect value={f.driver_id || 0} onChange={v => setF({ ...f, driver_id: v })} filter={drivers} />
        </Field>
        {!(f.children || []).length && <Field label="What it is"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Grandma from the station" /></Field>}
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
      </form>
      {del && <Confirm title="Remove this run?" message={f.plan_id ? 'It is part of a weekly plan: only this day is called off.' : 'It goes for everyone.'} confirmLabel="Remove"
        onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/runs/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- a weekly plan ----------

export function PlanModal({ plan, onClose }: { plan?: Partial<RunPlan>; onClose: () => void }) {
  const { people } = useFamily()
  const [f, setF] = useState<Partial<RunPlan>>({ kind: 'pickup', weekdays: [1, 2, 3, 4, 5], time: '', children: [], driver_id: 0, rota: {}, term_only: true, place: '', title: '', notes: '', active: true, ...plan })
  const [del, setDel] = useState(false)
  const [rota, setRota] = useState(Object.keys(plan?.rota || {}).length > 0)
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const body = { ...f, rota: rota ? f.rota : {} }
    save(() => f.id ? api.put(`${V1}/plans/${f.id}`, body) : api.post(`${V1}/plans`, body), f.id ? 'Saved' : 'Every week from now on', () => onClose())
  }
  const grown = people.filter(drivers)
  return (
    <Modal title={f.id ? 'Change the weekly plan' : 'Every week'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Segmented value={f.kind!} onChange={v => setF({ ...f, kind: v })} options={KIND_OPTS} />
        <Field label="For"><PeoplePicker value={f.children || []} onChange={v => setF({ ...f, children: v })} filter={children} label="Children" /></Field>
        <Field label="On"><WeekdayPicker value={f.weekdays || []} onChange={v => setF({ ...f, weekdays: v })} /></Field>
        <div className="two-col tight">
          <Field label="At"><input type="time" value={f.time} onChange={e => setF({ ...f, time: e.target.value })} required /></Field>
          <Field label="Where"><input value={f.place} onChange={e => setF({ ...f, place: e.target.value })} placeholder="School" /></Field>
        </div>
        <Field label="Who takes it"><PersonSelect value={f.driver_id || 0} onChange={v => setF({ ...f, driver_id: v })} filter={drivers} /></Field>
        <label className="check-row"><Switch checked={rota} onChange={setRota} label="Different people on different days" /><span>Different people on different days</span></label>
        {rota && (
          <div className="rota">
            {(f.weekdays || []).map(d => (
              <div key={d} className="rota-row"><span className="rota-day">{WEEKDAYS[d - 1]}</span>
                <Select ariaLabel={WEEKDAYS[d - 1]} value={String(f.rota?.[String(d)] || 0)} onChange={v => setF({ ...f, rota: { ...f.rota, [String(d)]: Number(v) } })}
                  options={[{ value: '0', label: 'As above' }, ...grown.map(p => ({ value: String(p.id), label: p.name }))]} />
              </div>
            ))}
          </div>
        )}
        <label className="check-row"><Switch checked={!!f.term_only} onChange={v => setF({ ...f, term_only: v })} label="Not in school holidays" /><span>Not in school holidays <span className="muted small">(School → holidays)</span></span></label>
        <div className="two-col tight">
          <Field label="From (optional)"><input type="date" value={f.starts_on} onChange={e => setF({ ...f, starts_on: e.target.value })} /></Field>
          <Field label="Until (optional)"><input type="date" value={f.ends_on} onChange={e => setF({ ...f, ends_on: e.target.value })} /></Field>
        </div>
        {f.id ? <label className="check-row"><Switch checked={!!f.active} onChange={v => setF({ ...f, active: v })} label="Active" /><span>Active</span></label> : null}
        <p className="muted small">Each day becomes its own run: you can change one day (someone else takes Tuesday) without touching the plan.</p>
      </form>
      {del && <Confirm title="Stop this weekly plan?" message="Runs already done or changed by hand stay; the rest go." confirmLabel="Stop it"
        onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/plans/${f.id}`), 'Stopped', () => onClose())} />}
    </Modal>
  )
}

// ---------- a chore ----------

const EMOJI = ['🍽️', '🧹', '🗑️', '🧺', '🛏️', '🧸', '🪴', '🐈', '🐕', '📚', '🚗', '🛒', '🧽', '🍳']

export function ChoreModal({ chore, onClose }: { chore?: Partial<Chore>; onClose: () => void }) {
  const [f, setF] = useState<Partial<Chore>>({ title: '', emoji: '', notes: '', repeat: 'weekly', every_days: 3, weekdays: [], assign: 'one', people: [], stars: 0, part_of_day: '', ...chore })
  const [del, setDel] = useState(false)
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => f.id ? api.put(`${V1}/chores/${f.id}`, f) : api.post(`${V1}/chores`, f), f.id ? 'Saved' : 'Chore added', () => onClose())
  }
  return (
    <Modal title={f.id ? 'Change the chore' : 'A chore'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Field label="What"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Dishes after dinner" required autoFocus /></Field>
        <div className="emoji-row">{EMOJI.map(em => <button key={em} type="button" className={`emoji-btn ${f.emoji === em ? 'on' : ''}`} onClick={() => setF({ ...f, emoji: f.emoji === em ? '' : em })}>{em}</button>)}</div>
        <Field label="How often">
          <Segmented size="sm" value={f.repeat!} onChange={v => setF({ ...f, repeat: v })} options={[{ value: 'daily', label: 'Every day' }, { value: 'weekly', label: 'Weekly' }, { value: 'every', label: 'Every few days' }, { value: 'monthly', label: 'Monthly' }, { value: 'once', label: 'Once' }]} />
        </Field>
        {f.repeat === 'weekly' && <Field label="On"><WeekdayPicker value={f.weekdays || []} onChange={v => setF({ ...f, weekdays: v })} /></Field>}
        {f.repeat === 'every' && <Field label="Every how many days"><input type="number" min={1} max={365} value={f.every_days} onChange={e => setF({ ...f, every_days: Number(e.target.value) })} /></Field>}
        <Field label="Who">
          <Segmented size="sm" value={f.assign!} onChange={v => setF({ ...f, assign: v })} options={[{ value: 'one', label: 'Each of them' }, { value: 'turns', label: 'In turns' }, { value: 'anyone', label: 'Anyone' }]} />
        </Field>
        {f.assign !== 'anyone' && <PeoplePicker value={f.people || []} onChange={v => setF({ ...f, people: v })} />}
        <div className="two-col tight">
          <Field label="Part of the day"><Select ariaLabel="Part of the day" value={f.part_of_day || ''} onChange={v => setF({ ...f, part_of_day: v as Chore['part_of_day'] })}
            options={[{ value: '', label: 'Any time' }, { value: 'morning', label: 'Morning' }, { value: 'afternoon', label: 'Afternoon' }, { value: 'evening', label: 'Evening' }]} /></Field>
          <Field label="Stars" help="For the children's star chart."><input type="number" min={0} max={20} value={f.stars} onChange={e => setF({ ...f, stars: Number(e.target.value) })} /></Field>
        </div>
        {f.id ? <Field label="Next due"><input type="date" value={f.next_due} onChange={e => setF({ ...f, next_due: e.target.value })} /></Field> : null}
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="How it's done, where things are" /></Field>
      </form>
      {del && <Confirm title={`Remove “${f.title}”?`} message="Its history (who did it when) goes too." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/chores/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- a plan on the calendar ----------

export function EventModal({ event, day, onClose }: { event?: Partial<FamEvent>; day?: string; onClose: () => void }) {
  const [f, setF] = useState<Partial<FamEvent>>({ title: '', day: day || todayISO(), end_day: '', start_time: '', end_time: '', place: '', people: [], kind: 'family', repeat: 'none', weekdays: [], until: '', bring: '', notes: '', ...event })
  const [del, setDel] = useState(false)
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => f.id ? api.put(`${V1}/events/${f.id}`, f) : api.post(`${V1}/events`, f), f.id ? 'Saved' : 'Added to the family week', () => onClose())
  }
  return (
    <Modal title={f.id ? 'Change the plan' : 'A plan for the family'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Field label="What"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder="Mila at the dentist" required autoFocus /></Field>
        <Field label="Kind"><Segmented size="sm" value={f.kind!} onChange={v => setF({ ...f, kind: v as FamEvent['kind'] })} options={EVENT_KINDS.map(k => ({ value: k.value, label: k.label }))} /></Field>
        <div className="two-col tight">
          <Field label="Day"><input type="date" value={f.day} onChange={e => setF({ ...f, day: e.target.value })} required /></Field>
          <Field label="Until (several days)"><input type="date" value={f.end_day} min={f.day} onChange={e => setF({ ...f, end_day: e.target.value })} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="From"><input type="time" value={f.start_time} onChange={e => setF({ ...f, start_time: e.target.value })} /></Field>
          <Field label="To"><input type="time" value={f.end_time} onChange={e => setF({ ...f, end_time: e.target.value })} /></Field>
        </div>
        <Field label="Where"><input value={f.place} onChange={e => setF({ ...f, place: e.target.value })} /></Field>
        <Field label="Who"><PeoplePicker value={f.people || []} onChange={v => setF({ ...f, people: v })} /></Field>
        <Field label="Repeats"><Segmented size="sm" value={f.repeat!} onChange={v => setF({ ...f, repeat: v as FamEvent['repeat'] })} options={[{ value: 'none', label: 'No' }, { value: 'weekly', label: 'Every week' }, { value: 'yearly', label: 'Every year' }]} /></Field>
        {f.repeat === 'weekly' && <>
          <Field label="On"><WeekdayPicker value={f.weekdays || []} onChange={v => setF({ ...f, weekdays: v })} /></Field>
          <Field label="Until (optional)"><input type="date" value={f.until} onChange={e => setF({ ...f, until: e.target.value })} /></Field>
        </>}
        <Field label="Take along" help="Shows on the day under “Take along”."><input value={f.bring} onChange={e => setF({ ...f, bring: e.target.value })} placeholder="Football boots, water" /></Field>
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
      </form>
      {del && <Confirm title={`Remove “${f.title}”?`} message={f.repeat !== 'none' ? 'Every time it repeats goes.' : 'It goes for everyone.'} confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/events/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- a letter from school (with Pocket AI reading it) ----------

interface ReadLetter { title: string; sender: string; summary: string; kind: Letter['kind']; action: string; due_on: string; amount: number; currency: string; people: number[]; events: { title: string; day: string; start_time: string; end_time: string; place: string; bring: string }[] }

export function LetterModal({ letter, onClose, onSaved }: { letter?: Partial<Letter>; onClose: () => void; onSaved?: Saved<Letter> }) {
  const { toast } = useToast()
  const { settings } = useFamilySettings()
  const [f, setF] = useState<Partial<Letter>>({ title: '', sender: '', body: '', summary: '', kind: 'info', action: '', due_on: '', amount: 0, currency: settings?.currency || 'EUR', people: [], received_on: todayISO(), ...letter })
  const [events, setEvents] = useState<ReadLetter['events']>([])
  const [reading, setReading] = useState(false)
  const [amount, setAmount] = useState(letter?.amount ? (letter.amount / 100).toFixed(2) : '')
  const [del, setDel] = useState(false)
  const [ai, setAI] = useState<{ ready: boolean; detail: string } | null>(null)
  useEffect(() => { api.get<{ ready: boolean; detail: string }>(`${V1}/ai/status`).then(setAI).catch(() => setAI({ ready: false, detail: '' })) }, [])
  const { busy, run: save } = useSave()
  const read = async () => {
    setReading(true)
    try {
      const r = await api.post<ReadLetter>(`${V1}/ai/letter`, { text: f.body })
      setF({ ...f, title: r.title, sender: r.sender, summary: r.summary, kind: r.kind, action: r.action, due_on: r.due_on, people: r.people, currency: r.currency || f.currency, amount: r.amount })
      setAmount(r.amount ? (r.amount / 100).toFixed(2) : '')
      setEvents(r.events)
      toast('Read — check it before adding')
    } catch (e) { toast(errMsg(e), 'err') } finally { setReading(false) }
  }
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const body = { ...f, amount: Math.round(parseFloat((amount || '0').replace(',', '.')) * 100) || 0, events: f.id ? undefined : events, state: f.id ? f.state : undefined }
    save(() => f.id ? api.put<Letter>(`${V1}/letters/${f.id}`, body) : api.post<Letter>(`${V1}/letters`, body), f.id ? 'Saved' : 'Letter added', x => { onSaved?.(x); onClose() })
  }
  return (
    <Modal size="lg" title={f.id ? 'Change the letter' : 'A letter from school or a club'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        {!f.id && <>
          <Field label="The letter" help="Paste the text of the letter, the email or the message.">
            <textarea rows={6} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} placeholder="Dear parents, on Friday class 2a goes to…" autoFocus />
          </Field>
          <div className="row wrap">
            <button type="button" className="btn" onClick={read} disabled={reading || !ai?.ready || (f.body || '').trim().length < 20} title={ai?.ready ? 'Pocket AI reads it: dates, what to do, what to pay and take' : ai?.detail || 'Needs AI (Settings → AI)'}>
              <Icon name="sparkles" size={15} />{reading ? 'Reading…' : 'Read it for me'}
            </button>
            {ai && !ai.ready && <span className="muted small">Reading needs AI (Pocket AI through Pocket Core, or your own key in Settings → AI). Fill it in below.</span>}
          </div>
        </>}
        <div className="two-col tight">
          <Field label="Title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} required placeholder="School trip to Jahorina" /></Field>
          <Field label="From"><input value={f.sender} onChange={e => setF({ ...f, sender: e.target.value })} placeholder="Ms. Mehić, class 2a" /></Field>
        </div>
        <Field label="For"><PeoplePicker value={f.people || []} onChange={v => setF({ ...f, people: v })} filter={children} /></Field>
        <Field label="It asks for">
          <Segmented size="sm" value={f.kind!} onChange={v => setF({ ...f, kind: v as Letter['kind'] })} options={[{ value: 'info', label: 'Nothing' }, { value: 'permission', label: 'A signature' }, { value: 'payment', label: 'Money' }, { value: 'bring', label: 'Something to take' }, { value: 'event', label: 'A date' }]} />
        </Field>
        <Field label="What to do"><input value={f.action} onChange={e => setF({ ...f, action: e.target.value })} placeholder="Sign the slip and pay 25 €" /></Field>
        <div className="two-col tight">
          <Field label="By"><input type="date" value={f.due_on} onChange={e => setF({ ...f, due_on: e.target.value })} /></Field>
          <Field label={`Amount (${f.currency || 'EUR'})`}><input inputMode="decimal" value={amount} onChange={e => setAmount(e.target.value)} placeholder="0.00" /></Field>
        </div>
        {f.summary && <Field label="In short"><textarea rows={2} value={f.summary} onChange={e => setF({ ...f, summary: e.target.value })} /></Field>}
        {events.length > 0 && (
          <div className="ai-events">
            <h4 className="sub-h">Dates in the letter — they go on the family week</h4>
            {events.map((ev, i) => (
              <div key={i} className="ai-event">
                <Icon name="calendar" size={15} />
                <div className="grow"><b>{ev.title}</b> · {dayWords(ev.day)}{ev.start_time && ` ${ev.start_time}`}{ev.place && ` · ${ev.place}`}{ev.bring && <div className="muted small">Take along: {ev.bring}</div>}</div>
                <button type="button" className="btn ghost icon-only sm" aria-label="Leave out" onClick={() => setEvents(events.filter((_, j) => j !== i))}><Icon name="x" size={14} /></button>
              </div>
            ))}
          </div>
        )}
        {f.id ? <Field label="The letter"><textarea rows={5} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} /></Field> : null}
      </form>
      {del && <Confirm title="Remove this letter?" message="Dates made from it stay on the week." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/letters/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

function useFamilySettings() {
  const [settings, setSettings] = useState<{ currency: string } | null>(null)
  useEffect(() => { api.get<{ currency: string }>(`${V1}/settings`).then(setSettings).catch(() => {}) }, [])
  return { settings }
}

// ---------- a line of family info ----------

export function InfoModal({ info, onClose }: { info?: Partial<Info>; onClose: () => void }) {
  const [f, setF] = useState<Partial<Info>>({ section: 'emergency', label: '', value: '', phone: '', person_id: 0, on_sheet: true, contact_ref: '', ...info })
  const [del, setDel] = useState(false)
  const [contacts, setContacts] = useState<{ id: string; name: string; phone: string; title: string; org: string }[] | null>(null)
  const [contactsErr, setContactsErr] = useState('')
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => f.id ? api.put(`${V1}/info/${f.id}`, f) : api.post(`${V1}/info`, f), 'Saved', () => onClose())
  }
  const fromContacts = async () => {
    try { setContacts(await api.get(`${V1}/pick/contacts`)) } catch (e) { setContactsErr(errMsg(e)) }
  }
  return (
    <Modal title={f.id ? 'Change' : 'Something everyone should know'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label="Save" onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Field label="Section"><Select ariaLabel="Section" value={f.section!} onChange={v => setF({ ...f, section: v as Info['section'] })} options={INFO_SECTIONS.map(s => ({ value: s.value, label: s.label }))} /></Field>
        <div className="two-col tight">
          <Field label="What"><input value={f.label} onChange={e => setF({ ...f, label: e.target.value })} placeholder="Family doctor" required autoFocus /></Field>
          <Field label="About"><PersonSelect value={f.person_id || 0} onChange={v => setF({ ...f, person_id: v })} nobody="The whole family" ariaLabel="About" /></Field>
        </div>
        <Field label="Details"><textarea rows={2} value={f.value} onChange={e => setF({ ...f, value: e.target.value })} placeholder="Dr. Hodžić, Dom zdravlja Centar" /></Field>
        <Field label="Phone"><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} /></Field>
        <div className="row wrap">
          <button type="button" className="btn sm" onClick={fromContacts}><Icon name="users" size={14} />From my contacts</button>
          {contactsErr && <span className="muted small">{contactsErr}</span>}
        </div>
        {contacts && <Select ariaLabel="Contact" placeholder="Pick a contact…" value={f.contact_ref || ''} onChange={v => {
          const c = contacts.find(x => x.id === v)
          if (c) setF({ ...f, contact_ref: c.id, label: f.label || c.title || c.name, value: f.value || [c.name, c.org].filter(Boolean).join(', '), phone: f.phone || c.phone })
        }} options={contacts.map(c => ({ value: c.id, label: c.name, hint: c.phone }))} />}
        <label className="check-row"><Switch checked={!!f.on_sheet} onChange={v => setF({ ...f, on_sheet: v })} label="On the sitter sheet" /><span>On the sitter sheet</span></label>
      </form>
      {del && <Confirm title={`Remove “${f.label}”?`} message="It goes for everyone." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/info/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- a person ----------

export function PersonModal({ person, onClose }: { person?: Partial<Person>; onClose: () => void }) {
  const { me, people } = useFamily()
  const [f, setF] = useState<Partial<Person>>({ name: '', role: 'child', email: '', color: PALETTE[people.length % PALETTE.length], emoji: '', birthday: '', school: '', class_name: '', phone: '', notes: '', manager: false, ...person })
  const [del, setDel] = useState(false)
  const { busy, run: save } = useSave()
  const self = f.id === me?.id
  const manage = !!me?.manager
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => f.id ? api.put(`${V1}/people/${f.id}`, f) : api.post(`${V1}/people`, f), f.id ? 'Saved' : `${f.name} is in the family`, () => onClose())
  }
  return (
    <Modal title={f.id ? f.name : 'Someone in the family'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={f.id ? 'Save' : 'Add'} onDelete={f.id && manage && !self ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required autoFocus /></Field>
        {manage && <Field label="Is"><Segmented value={f.role!} onChange={v => setF({ ...f, role: v })} options={[{ value: 'grownup', label: 'A grown-up' }, { value: 'child', label: 'A child' }, { value: 'helper', label: 'Helps out' }]} /></Field>}
        {manage && <Field label="Email (optional)" help={f.role === 'child' ? 'Older children can sign in too: they tick their chores, add to the list and see the week.' : 'They see the family when they sign in to Pocket Family with this email.'}>
          <input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} placeholder="ana@example.com" disabled={!!f.user_id && !f.waiting} />
        </Field>}
        <Field label="Colour and emoji">
          <div className="row wrap">
            {PALETTE.map(c => <button key={c} type="button" className={`swatch ${f.color === c ? 'on' : ''}`} style={{ background: c }} aria-label={c} onClick={() => setF({ ...f, color: c })} />)}
            <input className="emoji-input" value={f.emoji} onChange={e => setF({ ...f, emoji: [...e.target.value].slice(-2).join('') })} placeholder="🙂" aria-label="Emoji" />
          </div>
        </Field>
        <div className="two-col tight">
          <Field label="Birthday"><input type="date" value={f.birthday?.length === 10 ? f.birthday : ''} onChange={e => setF({ ...f, birthday: e.target.value })} /></Field>
          <Field label="Phone"><input type="tel" value={f.phone} onChange={e => setF({ ...f, phone: e.target.value })} /></Field>
        </div>
        {f.role === 'child' && manage && <div className="two-col tight">
          <Field label="School"><input value={f.school} onChange={e => setF({ ...f, school: e.target.value })} /></Field>
          <Field label="Class"><input value={f.class_name} onChange={e => setF({ ...f, class_name: e.target.value })} /></Field>
        </div>}
        {manage && <Field label="Good to know" help="Allergies, what helps, the shoe size — shows on the family page."><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>}
        {f.id && canEdit(me) && <div className="row wrap"><MakeMenu owner="person" ownerID={f.id} targets={['contact']} label="Pocket Contacts" />{f.contact_ref && <span className="muted small">In your contacts</span>}</div>}
        {manage && f.role === 'grownup' && <label className="check-row"><Switch checked={!!f.manager} onChange={v => setF({ ...f, manager: v })} label="Looks after the family settings" /><span>Looks after the family settings <span className="muted small">(adds and removes people)</span></span></label>}
      </form>
      {del && <Confirm title={`Take ${f.name} off the family?`} message="Their pick-ups and chores stay, without a name." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/people/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- a term, a holiday, a closed day ----------

export function TermModal({ term, onClose }: { term?: Partial<Term>; onClose: () => void }) {
  const [f, setF] = useState<Partial<Term>>({ name: '', kind: 'holiday', starts_on: todayISO(), ends_on: '', ...term })
  const [del, setDel] = useState(false)
  const { busy, run: save } = useSave()
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => f.id ? api.put(`${V1}/terms/${f.id}`, f) : api.post(`${V1}/terms`, f), 'Saved', () => onClose())
  }
  return (
    <Modal size="sm" title={f.id ? 'Change' : 'Holidays and terms'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label="Save" onDelete={f.id ? () => setDel(true) : undefined} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <Segmented value={f.kind!} onChange={v => setF({ ...f, kind: v as Term['kind'] })} options={[{ value: 'holiday', label: 'Holiday' }, { value: 'closed', label: 'Closed day' }, { value: 'term', label: 'Term' }]} />
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} required placeholder="Winter holiday" autoFocus /></Field>
        <div className="two-col tight">
          <Field label="From"><input type="date" value={f.starts_on} onChange={e => setF({ ...f, starts_on: e.target.value })} required /></Field>
          <Field label="To"><input type="date" value={f.ends_on} min={f.starts_on} onChange={e => setF({ ...f, ends_on: e.target.value })} /></Field>
        </div>
        <p className="muted small">No school on holidays and closed days: weekly school runs marked “not in school holidays” skip them.</p>
      </form>
      {del && <Confirm title={`Remove “${f.name}”?`} message="School runs on those days come back." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={() => save(() => api.del(`${V1}/terms/${f.id}`), 'Removed', () => onClose())} />}
    </Modal>
  )
}

// ---------- say it in your words ----------

interface Quick {
  kind: 'run' | 'event' | 'chore' | 'item' | 'post'; summary?: string
  run?: Partial<Run> & { weekly?: boolean; weekdays?: number[] }; event?: Partial<FamEvent>; chore?: Partial<Chore>; items?: string[]; post?: { body: string }
}

export function QuickAdd({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<React.ReactNode>(null)
  const go = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const q = await api.post<Quick>(`${V1}/ai/quick`, { text })
      switch (q.kind) {
        case 'run':
          if (q.run?.weekly) setOpen(<PlanModal plan={{ kind: q.run.kind, time: q.run.time, children: q.run.children, driver_id: q.run.driver_id, place: q.run.place, title: q.run.title, weekdays: q.run.weekdays?.length ? q.run.weekdays : [1, 2, 3, 4, 5] }} onClose={onClose} />)
          else setOpen(<RunModal run={{ ...q.run, id: undefined }} onClose={onClose} />)
          break
        case 'event': setOpen(<EventModal event={{ ...q.event, id: undefined }} onClose={onClose} />); break
        case 'chore': setOpen(<ChoreModal chore={{ ...q.chore, id: undefined }} onClose={onClose} />); break
        case 'item': {
          const r = await api.post<{ added: number; list: { name: string } }>(`${V1}/items`, { names: q.items || [] })
          toast(`${r.added || 'Nothing new'} on ${r.list.name}`); announceChange(); onClose(); break
        }
        case 'post': setOpen(<PostModal body={q.post?.body || text} onClose={onClose} />); break
      }
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  if (open) return <>{open}</>
  return (
    <Modal size="sm" title="Say it in your words" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="submit" form="dlg-form" className="btn primary" disabled={busy || !text.trim()}><Icon name="sparkles" size={15} />{busy ? 'Thinking…' : 'Next'}</button></>}>
      <form id="dlg-form" onSubmit={go} className="stack">
        <textarea rows={3} value={text} onChange={e => setText(e.target.value)} autoFocus placeholder="Ana picks Mila up from football on Thursday at 6 · Dentist for Luka next Tuesday 10:30 · We need milk, bread and eggs" />
        <p className="muted small">Pocket AI turns it into a pick-up, a plan, a chore, shopping or a note. You check it before anything is added.</p>
      </form>
    </Modal>
  )
}

export function PostModal({ post, body, onClose }: { post?: { id: number; body: string; pinned: boolean; until: string }; body?: string; onClose: () => void }) {
  const [f, setF] = useState({ body: post?.body || body || '', pinned: post?.pinned || false, until: post?.until || '', chat: '' })
  const [chats, setChats] = useState<{ value: string; label: string }[] | null>(null)
  const { busy, run: save } = useSave()
  useEffect(() => { if (!post) api.get<{ id: string; name: string }[]>(`${V1}/make/chats`).then(l => setChats(l.map(c => ({ value: c.id, label: c.name })))).catch(() => setChats(null)) }, [post])
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save(() => post ? api.put(`${V1}/posts/${post.id}`, f) : api.post(`${V1}/posts`, f), post ? 'Saved' : 'On the board', () => onClose())
  }
  return (
    <Modal size="sm" title={post ? 'Change the note' : 'A note for everyone'} onClose={onClose} footer={<Foot onClose={onClose} busy={busy} label={post ? 'Save' : 'Put it up'} />}>
      <form id="dlg-form" onSubmit={submit} className="stack">
        <textarea rows={4} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} required autoFocus placeholder="Grandma comes for lunch on Sunday" />
        <label className="check-row"><Switch checked={f.pinned} onChange={v => setF({ ...f, pinned: v })} label="Pin to the top" /><span>Pin to the top</span></label>
        <Field label="Take it down after (optional)"><input type="date" value={f.until} onChange={e => setF({ ...f, until: e.target.value })} /></Field>
        {chats && chats.length > 0 && <Field label="Also tell (Pocket Chat)"><Select ariaLabel="Chat" value={f.chat} onChange={v => setF({ ...f, chat: v })} options={[{ value: '', label: 'No' }, ...chats]} /></Field>}
      </form>
    </Modal>
  )
}

