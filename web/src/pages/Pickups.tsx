// Pick-ups: the next two weeks day by day (who takes each one, what nobody
// has taken yet), and the weekly plans they come from.
import { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { addDays, fmtClock, todayISO } from '../lib/format'
import { RUN_KIND, WEEKDAYS, canEdit, dayWords, runTitle, useFamily, useOnChange } from '../lib/family'
import type { Run, RunPlan } from '../lib/types'
import { Panel, RunRow } from '../components/Bits'
import { PlanModal } from '../components/Forms'
import { openCreate } from '../components/Layout'
import { Faces, PersonChip } from '../components/People'
import { Icon } from '../components/icons'
import { Empty, Loading, Segmented, errMsg } from '../components/ui'

export default function Pickups() {
  const { family, me, name } = useFamily()
  const [sp] = useSearchParams()
  const [runs, setRuns] = useState<Run[] | null>(null)
  const [plans, setPlans] = useState<RunPlan[] | null>(null)
  const [err, setErr] = useState('')
  const [show, setShow] = useState<'all' | 'mine' | 'open'>('all')
  const [plan, setPlan] = useState<Partial<RunPlan> | null>(null)
  const from = sp.get('day') && (sp.get('day') as string) < todayISO() ? (sp.get('day') as string) : todayISO()
  const load = useCallback(() => {
    api.get<{ runs: Run[] }>(`${V1}/runs?from=${from}&to=${addDays(from, 13)}`).then(r => setRuns(r.runs)).catch(e => setErr(errMsg(e)))
    api.get<RunPlan[]>(`${V1}/plans`).then(setPlans).catch(() => {})
  }, [from])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  useEffect(() => { const d = sp.get('day'); if (d && runs) document.getElementById('d-' + d)?.scrollIntoView({ block: 'start' }) }, [sp, runs])
  if (err) return <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div>
  if (!runs || !me) return <Loading />
  const grown = canEdit(me)
  const shown = runs.filter(r => show === 'mine' ? r.driver_id === me.id : show === 'open' ? (r.state !== 'done' && r.state !== 'skipped' && (!r.driver_id || r.state === 'asking')) : true)
  const byDay = new Map<string, Run[]>()
  for (const r of shown) byDay.set(r.day, [...(byDay.get(r.day) || []), r])
  const open = runs.filter(r => r.state !== 'done' && r.state !== 'skipped' && (!r.driver_id || r.state === 'asking')).length
  return (
    <div className="pickups">
      <header className="page-head">
        <div><h2>Who's picking up</h2><p className="muted">The next two weeks. Tap <b>I'll do it</b> to take one; <b>Can't make it</b> asks the others.</p></div>
        {grown && <div className="row"><button type="button" className="btn" onClick={() => setPlan({})}><Icon name="repeat" size={15} />Every week…</button><button type="button" className="btn primary" onClick={() => openCreate('run')}><Icon name="plus" size={15} />One run</button></div>}
      </header>
      <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'all', label: 'All' }, { value: 'mine', label: 'Mine' }, { value: 'open', label: open ? `Nobody yet (${open})` : 'Nobody yet' }]} />
      <div className="pickup-days">
        {shown.length === 0 ? <Empty icon="car" title={show === 'open' ? 'Every run has someone' : 'No runs'}>{show === 'all' && grown ? 'Add the school run once with “Every week…”; each day appears here.' : ''}</Empty> : [...byDay.entries()].map(([day, list]) => (
          <section key={day} id={'d-' + day} className={`pday ${day === todayISO() ? 'is-today' : ''}`}>
            <h3 className="pday-h">{dayWords(day, true)}{day === todayISO() && <span className="badge accent">today</span>}</h3>
            {list.map(r => <RunRow key={r.id} run={r} onChanged={load} />)}
          </section>
        ))}
      </div>
      <Panel title="Every week" icon="repeat" count={plans?.length}>
        {!plans ? <Loading /> : plans.length === 0 ? <p className="muted small pad">No weekly runs yet.</p> : plans.map(p => (
          <button key={p.id} type="button" className={`plan-row ${p.active ? '' : 'is-off'}`} onClick={() => grown && setPlan(p)} disabled={!grown}>
            <span className="plan-time tnum">{fmtClock(p.time)}</span>
            <span className="grow">
              <span className="plan-title">{runTitle(p, name)}{p.place && <span className="muted"> · {p.place}</span>}</span>
              <span className="muted small">{RUN_KIND[p.kind]} · {p.weekdays.map(d => WEEKDAYS[d - 1]).join(', ')}{p.term_only && ' · not in holidays'}{!p.active && ' · paused'}</span>
              {Object.keys(p.rota).length > 0 && <span className="plan-rota small">{p.weekdays.map(d => <span key={d}>{WEEKDAYS[d - 1]} {name(p.rota[String(d)] || p.driver_id) || '—'}</span>)}</span>}
            </span>
            <Faces ids={p.children} size={20} />
            {!Object.keys(p.rota).length && <PersonChip id={p.driver_id} small />}
          </button>
        ))}
      </Panel>
      {plan && <PlanModal plan={plan} onClose={() => { setPlan(null); load() }} />}
    </div>
  )
}
