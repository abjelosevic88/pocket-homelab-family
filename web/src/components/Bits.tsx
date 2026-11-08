// Rows the pages share: a pick-up with its buttons, a line on a day, a chore
// to tick, what to take along, and the line that says which apps are not there.
import type React from 'react'
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtClock, todayISO } from '../lib/format'
import { KIND_ICON, RUN_KIND, announceChange, appName, canEdit, runTitle, useFamily } from '../lib/family'
import type { AgendaItem, BringItem, Chore, Run, SourceState } from '../lib/types'
import { Icon } from './icons'
import { Face, Faces, PersonChip } from './People'
import { MadeLinks, MakeMenu } from './MakeMenu'
import { RunModal } from './Forms'
import { MenuButton, type MenuItem, Modal, errMsg, useToast } from './ui'

/** One line for the week grid: time, who it is for, who takes it. */
export function RunMini({ run }: { run: Run }) {
  const { person } = useFamily()
  const open = !run.driver_id || run.state === 'asking'
  return (
    <a href={`/pickups?day=${run.day}`} className={`run-mini ${run.state === 'done' ? 'is-done' : ''} ${open && run.state !== 'done' ? 'is-open' : ''}`}
      title={`${RUN_KIND[run.kind]}${run.place ? ' · ' + run.place : ''}${open ? ' · nobody yet' : ''}`}>
      <span className="tnum run-mini-t">{run.time ? fmtClock(run.time) : '—'}</span>
      <Icon name="car" size={13} className="run-mini-ico" />
      <Faces ids={run.children} size={18} max={3} />
      <span className="grow" />
      {run.state === 'done' ? <Icon name="check" size={13} className="ok" /> : open ? <span className="run-mini-open">?</span> : <Face person={person(run.driver_id)} size={18} />}
    </a>
  )
}

