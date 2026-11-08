// Chores: what is due now for whom, every chore and how it repeats, and the
// week's stars for the children.
import { useCallback, useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ago, todayISO } from '../lib/format'
import { WEEKDAYS, canEdit, dayWords, useFamily, useOnChange } from '../lib/family'
import type { Chore, Done } from '../lib/types'
import { ChoreRow, Panel } from '../components/Bits'
import { ChoreModal } from '../components/Forms'
import { MakeMenu } from '../components/MakeMenu'
import { Face, PersonChip } from '../components/People'
import { Icon } from '../components/icons'
import { Empty, Loading, errMsg } from '../components/ui'

interface Stars { week: string; people: { person_id: number; done: number; stars: number; last_week_stars: number }[]; recent: Done[] }

const repeatWords = (c: Chore) => c.repeat === 'daily' ? 'every day' : c.repeat === 'weekly' ? (c.weekdays.length ? c.weekdays.map(d => WEEKDAYS[d - 1]).join(', ') : 'weekly') : c.repeat === 'every' ? `every ${c.every_days} days` : c.repeat === 'monthly' ? 'monthly' : 'once'

export default function Chores() {
  const { family, me, person, name } = useFamily()
  const { settings } = useApp()
  const [list, setList] = useState<Chore[] | null>(null)
  const [stars, setStars] = useState<Stars | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState<Partial<Chore> | null>(null)
  const load = useCallback(() => {
    api.get<Chore[]>(`${V1}/chores?all=1`).then(setList).catch(e => setErr(errMsg(e)))
    api.get<Stars>(`${V1}/stars`).then(setStars).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  if (err) return <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>
  if (!list || !me) return <Loading />
  const today = todayISO()
  const grown = canEdit(me)
  const due = list.filter(c => c.active && c.next_due && c.next_due <= today)
  const later = list.filter(c => c.active && c.next_due > today)
  const off = list.filter(c => !c.active)
  const kids = (stars?.people || []).filter(p => person(p.person_id)?.role === 'child')
  return (
    <div className="chores-page">
      <header className="page-head">
        <div><h2>Chores</h2><p className="muted">Tick off what's done; chores in turns pass to the next person by themselves.</p></div>
        {grown && <button type="button" className="btn primary" onClick={() => setEdit({})}><Icon name="plus" size={15} />A chore</button>}
      </header>
      <div className="two-panels">
        <div className="col">
          <Panel title="To do now" icon="broom" count={due.length}>
            {due.length ? due.map(c => <ChoreRow key={c.id} chore={c} onChanged={load} />) : <p className="muted small pad">Nothing due. 🎉</p>}
          </Panel>
          <Panel title="Coming up" icon="calendar">
            {later.length === 0 ? <p className="muted small pad">Nothing coming up.</p> : later.map(c => (
              <button key={c.id} type="button" className="li-row" onClick={() => grown && setEdit(c)} disabled={!grown}>
                <span className="li-emoji">{c.emoji || '•'}</span>
                <span className="grow"><span className="ellipsis">{c.title}</span><span className="muted small">{dayWords(c.next_due)} · {repeatWords(c)}{c.stars ? ' · ' + '⭐'.repeat(Math.min(c.stars, 5)) : ''}</span></span>
                {c.whose > 0 ? <PersonChip id={c.whose} small /> : c.assign === 'one' && c.people.length ? <span className="faces">{c.people.map(id => <Face key={id} person={person(id)} size={20} />)}</span> : <span className="muted small">anyone</span>}
              </button>
            ))}
          </Panel>
          {grown && (
            <Panel title="Every chore" icon="list" count={list.length}>
              {list.length === 0 ? <Empty icon="broom" title="No chores yet">Dishes in turns, feeding the cat, the bins on Tuesdays — add them once.</Empty> : list.map(c => (
                <div key={c.id} className={`li-row ${c.active ? '' : 'is-off'}`}>
                  <span className="li-emoji">{c.emoji || '•'}</span>
                  <button type="button" className="grow li-btn" onClick={() => setEdit(c)}>
                    <span className="ellipsis">{c.title}</span>
                    <span className="muted small">{repeatWords(c)} · {c.assign === 'turns' ? 'in turns: ' : c.assign === 'anyone' ? 'anyone' : ''}{c.people.map(id => name(id)).join(', ')}{c.last_done ? ` · last ${ago(c.last_done.created_at)} by ${name(c.last_done.person_id)}` : ''}{!c.active && ' · done (one-off)'}</span>
                  </button>
                  <MakeMenu owner="chore" ownerID={c.id} targets={['task']} label="" className="btn sm ghost icon-only" />
                </div>
              ))}
              {off.length > 0 && <p className="muted small pad">{off.length} done one-off {off.length === 1 ? 'chore' : 'chores'} kept for the history.</p>}
            </Panel>
          )}
        </div>
        <div className="col">
          {settings?.show_stars !== false && kids.length > 0 && (
            <Panel title="Stars this week" icon="star">
              <div className="stars">
                {kids.sort((a, b) => b.stars - a.stars).map(p => (
                  <div key={p.person_id} className="star-row">
                    <Face person={person(p.person_id)} size={30} />
                    <span className="grow"><b>{name(p.person_id)}</b><span className="muted small">{p.done} done · last week {p.last_week_stars} ⭐</span></span>
                    <span className="star-n tnum">{p.stars}<span aria-hidden="true"> ⭐</span></span>
                  </div>
                ))}
              </div>
            </Panel>
          )}
          <Panel title="Done this week" icon="check-circle">
            {!stars || stars.recent.length === 0 ? <p className="muted small pad">Nothing yet this week.</p> : (
              <ul className="acts">
                {stars.recent.map(d => <li key={d.id}><Face person={person(d.person_id)} size={18} /><span><b>{name(d.person_id) || 'Someone'}</b> {d.emoji} {d.title}</span><span className="muted small">{dayWords(d.day)}</span></li>)}
              </ul>
            )}
          </Panel>
        </div>
      </div>
      {edit && <ChoreModal chore={edit} onClose={() => { setEdit(null); load() }} />}
    </div>
  )
}
