// School: letters (what they ask for, read by Pocket AI), each child's week
// with what to take each day, and the school year's holidays.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { fmtClock, fmtDate, fmtRange, todayISO } from '../lib/format'
import { WEEKDAYS, announceChange, canEdit, dayWords, money, useFamily, useOnChange } from '../lib/family'
import type { Letter, SchoolDay, Term } from '../lib/types'
import { Panel } from '../components/Bits'
import { LetterModal, TermModal } from '../components/Forms'
import { useCrumb } from '../components/Layout'
import { MadeLinks, MakeMenu } from '../components/MakeMenu'
import { Face, Faces } from '../components/People'
import { Icon } from '../components/icons'
import { Empty, Loading, Modal, Segmented, errMsg, useToast } from '../components/ui'

interface SchoolData { terms: Term[]; days: SchoolDay[]; now: { no_school: boolean; term?: Term; next_break?: Term } }

const LETTER_ICON: Record<string, string> = { payment: 'wallet', permission: 'pen', bring: 'package', event: 'calendar', info: 'mail' }

export default function School() {
  const { family, me, people, name } = useFamily()
  const navigate = useNavigate()
  const [letters, setLetters] = useState<Letter[] | null>(null)
  const [school, setSchool] = useState<SchoolData | null>(null)
  const [show, setShow] = useState<'todo' | 'all'>('todo')
  const [term, setTerm] = useState<Partial<Term> | null>(null)
  const [week, setWeek] = useState<number | null>(null)
  const [add, setAdd] = useState(false)
  const load = useCallback(() => {
    api.get<Letter[]>(`${V1}/letters`).then(setLetters).catch(() => setLetters([]))
    api.get<SchoolData>(`${V1}/school`).then(setSchool).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  if (!letters || !school || !me) return <Loading />
  const grown = canEdit(me)
  const kids = people.filter(p => p.role === 'child' && !p.archived)
  const shown = letters.filter(l => show === 'all' || l.state === 'todo')
  const today = todayISO()
  return (
    <div className="school">
      <header className="page-head">
        <div><h2>School</h2><p className="muted">{school.now.no_school ? 'No school today. ' : ''}{school.now.next_break ? `Next break: ${school.now.next_break.name}, ${dayWords(school.now.next_break.starts_on)}.` : ''}</p></div>
        {grown && <button type="button" className="btn primary" onClick={() => setAdd(true)}><Icon name="mail" size={15} />A letter</button>}
      </header>
      <div className="two-panels">
        <div className="col">
          <Panel title="Letters" icon="mail" actions={<Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'todo', label: 'To do' }, { value: 'all', label: 'All' }]} />}>
            {shown.length === 0 ? <Empty icon="mail" title={show === 'todo' ? 'Nothing to do' : 'No letters yet'}>{grown ? 'Paste a letter, an email or a message from school: Pocket AI picks out the dates, what to sign, pay and take along.' : ''}</Empty> : shown.map(l => (
              <button key={l.id} type="button" className={`letter-row ${l.state !== 'todo' ? 'is-done' : ''}`} onClick={() => navigate(`/school/letters/${l.id}`)}>
                <span className={`letter-ico k-${l.kind}`}><Icon name={LETTER_ICON[l.kind] || 'mail'} size={16} /></span>
                <span className="grow">
                  <span className="letter-title ellipsis">{l.title}</span>
                  <span className="muted small">{[l.action, l.due_on && (l.state === 'todo' ? `by ${dayWords(l.due_on)}` : ''), money(l.amount, l.currency), l.sender].filter(Boolean).join(' · ')}</span>
                </span>
                <Faces ids={l.people} size={20} />
                {l.state === 'todo' && l.due_on && l.due_on < today && <span className="badge danger">late</span>}
                {l.state === 'done' && <Icon name="check" size={15} className="ok" />}
              </button>
            ))}
          </Panel>
        </div>
        <div className="col">
          <Panel title="Each child's week" icon="school">
            {kids.length === 0 ? <p className="muted small pad">Add the children under Family first.</p> : kids.map(k => {
              const days = school.days.filter(d => d.person_id === k.id)
              return (
                <div key={k.id} className="kid-week">
                  <div className="kid-week-head"><Face person={k} size={24} /><b>{k.name.split(' ')[0]}</b><span className="muted small">{[k.school, k.class_name].filter(Boolean).join(' · ')}</span><span className="spacer" />{grown && <button type="button" className="btn sm ghost" onClick={() => setWeek(k.id)}>Change</button>}</div>
                  {days.length === 0 ? <p className="muted small">No school hours yet.</p> : (
                    <div className="kid-days">
                      {days.map(d => <div key={d.weekday} className="kid-day"><span className="kid-day-name">{WEEKDAYS[d.weekday - 1]}</span><span className="tnum small">{d.starts && fmtClock(d.starts)}{d.ends && '–' + fmtClock(d.ends)}</span>{d.bring && <span className="kid-bring small"><Icon name="package" size={12} />{d.bring}</span>}</div>)}
                    </div>
                  )}
                </div>
              )
            })}
          </Panel>
          <Panel title="Holidays and terms" icon="sun" actions={grown && <button type="button" className="btn sm ghost" onClick={() => setTerm({})}><Icon name="plus" size={14} />Add</button>}>
            {school.terms.length === 0 ? <p className="muted small pad">Add the school holidays: weekly school runs skip them by themselves.</p> : school.terms.filter(t => t.ends_on >= today || school.terms.length < 6).map(t => (
              <button key={t.id} type="button" className={`li-row ${t.ends_on < today ? 'is-off' : ''}`} onClick={() => grown && setTerm(t)} disabled={!grown}>
                <Icon name={t.kind === 'term' ? 'school' : 'sun'} size={15} />
                <span className="grow"><span className="ellipsis">{t.name}</span><span className="muted small">{t.starts_on === t.ends_on ? fmtDate(t.starts_on) : fmtRange(t.starts_on, t.ends_on)}</span></span>
                {today >= t.starts_on && today <= t.ends_on && <span className="badge accent">now</span>}
              </button>
            ))}
          </Panel>
        </div>
      </div>
      {add && <LetterModal onClose={() => { setAdd(false); load() }} onSaved={l => navigate(`/school/letters/${l.id}`)} />}
      {term && <TermModal term={term} onClose={() => { setTerm(null); load() }} />}
      {week && <WeekModal personID={week} days={school.days.filter(d => d.person_id === week)} name={name(week)} onClose={() => { setWeek(null); load() }} />}
    </div>
  )
}

