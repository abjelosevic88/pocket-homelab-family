// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { canEdit, useFamily, useOnChange } from '../lib/family'
import { Icon } from './icons'
import { Sidebar, type Counts } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ChoreModal, EventModal, LetterModal, PlanModal, PostModal, QuickAdd, RunModal } from './Forms'
import { Avatar, Loading, type Anchor, type MenuItem, Menu, MenuButton, modalOpen, rectOf, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'
import Welcome from '../pages/Welcome'

const SIDEBAR_KEY = 'pfy-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'run' | 'plan' | 'event' | 'chore' | 'item' | 'letter' | 'post' | 'quick'

/** Open a create action from anywhere (a page's button). */
export const openCreate = (k: CreateKind, detail: Record<string, unknown> = {}) => window.dispatchEvent(new CustomEvent('pfy:create', { detail: { kind: k, ...detail } }))

/** The add menu: the sidebar's "Add" button and the phone's + button. */
export function createItems(onCreate: (k: CreateKind) => void, grown: boolean): MenuItem[] {
  const all: MenuItem[] = [
    { label: 'Say it in your words…', icon: 'sparkles', hint: 'A', onSelect: () => onCreate('quick') },
    'sep',
    { label: 'A pick-up or drop-off', icon: 'car', hint: 'P', onSelect: () => onCreate('run') },
    { label: 'Every week (school run, club)', icon: 'repeat', onSelect: () => onCreate('plan') },
    { label: 'A plan on the family week', icon: 'calendar-plus', hint: 'E', onSelect: () => onCreate('event') },
    { label: 'A chore', icon: 'broom', hint: 'C', onSelect: () => onCreate('chore') },
    { label: 'Something to buy', icon: 'cart', hint: 'B', onSelect: () => onCreate('item') },
    { label: 'A letter from school', icon: 'mail', hint: 'L', onSelect: () => onCreate('letter') },
    { label: 'A note on the board', icon: 'pin', hint: 'N', onSelect: () => onCreate('post') },
  ]
  if (grown) return all
  return all.filter(i => typeof i === 'object' && 'label' in i && (i.icon === 'cart' || i.icon === 'pin'))
}

export default function Layout() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { families, family, me } = useFamily()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [counts, setCounts] = useState<Counts | null>(null)
  const load = useCallback(() => { if (family) api.get<Counts>(`${V1}/summary`).then(setCounts).catch(() => {}) }, [family])
  useEffect(() => { load() }, [loc.pathname, load])
  useOnChange(load)

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [modal, setModal] = useState<{ kind: CreateKind; day?: string } | null>(null)
  const [fabMenu, setFabMenu] = useState<Anchor | null>(null)
  const grown = canEdit(me)

  const create = (k: CreateKind, day?: string) => {
    closeDrawer()
    if (k === 'item') { navigate('/shopping?add=1'); return }
    setModal({ kind: k, day })
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; day?: string }>).detail; createRef.current(d.kind, d.day) }
    window.addEventListener('pfy:shortcuts', open)
    window.addEventListener('pfy:create', make)
    return () => { window.removeEventListener('pfy:shortcuts', open); window.removeEventListener('pfy:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); if (grown || k === 'item' || k === 'post') createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'a': mk('quick'); break
        case 'p': mk('run'); break
        case 'e': mk('event'); break
        case 'c': mk('chore'); break
        case 'b': mk('item'); break
        case 'l': mk('letter'); break
        case 'n': mk('post'); break
        case 'h': go('/'); break
        case 'w': go('/week'); break
        case 'u': go('/pickups'); break
        case 'k': go('/chores'); break
        case 's': go('/shopping'); break
        case 'o': go('/school'); break
        case 'f': go('/family'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    ...(grown ? [
      { id: 'quick', label: 'Say it in your words (Pocket AI)', icon: 'sparkles', hint: 'A', keywords: 'add ai quick sentence', run: () => create('quick') },
      { id: 'run', label: 'Add a pick-up or drop-off', icon: 'car', hint: 'P', keywords: 'collect drive lift school run', run: () => create('run') },
      { id: 'plan', label: 'Add a weekly run', icon: 'repeat', keywords: 'every week rota school run', run: () => create('plan') },
      { id: 'event', label: 'Add a plan', icon: 'calendar-plus', hint: 'E', keywords: 'event appointment party', run: () => create('event') },
      { id: 'chore', label: 'Add a chore', icon: 'broom', hint: 'C', keywords: 'job housework', run: () => create('chore') },
      { id: 'letter', label: 'Add a letter from school', icon: 'mail', hint: 'L', keywords: 'notice permission slip trip', run: () => create('letter') },
    ] : []),
    { id: 'item', label: 'Add something to buy', icon: 'cart', hint: 'B', keywords: 'shopping groceries list', run: () => create('item') },
    { id: 'post', label: 'Put a note on the board', icon: 'pin', hint: 'N', keywords: 'message fridge', run: () => create('post') },
    { id: 'home', label: 'Today', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'week', label: 'The week', icon: 'calendar', hint: 'W', run: () => navigate('/week') },
    { id: 'pickups', label: 'Pick-ups', icon: 'car', hint: 'U', keywords: 'runs drivers rota', run: () => navigate('/pickups') },
    { id: 'chores', label: 'Chores', icon: 'broom', hint: 'K', keywords: 'stars jobs', run: () => navigate('/chores') },
    { id: 'shopping', label: 'Shopping', icon: 'cart', hint: 'S', run: () => navigate('/shopping') },
    { id: 'school', label: 'School', icon: 'school', hint: 'O', keywords: 'letters holidays term timetable', run: () => navigate('/school') },
    { id: 'board', label: 'The board', icon: 'pin', keywords: 'notes posts', run: () => navigate('/board') },
    { id: 'family', label: 'Family and info', icon: 'users', hint: 'F', keywords: 'people emergency doctor sitter', run: () => navigate('/family') },
    { id: 'sheet', label: 'The sitter sheet', icon: 'clipboard', keywords: 'babysitter print emergency', run: () => navigate('/family/sheet') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'layers', keywords: 'calendar tasks kitchen money core integrations', run: () => navigate('/settings/apps') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const closeModal = () => setModal(null)
  const noFamily = families !== null && families.length === 0
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={k => create(k)} onNavigate={closeDrawer} counts={counts} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner">{families === null ? <Loading /> : noFamily ? <Welcome /> : <Outlet />}</div></main>
        </div>
        {mobile && !noFamily && family && <button type="button" className="fab" aria-label="Add" title="Add" aria-haspopup="menu" aria-expanded={!!fabMenu} onClick={e => setFabMenu(fabMenu ? null : rectOf(e.currentTarget))}><Icon name="plus" size={26} strokeWidth={2.2} /></button>}
        {fabMenu && <Menu anchor={fabMenu} align="right" items={createItems(k => create(k), grown)} onClose={() => setFabMenu(null)} width={270} />}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {modal?.kind === 'run' && <RunModal day={modal.day} onClose={closeModal} />}
        {modal?.kind === 'plan' && <PlanModal onClose={closeModal} />}
        {modal?.kind === 'event' && <EventModal day={modal.day} onClose={closeModal} />}
        {modal?.kind === 'chore' && <ChoreModal onClose={closeModal} />}
        {modal?.kind === 'letter' && <LetterModal onClose={closeModal} onSaved={l => navigate(`/school/letters/${l.id}`)} />}
        {modal?.kind === 'post' && <PostModal onClose={closeModal} />}
        {modal?.kind === 'quick' && <QuickAdd onClose={closeModal} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = {
  '': ['Today', '/'], week: ['The week', '/week'], pickups: ['Pick-ups', '/pickups'], chores: ['Chores', '/chores'], shopping: ['Shopping', '/shopping'],
  school: ['School', '/school'], board: ['The board', '/board'], family: ['Family', '/family'],
}

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pfy:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}