export function RunRow({ run, compact, onChanged }: { run: Run; compact?: boolean; onChanged?: () => void }) {
  const { name, me, person, people } = useFamily()
  const { toast } = useToast()
  const [edit, setEdit] = useState(false)
  const [ask, setAsk] = useState(false)
  const [busy, setBusy] = useState(false)
  const act = async (action: string, body: Record<string, unknown> = {}, ok = '') => {
    setBusy(true)
    try { await api.post(`${V1}/runs/${run.id}/${action}`, body); if (ok) toast(ok); announceChange(); onChanged?.() } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const grown = canEdit(me)
  const mine = run.driver_id === me?.id
  const nobody = !run.driver_id
  const asking = run.state === 'asking'
  const done = run.state === 'done'
  const skipped = run.state === 'skipped'
  const menu = (): MenuItem[] => [
    { label: 'Change…', icon: 'edit', onSelect: () => setEdit(true) },
    ...(!done && !skipped ? [{ label: 'Give it to…', icon: 'users', submenu: people.filter(p => p.role !== 'child' && !p.archived && p.id !== run.driver_id).map(p => ({ label: p.name, onSelect: () => act('assign', { person_id: p.id }, `${p.name.split(' ')[0]} takes it`) })) }] : []),
    ...(!done && !skipped ? [{ label: 'Not needed this time', icon: 'x-circle', onSelect: () => act('skip', {}, 'Called off') }] : []),
    ...(done || skipped ? [{ label: done ? 'Not done after all' : 'Needed after all', icon: 'undo', onSelect: () => act('undo') }] : []),
  ]
  return (
    <div className={`run ${done ? 'is-done' : ''} ${skipped ? 'is-skipped' : ''} ${(nobody || asking) && !done && !skipped ? 'is-open' : ''}`}>
      <div className="run-time tnum">{run.time ? fmtClock(run.time) : '—'}</div>
      <div className="run-main">
        <div className="run-title">
          <span className="run-kind">{RUN_KIND[run.kind]}</span>
          <span className="ellipsis">{runTitle(run, name).replace(/^(Pick up|Drop off|Lift for) /, '')}</span>
          <Faces ids={run.children} size={20} />
        </div>
        <div className="run-sub muted small">
          {run.place && <span><Icon name="pin" size={12} />{run.place}</span>}
          {asking && <span className="warn-text"><Icon name="alert" size={12} />{name(run.asked_by)} asked who can take it{run.asked_note ? `: “${run.asked_note}”` : ''}</span>}
          {run.clash && !done && <span className="warn-text"><Icon name="alert" size={12} />{name(run.driver_id)} is also due at {run.clash}</span>}
          {done && <span className="ok"><Icon name="check" size={12} />done{run.done_by ? ` by ${name(run.done_by)}` : ''}</span>}
          {skipped && <span>not needed</span>}
          {run.notes && !compact && <span>{run.notes}</span>}
        </div>
        {!compact && <MadeLinks made={run.made} />}
      </div>
      <div className="run-who">
        {nobody ? <span className="pchip nobody"><Face size={22} />Nobody yet</span> : <PersonChip id={run.driver_id} />}
      </div>
      {grown && (
        <div className="run-acts">
          {!done && !skipped && (nobody || asking) && !mine && <button type="button" className="btn sm primary" disabled={busy} onClick={() => act('take', {}, 'Thank you — it is yours')}>I'll do it</button>}
          {!done && !skipped && mine && !asking && <button type="button" className="btn sm" disabled={busy} onClick={() => setAsk(true)} title="Ask the others to take it">Can't make it</button>}
          {!done && !skipped && !nobody && (mine || person(run.driver_id)?.user_id === 0) && run.day <= todayISO() && <button type="button" className="btn sm" disabled={busy} onClick={() => act('done', {}, 'Done')}><Icon name="check" size={14} />Done</button>}
          {!compact && <MakeMenu owner="run" ownerID={run.id} targets={['event', 'chat']} made={run.made} label="" className="btn sm ghost icon-only" />}
          <MenuButton label="More" items={menu} align="right" className="btn sm ghost icon-only"><Icon name="more" size={16} /></MenuButton>
        </div>
      )}
      {edit && <RunModal run={run} onClose={() => setEdit(false)} onSaved={() => onChanged?.()} />}
      {ask && <AskModal onClose={() => setAsk(false)} onAsk={note => { setAsk(false); act('ask', { note }, 'Asked — the others are told') }} />}
    </div>
  )
}

function AskModal({ onClose, onAsk }: { onClose: () => void; onAsk: (note: string) => void }) {
  const [note, setNote] = useState('')
  return (
    <Modal size="sm" title="Ask someone else to take it" onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={() => onAsk(note)}>Ask</button></>}>
      <div className="stack">
        <p className="muted">The other grown-ups (and helpers) get a message and can take it with one tap. Until someone does, it stays yours.</p>
        <input value={note} onChange={e => setNote(e.target.value)} placeholder="Why (optional): I'm stuck at work" autoFocus />
      </div>
    </Modal>
  )
}

export function AgendaRow({ item, onOpen, mini }: { item: AgendaItem; onOpen?: (it: AgendaItem) => void; mini?: boolean }) {
  const ext = item.source !== 'family'
  const time = item.time ? fmtClock(item.time) + (item.end_time && !mini ? '–' + fmtClock(item.end_time) : '') : ''
  if (mini) {
    const inner = <><Icon name={KIND_ICON[item.kind] || 'calendar'} size={13} className={ext ? 'muted' : 'accent-text'} /><span className="ag-mini-text">{time && <b className="tnum">{time} </b>}{item.title}</span></>
    if (ext && item.url) return <a className="ag-mini" href={item.url} target="_blank" rel="noreferrer" title={appName(item.source)}>{inner}</a>
    if (!ext && onOpen && item.id && item.kind !== 'birthday' && item.kind !== 'term') return <button type="button" className="ag-mini" onClick={() => onOpen(item)}>{inner}</button>
    return <div className="ag-mini">{inner}</div>
  }
  const body = (
    <>
      <span className={`ag-ico ${ext ? 'ext' : ''} k-${item.kind}`}><Icon name={KIND_ICON[item.kind] || 'calendar'} size={15} /></span>
      <span className="ag-main">
        <span className="ag-title ellipsis">{item.title}</span>
        <span className="ag-sub muted small">
          {[time, item.place, item.sub, ext ? appName(item.source) : ''].filter(Boolean).join(' · ')}
          {item.bring && <span className="ag-bring"><Icon name="package" size={12} />{item.bring}</span>}
        </span>
      </span>
      {!!item.people?.length && <Faces ids={item.people} size={20} />}
      {ext && item.url && <Icon name="external" size={13} className="muted" />}
    </>
  )
  if (ext && item.url) return <a className={`ag-row ${item.overdue ? 'is-late' : ''}`} href={item.url} target="_blank" rel="noreferrer">{body}</a>
  if (!ext && onOpen && item.id && (item.kind !== 'birthday' && item.kind !== 'term')) return <button type="button" className="ag-row" onClick={() => onOpen(item)}>{body}</button>
  return <div className={`ag-row ${item.overdue ? 'is-late' : ''}`}>{body}</div>
}

export function ChoreRow({ chore, day, onChanged, showWho = true }: { chore: Chore; day?: string; onChanged?: () => void; showWho?: boolean }) {
  const { me, person } = useFamily()
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const today = todayISO()
  const late = chore.next_due < today
  const canTick = canEdit(me) || chore.whose === me?.id || chore.assign === 'anyone' || (chore.assign === 'one' && chore.people.includes(me?.id || 0))
  const tick = async (pid?: number) => {
    setBusy(true)
    try {
      await api.post(`${V1}/chores/${chore.id}/done`, { person_id: pid })
      toast(chore.stars ? `Done — ${'⭐'.repeat(Math.min(chore.stars, 5))}` : 'Done', 'ok', { label: 'Undo', run: () => { api.post(`${V1}/chores/${chore.id}/undo`).then(() => { announceChange(); onChanged?.() }) } })
      announceChange(); onChanged?.()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const who = chore.assign === 'anyone' ? 0 : chore.assign === 'turns' ? chore.whose : 0
  const each = chore.assign === 'one' ? chore.people : []
  return (
    <div className={`chore ${late ? 'is-late' : ''}`}>
      {each.length > 1 ? (
        <span className="chore-ticks">{each.map(pid => <button key={pid} type="button" className="tick" disabled={busy || (!canEdit(me) && pid !== me?.id)} onClick={() => tick(pid)} title={`${person(pid)?.name} did it`} aria-label={`${person(pid)?.name} did it`}><Face person={person(pid)} size={22} /></button>)}</span>
      ) : (
        <button type="button" className="tick-box" disabled={busy || !canTick || (day !== undefined && day > today)} onClick={() => tick(who || each[0] || undefined)} aria-label={`Done: ${chore.title}`}><Icon name="check" size={14} strokeWidth={2.4} /></button>
      )}
      <span className="chore-emoji">{chore.emoji}</span>
      <span className="chore-main">
        <span className="chore-title ellipsis">{chore.title}</span>
        <span className="muted small">
          {[late ? 'was due ' + chore.next_due.slice(5).split('-').reverse().join('.') : '', chore.part_of_day, chore.assign === 'turns' ? 'in turns' : chore.assign === 'anyone' ? 'anyone' : '', chore.stars ? '⭐'.repeat(Math.min(chore.stars, 5)) : ''].filter(Boolean).join(' · ')}
        </span>
      </span>
      {showWho && who > 0 && <PersonChip id={who} small />}
      {showWho && each.length === 1 && <PersonChip id={each[0]} small />}
    </div>
  )
}

export function BringList({ items }: { items: BringItem[] }) {
  const { person } = useFamily()
  if (!items.length) return null
  return (
    <ul className="bring">
      {items.map((b, i) => (
        <li key={i}><Face person={person(b.person_id)} size={20} title={person(b.person_id)?.name || b.for} /><span><b>{b.what}</b>{b.for && <span className="muted small"> · {b.for}</span>}</span></li>
      ))}
    </ul>
  )
}

/** Which apps were not read for this page, in one quiet line. */
export function SourcesLine({ sources }: { sources: SourceState[] }) {
  const [open, setOpen] = useState(false)
  const off = sources.filter(s => s.state !== 'on')
  if (!off.length) return null
  return (
    <div className="sources">
      <button type="button" className="sources-btn" onClick={() => setOpen(o => !o)} aria-expanded={open}>
        <Icon name="layers" size={13} />{off.length === 1 ? `${off[0].name} is not shown` : `${off.length} of your apps are not shown`}<Icon name={open ? 'chevron-up' : 'chevron-down'} size={13} />
      </button>
      {open && (
        <ul className="sources-list">
          {off.map(s => <li key={s.slug}><b>{s.name}</b>: <span className="muted">{s.detail || 'not connected'}{s.fix ? ` — ${s.fix}` : ''}</span></li>)}
        </ul>
      )}
    </div>
  )
}

export function Panel({ title, icon, actions, children, className = '', count }: { title: React.ReactNode; icon?: string; actions?: React.ReactNode; children: React.ReactNode; className?: string; count?: number }) {
  return (
    <section className={`panel ${className}`}>
      <header className="panel-head">{icon && <Icon name={icon} size={16} />}<h3>{title}</h3>{!!count && <span className="panel-count">{count}</span>}<span className="spacer" />{actions}</header>
      <div className="panel-body">{children}</div>
    </section>
  )
}