function WeekModal({ personID, days, name, onClose }: { personID: number; days: SchoolDay[]; name: string; onClose: () => void }) {
  const { toast } = useToast()
  const [rows, setRows] = useState<SchoolDay[]>(() => [1, 2, 3, 4, 5, 6, 7].map(d => days.find(x => x.weekday === d) || { person_id: personID, weekday: d, starts: d <= 5 ? '' : '', ends: '', bring: '' }))
  const set = (i: number, patch: Partial<SchoolDay>) => setRows(rows.map((r, j) => j === i ? { ...r, ...patch } : r))
  const save = async () => {
    try { await api.put(`${V1}/school/week/${personID}`, rows); toast('Saved'); announceChange(); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  const sameAsMon = () => setRows(rows.map((r, i) => i > 0 && i < 5 ? { ...r, starts: rows[0].starts, ends: rows[0].ends } : r))
  return (
    <Modal size="lg" title={`${name}'s school week`} onClose={onClose} footer={<><button type="button" className="btn" onClick={sameAsMon}>Hours like Monday</button><span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="week-edit">
        <div className="week-edit-head muted small"><span /><span>From</span><span>To</span><span>Take along</span></div>
        {rows.slice(0, 5).concat(rows.slice(5).filter(r => r.starts || r.bring)).map((r) => {
          const i = rows.indexOf(r)
          return (
            <div key={r.weekday} className="week-edit-row">
              <b>{WEEKDAYS[r.weekday - 1]}</b>
              <input type="time" value={r.starts} onChange={e => set(i, { starts: e.target.value })} aria-label="From" />
              <input type="time" value={r.ends} onChange={e => set(i, { ends: e.target.value })} aria-label="To" />
              <input value={r.bring} onChange={e => set(i, { bring: e.target.value })} placeholder="PE kit, library book" aria-label="Take along" />
            </div>
          )
        })}
      </div>
    </Modal>
  )
}

export function LetterPage() {
  const { id } = useParams()
  const { me, name } = useFamily()
  const { toast } = useToast()
  const [l, setL] = useState<Letter | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const load = useCallback(() => { api.get<Letter>(`${V1}/letters/${id}`).then(setL).catch(e => setErr(errMsg(e))) }, [id])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  useCrumb(l?.title)
  if (err) return <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>
  if (!l || !me) return <Loading />
  const grown = canEdit(me)
  const setState = async (state: Letter['state']) => {
    try { setL(await api.put<Letter>(`${V1}/letters/${l.id}`, { state })); toast(state === 'done' ? 'Done — thank you' : 'Back on the list'); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="letter-page">
      <Link to="/school" className="back-link"><Icon name="chevron-left" size={15} />School</Link>
      <header className="page-head">
        <div>
          <h2>{l.title}</h2>
          <p className="muted">{[l.sender, l.received_on && `came ${(w => /^(Today|Yesterday)$/.test(w) ? w.toLowerCase() : w)(dayWords(l.received_on))}`, l.people.map(p => name(p)).join(', ')].filter(Boolean).join(' · ')}</p>
        </div>
        {grown && <div className="row wrap">
          {l.state === 'todo' ? <button type="button" className="btn primary" onClick={() => setState('done')}><Icon name="check" size={15} />Done</button> : <button type="button" className="btn" onClick={() => setState('todo')}>Not done yet</button>}
          <MakeMenu owner="letter" ownerID={l.id} targets={['task', 'event', 'payment', 'expense', 'note']} made={l.made} amount={l.amount} currency={l.currency} onMade={load} />
          <button type="button" className="btn ghost" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Change</button>
        </div>}
      </header>
      <div className="letter-facts">
        {l.action && <div className="fact"><Icon name={LETTER_ICON[l.kind]} size={16} /><span><b>{l.action}</b>{l.due_on && <span className="muted"> · by {dayWords(l.due_on, true)}</span>}</span></div>}
        {l.amount > 0 && <div className="fact"><Icon name="wallet" size={16} /><span>{money(l.amount, l.currency)}</span></div>}
        {l.state === 'done' && <div className="fact ok"><Icon name="check-circle" size={16} /><span>Done{l.done_by ? ` by ${name(l.done_by)}` : ''}</span></div>}
      </div>
      <MadeLinks made={l.made} />
      {l.summary && <p className="letter-summary">{l.summary}</p>}
      {l.events && l.events.length > 0 && (
        <Panel title="Dates from this letter" icon="calendar">
          {l.events.map(e => <div key={e.id} className="li-row"><Icon name="calendar" size={15} /><span className="grow"><span>{e.title}</span><span className="muted small">{dayWords(e.day, true)}{e.start_time && ' ' + fmtClock(e.start_time)}{e.place && ' · ' + e.place}{e.bring && ' · take along: ' + e.bring}</span></span><Faces ids={e.people} size={20} /></div>)}
        </Panel>
      )}
      {l.body && <details className="letter-body" open={!l.summary}><summary>The letter</summary><pre>{l.body}</pre></details>}
      {edit && <LetterModal letter={l} onClose={() => { setEdit(false); load() }} />}
    </div>
  )
}
