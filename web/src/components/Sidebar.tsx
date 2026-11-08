// Left column: the family, the add button, sections with what waits in each — the Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { canEdit, useFamily } from '../lib/family'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { Face } from './People'
import { type Anchor, Menu, type MenuItem, rectOf } from './ui'

export interface Counts {
  needs_someone: unknown[]; my_runs_today: unknown[]; chores_due_today: number; my_chores_today: number; shopping_open: number
  letters_to_do: number; unseen_posts: number; no_school_today: boolean
}

export function Sidebar({ onCreate, onNavigate, counts }: { onCreate: (k: CreateKind) => void; onNavigate: () => void; counts: Counts | null }) {
  const { status, settings } = useApp()
  const { families, family, me, people, switchTo } = useFamily()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const [famMenu, setFamMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Family'
  const n = (v?: number) => (v ? String(v) : undefined)
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: string; warn?: boolean }[] = [
    { to: '/', label: 'Today', icon: 'home', end: true },
    { to: '/week', label: 'The week', icon: 'calendar' },
    { to: '/pickups', label: 'Pick-ups', icon: 'car', count: n(counts?.needs_someone?.length), warn: !!counts?.needs_someone?.length },
    { to: '/chores', label: 'Chores', icon: 'broom', count: n(counts?.chores_due_today) },
    { to: '/shopping', label: 'Shopping', icon: 'cart', count: n(counts?.shopping_open) },
    { to: '/school', label: 'School', icon: 'school', count: n(counts?.letters_to_do) },
    { to: '/board', label: 'The board', icon: 'pin', count: n(counts?.unseen_posts), warn: !!counts?.unseen_posts },
    { to: '/family', label: 'Family', icon: 'users' },
  ]
  const famItems = (): MenuItem[] => [
    { heading: 'Your families' },
    ...(families || []).map(f => ({ label: `${f.emoji} ${f.name}`, checked: f.id === family?.id, onSelect: () => { setFamMenu(null); switchTo(f.id) } })),
  ]
  return (
    <aside className="sidebar" aria-label="Family">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Family' ? title : <>Pocket <b>Family</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      {family && (
        <button type="button" className="sb-family" onClick={e => (families?.length || 0) > 1 ? setFamMenu(famMenu ? null : rectOf(e.currentTarget)) : undefined} disabled={(families?.length || 0) < 2} title={(families?.length || 0) > 1 ? 'Switch family' : family.name}>
          <span className="sb-family-emoji">{family.emoji || '🏡'}</span>
          <span className="grow ellipsis">{family.name}</span>
          {(families?.length || 0) > 1 && <Icon name="chevron-down" size={14} />}
        </button>
      )}
      {famMenu && <Menu anchor={famMenu} items={famItems()} onClose={() => setFamMenu(null)} width={248} />}
      {family && (
        <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="plus" size={15} strokeWidth={2.2} />Add<Icon name="chevron-down" size={15} className="create-caret" />
        </button>
      )}
      {menu && <Menu anchor={menu} items={createItems(onCreate, canEdit(me))} onClose={() => setMenu(null)} width={268} />}

      {family && (
        <nav className="sb-nav" aria-label="Sections">
          {nav.map(x => (
            <NavLink key={x.to} to={x.to} end={x.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
              <Icon name={x.icon} size={16} />{x.label}
              {!!x.count && <span className={`sb-count ${x.warn ? 'warn' : ''}`}>{x.count}</span>}
            </NavLink>
          ))}
        </nav>
      )}

      {family && people.length > 0 && (
        <section className="sb-sec">
          <h4 className="sb-h"><span className="sb-h-btn">The family</span></h4>
          <div className="sb-people">
            {people.filter(p => !p.archived).map(p => (
              <Link key={p.id} to="/family" onClick={onNavigate} className="sb-person" title={p.name}>
                <Face person={p} size={22} /><span className="ellipsis">{p.name.split(' ')[0]}</span>{p.id === me?.id && <span className="muted small">you</span>}
              </Link>
            ))}
          </div>
          {counts?.no_school_today && <p className="sb-note muted small"><Icon name="sun" size={13} />No school today</p>}
        </section>
      )}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pfy:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
