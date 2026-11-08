// The week: every day's runs, plans (the family's and each person's own
// apps), chores and things to take, one column a day (a list on a phone).
import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { addDays, fmtDate, todayISO } from '../lib/format'
import { WEEKDAYS, canEdit, isoWeekday, useFamily, useOnChange } from '../lib/family'
import type { AgendaItem, SourceState, WeekDay } from '../lib/types'
import { AgendaRow, BringList, RunMini, SourcesLine } from '../components/Bits'
import { openCreate } from '../components/Layout'
import { MakeMenu } from '../components/MakeMenu'
import { Face } from '../components/People'
import { Icon } from '../components/icons'
import { Loading, errMsg, useIsMobile } from '../components/ui'
import { EventLoader } from './Today'

function weekStart(day: string, start: number) {
  let d = day
  while (isoWeekday(d) !== start) d = addDays(d, -1)
  return d
}

export default function Week() {
  const { settings } = useApp()
  const { family, people, me } = useFamily()
  const [sp, setSp] = useSearchParams()
  const mobile = useIsMobile()
  const start = settings?.week_start || 1
  const from = weekStart(sp.get('day') || todayISO(), start === 6 ? 6 : start === 7 ? 7 : 1)
  const [days, setDays] = useState<WeekDay[] | null>(null)
  const [sources, setSources] = useState<SourceState[]>([])
  const [err, setErr] = useState('')
  const [who, setWho] = useState(0)
  const [open, setOpen] = useState<AgendaItem | null>(null)
  const load = useCallback(() => {
    api.get<{ days: WeekDay[]; sources: SourceState[] }>(`${V1}/week?from=${from}`).then(r => { setDays(r.days); setSources(r.sources) }).catch(e => setErr(errMsg(e)))
  }, [from])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  const go = (n: number) => setSp({ day: addDays(from, n) }, { replace: true })
  const today = todayISO()
  const forWho = (ids?: number[]) => !who || !ids || ids.length === 0 || ids.includes(who)
  return (
    <div className="week">
      <header className="page-head">
        <div className="row">
          <button type="button" className="btn icon-only" onClick={() => go(-7)} aria-label="Week before"><Icon name="chevron-left" size={16} /></button>
          <button type="button" className="btn" onClick={() => setSp({}, { replace: true })} disabled={from === weekStart(today, start)}>This week</button>
          <button type="button" className="btn icon-only" onClick={() => go(7)} aria-label="Week after"><Icon name="chevron-right" size={16} /></button>
          <h2 className="week-title">{fmtDate(from, false)} – {fmtDate(addDays(from, 6), true)}</h2>
        </div>
        <div className="row wrap">
          <div className="who-filter" role="group" aria-label="Show for">
            <button type="button" className={`who-btn ${who === 0 ? 'on' : ''}`} onClick={() => setWho(0)}>Everyone</button>
            {people.filter(p => !p.archived).map(p => <button key={p.id} type="button" className={`who-btn ${who === p.id ? 'on' : ''}`} onClick={() => setWho(who === p.id ? 0 : p.id)} title={p.name}><Face person={p} size={20} />{!mobile && p.name.split(' ')[0]}</button>)}
          </div>
          <MakeMenu owner="week" ownerID={0} targets={['note']} label="Save" />
        </div>
      </header>
      {err && <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>}
      {!days ? <Loading /> : (
        <div className="week-grid">
          {days.map((d, i) => {
            const runs = d.runs.filter(r => r.state !== 'skipped' && (!who || r.children.includes(who) || r.driver_id === who))
            const ag = d.agenda.filter(a => forWho(a.people) || a.source !== 'family')
            const chores = d.chores.filter(c => !who || c.people.includes(who) || c.whose === who)
            const bring = d.bring.filter(b => !who || !b.person_id || b.person_id === who)
            return (
              <section key={d.day} className={`wday ${d.day === today ? 'is-today' : ''} ${d.day < today ? 'is-past' : ''}`}>
                <header className="wday-head">
                  <span className="wday-name">{WEEKDAYS[(isoWeekday(d.day) + 6) % 7]}</span>
                  <span className="wday-date tnum">{Number(d.day.slice(8))}</span>
                  {d.no_school && <span className="badge">no school</span>}
                  <span className="spacer" />
                  {canEdit(me) && <button type="button" className="btn ghost icon-only sm" aria-label="Add a plan this day" onClick={() => openCreate('event', { day: d.day })}><Icon name="plus" size={14} /></button>}
                </header>
                <div className="wday-body">
                  {runs.map(r => <RunMini key={r.id} run={r} />)}
                  {ag.map((a, j) => <AgendaRow key={j} item={a} onOpen={setOpen} mini />)}
                  {chores.length > 0 && <div className="wday-chores muted small"><Icon name="broom" size={13} />{chores.length === 1 ? chores[0].title : `${chores.length} chores`}</div>}
                  {bring.length > 0 && <BringList items={bring} />}
                  {!runs.length && !ag.length && !bring.length && <p className="muted small wday-empty">{i === 0 ? 'A quiet day' : 'Nothing'}</p>}
                </div>
              </section>
            )
          })}
        </div>
      )}
      <SourcesLine sources={sources} />
      {open && open.id && <EventLoader id={open.id} onClose={() => { setOpen(null); load() }} />}
    </div>
  )
}
