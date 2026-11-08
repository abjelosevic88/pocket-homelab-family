// Today: who takes which pick-up, the day's plans next to everyone's own
// apps, chores, what to take along tomorrow, letters, shopping, the board.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1, ApiError } from '../lib/api'
import { ago, fmtClock } from '../lib/format'
import { canEdit, dayWords, money, useFamily, useOnChange } from '../lib/family'
import type { AgendaItem, Today as TodayT } from '../lib/types'
import { AgendaRow, BringList, ChoreRow, Panel, RunRow, SourcesLine } from '../components/Bits'
import { EventModal } from '../components/Forms'
import { openCreate } from '../components/Layout'
import { Face } from '../components/People'
import { Icon } from '../components/icons'
import { Loading, errMsg, useToast } from '../components/ui'

function greeting() {
  const h = new Date().getHours()
  return h < 5 ? 'Good night' : h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening'
}

export default function Today() {
  const { me, family, name, person } = useFamily()
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const [data, setData] = useState<TodayT | null>(null)
  const [err, setErr] = useState('')
  const [open, setOpen] = useState<AgendaItem | null>(null)
  const load = useCallback((quick = false) => {
    api.get<TodayT>(`${V1}/today${quick ? '?quick=1' : ''}`).then(d => setData(prev => quick && prev ? { ...d, weather: prev.weather, sources: prev.sources, agenda: [...d.agenda, ...prev.agenda.filter(a => a.source !== 'family')].sort((a, b) => a.day.localeCompare(b.day) || (a.time || '').localeCompare(b.time || '')), renewals: prev.renewals } : d)).catch(e => setErr(errMsg(e)))
  }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(() => load(true))
  useEffect(() => { if (sp.get('day') === 'tomorrow') document.getElementById('tomorrow')?.scrollIntoView() }, [sp, data])
  if (err) return <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>
  if (!data || !me) return <Loading />
  const today = data.date
  const runsToday = data.runs.filter(r => r.day === today)
  const runsTomorrow = data.runs.filter(r => r.day === data.tomorrow)
  const agToday = data.agenda.filter(a => a.day === today || (a.day < today && (a.end_day || '') >= today))
  const agTomorrow = data.agenda.filter(a => a.day === data.tomorrow || (a.day < data.tomorrow && (a.end_day || '') >= data.tomorrow))
  const needs = data.needs.filter(r => r.day > today || !runsToday.find(x => x.id === r.id))
  const mineToday = runsToday.filter(r => r.driver_id === me.id && r.state !== 'done' && r.state !== 'skipped')
  const grown = canEdit(me)
  const choresFor = (pid: number) => data.chores.filter(c => c.assign === 'turns' ? c.whose === pid : c.assign === 'one' ? c.people.includes(pid) : false)
  const anyone = data.chores.filter(c => c.assign === 'anyone' || !c.people.length)
  const w = data.weather
  return (
    <div className="today">
      <header className="today-head">
        <div>
          <h1>{greeting()}, {me.name.split(' ')[0]}</h1>
          <p className="muted">{dayWords(today, true) === 'Today' ? new Date().toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' }) : ''}
            {data.no_school && ' · no school today'}
            {mineToday.length > 0 && <> · you have {mineToday.length === 1 ? `one ${mineToday[0].kind === 'dropoff' ? 'drop-off' : 'pick-up'} at ${fmtClock(mineToday[0].time)}` : `${mineToday.length} runs`}</>}
          </p>
        </div>
        {w && (
          <a className="wx" href={w.url || undefined} target="_blank" rel="noreferrer" title={`${w.place}: ${w.text}`}>
            <Icon name={wxIcon(w.today)} size={22} />
            <span className="wx-t tnum">{w.today.t_max !== undefined ? `${Math.round(w.today.t_max)}°` : ''}<span className="muted">{w.today.t_min !== undefined ? ` / ${Math.round(w.today.t_min)}°` : ''}</span></span>
            <span className="wx-text small muted ellipsis">{w.text}</span>
          </a>
        )}
      </header>

      {needs.length > 0 && grown && (
        <div className="needs">
          <div className="needs-head"><Icon name="alert" size={16} /><b>{needs.length === 1 ? 'A run needs someone' : `${needs.length} runs need someone`}</b><span className="muted small">this week</span></div>
          {needs.slice(0, 4).map(r => <div key={r.id} className="needs-row"><span className="needs-day">{dayWords(r.day)}</span><RunRow run={r} compact onChanged={() => load(true)} /></div>)}
          {needs.length > 4 && <Link to="/pickups" className="small">All of them</Link>}
        </div>
      )}

      <div className="today-grid">
        <div className="col">
          <Panel title="Who's picking up" icon="car" actions={grown && <button type="button" className="btn sm ghost" onClick={() => openCreate('run', { day: today })}><Icon name="plus" size={14} />Add</button>}>
            {runsToday.length ? runsToday.map(r => <RunRow key={r.id} run={r} onChanged={() => load(true)} />) : <p className="muted small pad">No pick-ups or drop-offs today.</p>}
          </Panel>

          <Panel title="Today" icon="calendar" actions={grown && <button type="button" className="btn sm ghost" onClick={() => openCreate('event', { day: today })}><Icon name="plus" size={14} />Plan</button>}>
            {agToday.length ? agToday.map((a, i) => <AgendaRow key={i} item={a} onOpen={setOpen} />) : <p className="muted small pad">Nothing planned.</p>}
            {data.bring_today.length > 0 && <div className="bring-wrap"><h4 className="sub-h">Take along today</h4><BringList items={data.bring_today} /></div>}
          </Panel>

          <Panel title="Tomorrow" icon="sunrise" className="tomorrow" actions={data.no_school_tomorrow ? <span className="badge">no school</span> : undefined}>
            <div id="tomorrow" />
            {runsTomorrow.map(r => <RunRow key={r.id} run={r} compact onChanged={() => load(true)} />)}
            {agTomorrow.map((a, i) => <AgendaRow key={i} item={a} onOpen={setOpen} />)}
            {data.bring_tomorrow.length > 0 && <div className="bring-wrap"><h4 className="sub-h">Pack tonight</h4><BringList items={data.bring_tomorrow} /></div>}
            {!runsTomorrow.length && !agTomorrow.length && !data.bring_tomorrow.length && <p className="muted small pad">Nothing yet.</p>}
          </Panel>

          <WeekInWords />
        </div>

        <div className="col">
          <Panel title="Chores today" icon="broom" count={data.chores.length} actions={<Link to="/chores" className="btn sm ghost">All</Link>}>
            {data.chores.length === 0 ? <p className="muted small pad">All done. 🎉</p> : (
              <div className="chores-by">
                {family!.people.filter(p => !p.archived && choresFor(p.id).length).map(p => (
                  <div key={p.id} className="chores-person">
                    <div className="chores-person-head"><Face person={p} size={22} /><b>{p.name.split(' ')[0]}</b></div>
                    {choresFor(p.id).map(c => <ChoreRow key={c.id + '-' + p.id} chore={c.assign === 'one' ? { ...c, people: [p.id] } : c} showWho={false} onChanged={() => load(true)} />)}
                  </div>
                ))}
                {anyone.length > 0 && <div className="chores-person"><div className="chores-person-head"><Icon name="users" size={16} /><b>Anyone</b></div>{anyone.map(c => <ChoreRow key={c.id} chore={c} showWho={false} onChanged={() => load(true)} />)}</div>}
              </div>
            )}
          </Panel>

          {data.letters.length > 0 && (
            <Panel title="Letters waiting" icon="mail" count={data.letters.length} actions={<Link to="/school" className="btn sm ghost">School</Link>}>
              {data.letters.map(l => (
                <button key={l.id} type="button" className="li-row" onClick={() => navigate(`/school/letters/${l.id}`)}>
                  <Icon name={l.kind === 'payment' ? 'wallet' : l.kind === 'permission' ? 'pen' : l.kind === 'bring' ? 'package' : 'mail'} size={15} />
                  <span className="grow"><span className="ellipsis">{l.action || l.title}</span><span className="muted small">{[l.people.map(id => name(id)).join(', '), l.due_on && `by ${dayWords(l.due_on)}`, money(l.amount, l.currency)].filter(Boolean).join(' · ')}</span></span>
                  {l.due_on && l.due_on <= data.tomorrow && <span className="badge warn">soon</span>}
                </button>
              ))}
            </Panel>
          )}

          <Panel title="Shopping" icon="cart" actions={<Link to="/shopping" className="btn sm ghost">Open</Link>}>
            {data.lists.map(l => (
              <Link key={l.id} to={`/shopping/${l.id}`} className="li-row">
                <span className="li-emoji">{l.emoji || '🛒'}</span>
                <span className="grow"><span className="ellipsis">{l.name}</span>{l.shopper_id > 0 && <span className="small accent-text">{person(l.shopper_id)?.name.split(' ')[0]} is at the shop</span>}</span>
                <span className="muted small tnum">{l.open ? `${l.open} to get` : 'nothing'}</span>
              </Link>
            ))}
          </Panel>

          <Panel title="The board" icon="pin" count={data.unseen} actions={<Link to="/board" className="btn sm ghost">Open</Link>}>
            {data.posts.length === 0 ? <p className="muted small pad">No notes. <button type="button" className="link-btn" onClick={() => openCreate('post')}>Put one up</button></p> : data.posts.map(p => (
              <div key={p.id} className={`post-mini ${p.seen ? '' : 'unseen'}`}>
                <Face person={person(p.person_id)} size={22} />
                <span className="grow"><span className="post-mini-body">{p.body}</span><span className="muted small">{name(p.person_id)} · {ago(p.created_at)}{p.pinned && ' · pinned'}</span></span>
              </div>
            ))}
          </Panel>

          {data.renewals.length > 0 && (
            <Panel title="Papers running out" icon="shield">
              {data.renewals.map((r, i) => (
                <a key={i} href={r.url || undefined} target="_blank" rel="noreferrer" className={`li-row ${r.overdue ? 'is-late' : ''}`}>
                  <Icon name="shield" size={15} /><span className="grow ellipsis">{r.title}</span><span className="muted small">{r.overdue ? 'ran out' : dayWords(r.day)}</span>
                </a>
              ))}
            </Panel>
          )}

          {data.activity.length > 0 && (
            <Panel title="Lately" icon="activity">
              <ul className="acts">
                {data.activity.slice(0, 8).map(a => <li key={a.id}><Face person={person(a.person_id)} size={18} /><span><b>{name(a.person_id) || 'Someone'}</b> {a.text}</span><span className="muted small">{ago(a.created_at)}</span></li>)}
              </ul>
            </Panel>
          )}
        </div>
      </div>
      <SourcesLine sources={data.sources} />
      {open && open.source === 'family' && open.id && <EventLoader id={open.id} onClose={() => { setOpen(null); load(true) }} />}
    </div>
  )
}

export function EventLoader({ id, onClose }: { id: number; onClose: () => void }) {
  const [ev, setEv] = useState<import('../lib/types').FamEvent | null>(null)
  const { toast } = useToast()
  useEffect(() => { api.get<import('../lib/types').FamEvent>(`${V1}/events/${id}`).then(setEv).catch(e => { toast(errMsg(e), 'err'); onClose() }) }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!ev) return null
  return <EventModal event={ev} onClose={onClose} />
}

export function wxIcon(d?: { code?: number; precip_prob?: number }): string {
  const c = d?.code ?? 0
  if (c >= 71 && c <= 86) return 'snowflake'
  if ((c >= 51 && c <= 67) || c >= 80 || (d?.precip_prob || 0) >= 60) return 'cloud-rain'
  if (c >= 2) return 'cloud'
  return 'sun'
}

function WeekInWords() {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [why, setWhy] = useState('')
  const [ready, setReady] = useState<boolean | null>(null)
  useEffect(() => { api.get<{ ready: boolean; detail: string }>(`${V1}/ai/status`).then(r => { setReady(r.ready); setWhy(r.detail) }).catch(() => setReady(false)) }, [])
  const ask = async (fresh = false) => {
    setBusy(true)
    try { const r = await api.get<{ text: string }>(`${V1}/ai/week${fresh ? '?fresh=1' : ''}`); setText(r.text) } catch (e) { setWhy(e instanceof ApiError ? e.message : errMsg(e)) } finally { setBusy(false) }
  }
  return (
    <Panel title="The week in a few lines" icon="sparkles" actions={text ? <button type="button" className="btn sm ghost" onClick={() => ask(true)} disabled={busy}>Again</button> : undefined}>
      {text ? <p className="ai-text">{text}</p> : (
        <div className="pad">
          <button type="button" className="btn sm" onClick={() => ask()} disabled={busy || !ready} title={ready ? 'Pocket AI says the busy days, what nobody has taken and what to remember' : why || 'Needs AI (Settings → AI)'}>
            <Icon name="sparkles" size={14} />{busy ? 'Thinking…' : 'Tell me about the week'}
          </button>
          {ready === false && <p className="muted small mt">Needs AI: Pocket AI through Pocket Core, or your own key in Settings → AI.</p>}
        </div>
      )}
    </Panel>
  )
}

