// The family in view, its people, and the words the pages share.
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { api, V1 } from './api'
import { fmtDate, todayISO, addDays } from './format'
import type { AppSlug, Family, MakeTarget, Person, Run, RunKind } from './types'

interface FamilyState {
  families: Family[] | null
  family: Family | null
  me: Person | null
  people: Person[]
  person: (id?: number) => Person | undefined
  name: (id?: number) => string
  reload: () => Promise<void>
  switchTo: (id: number) => Promise<void>
}

const Ctx = createContext<FamilyState>(null!)
export const useFamily = () => useContext(Ctx)

export function FamilyProvider({ children }: { children: React.ReactNode }) {
  const [families, setFamilies] = useState<Family[] | null>(null)
  const [current, setCurrent] = useState(0)
  const reload = useCallback(async () => {
    const r = await api.get<{ families: Family[]; current: number }>(`${V1}/families`)
    setFamilies(r.families)
    setCurrent(r.current)
  }, [])
  useEffect(() => { reload().catch(() => setFamilies([])) }, [reload])
  useOnChange(() => { reload().catch(() => {}) })
  const switchTo = useCallback(async (id: number) => {
    await api.post(`${V1}/families/switch`, { id })
    await reload()
    announceChange()
  }, [reload])
  const family = families?.find(f => f.id === current) || families?.[0] || null
  const value = useMemo<FamilyState>(() => {
    const people = family?.people || []
    const person = (id?: number) => (id ? people.find(p => p.id === id) : undefined)
    return {
      families, family, me: family?.me || null, people, person, reload, switchTo,
      name: (id?: number) => firstName(person(id)?.name || ''),
    }
  }, [families, family, reload, switchTo])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

/** Something changed (from a page or a dialog): pages and counts reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pfy:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pfy:changed', h)
    return () => window.removeEventListener('pfy:changed', h)
  }, [])
}

export const firstName = (n: string) => n.trim().split(/\s+/)[0] || ''

export const canEdit = (p?: Person | null) => !!p && p.role !== 'child'

export const ROLE_WORDS: Record<string, string> = { grownup: 'Grown-up', child: 'Child', helper: 'Helps out' }

export const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
export const WEEKDAYS_LONG = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']

/** ISO weekday 1–7 of a YYYY-MM-DD date. */
export function isoWeekday(day: string): number {
  const [y, m, d] = day.split('-').map(Number)
  const w = new Date(y, m - 1, d).getDay()
  return w === 0 ? 7 : w
}

/** "Today", "Tomorrow", "Yesterday", "Thursday", "Mon 3 Nov". */
export function dayWords(day: string, long = false): string {
  const t = todayISO()
  if (day === t) return 'Today'
  if (day === addDays(t, 1)) return 'Tomorrow'
  if (day === addDays(t, -1)) return 'Yesterday'
  if (day > t && day < addDays(t, 7)) return WEEKDAYS_LONG[isoWeekday(day) - 1]
  return (long ? WEEKDAYS_LONG : WEEKDAYS)[isoWeekday(day) - 1] + ' ' + fmtDate(day, day.slice(0, 4) !== t.slice(0, 4))
}

export const RUN_VERB: Record<RunKind, string> = { pickup: 'Pick up', dropoff: 'Drop off', lift: 'Lift for' }
export const RUN_KIND: Record<RunKind, string> = { pickup: 'Pick-up', dropoff: 'Drop-off', lift: 'Lift' }

/** A run as people say it: "Pick up Mila". */
export function runTitle(r: Pick<Run, 'kind' | 'children' | 'title'>, name: (id?: number) => string): string {
  const who = r.children.map(id => name(id)).filter(Boolean)
  if (!who.length) return r.title || RUN_KIND[r.kind]
  return `${RUN_VERB[r.kind]} ${andList(who)}`
}

export function andList(words: string[]): string {
  if (words.length <= 1) return words.join('')
  return words.slice(0, -1).join(', ') + ' and ' + words[words.length - 1]
}

export const APP_INFO: Record<AppSlug, { name: string; icon: string }> = {
  calendar: { name: 'Pocket Calendar', icon: 'calendar' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square' },
  contacts: { name: 'Pocket Contacts', icon: 'users' },
  kitchen: { name: 'Pocket Kitchen', icon: 'utensils' },
  money: { name: 'Pocket Money', icon: 'wallet' },
  split: { name: 'Pocket Split', icon: 'receipt' },
  notes: { name: 'Pocket Notes', icon: 'note' },
  weather: { name: 'Pocket Weather', icon: 'cloud' },
  league: { name: 'Pocket League', icon: 'ball' },
  travel: { name: 'Pocket Travel', icon: 'plane' },
  pets: { name: 'Pocket Pets', icon: 'paw' },
  home: { name: 'Pocket Home', icon: 'home' },
  renewals: { name: 'Pocket Renewals', icon: 'shield' },
  chat: { name: 'Pocket Chat', icon: 'message' },
}
export const appName = (slug?: string) => (slug && APP_INFO[slug as AppSlug]?.name) || (slug ? 'Pocket ' + slug[0].toUpperCase() + slug.slice(1) : '')

export const MAKE: Record<MakeTarget, { label: string; icon: string; app: AppSlug }> = {
  task: { label: 'Make it a task', icon: 'check-square', app: 'tasks' },
  event: { label: 'Put it in my calendar', icon: 'calendar-plus', app: 'calendar' },
  note: { label: 'Save as a note', icon: 'note', app: 'notes' },
  payment: { label: 'Book the payment', icon: 'wallet', app: 'money' },
  expense: { label: 'Share the cost', icon: 'receipt', app: 'split' },
  contact: { label: 'Add to my contacts', icon: 'users', app: 'contacts' },
  chat: { label: 'Tell the family chat', icon: 'message', app: 'chat' },
}

/** Icons for the kinds of lines on a day. */
export const KIND_ICON: Record<string, string> = {
  school: 'school', activity: 'ball', appointment: 'stethoscope', family: 'heart', birthday: 'cake', other: 'calendar', term: 'sun',
  event: 'calendar', game: 'trophy', trip: 'plane', meal: 'utensils', pet: 'paw', home: 'wrench', renewal: 'shield',
}

export const EVENT_KINDS: { value: string; label: string }[] = [
  { value: 'family', label: 'Family' }, { value: 'school', label: 'School' }, { value: 'activity', label: 'Club or activity' },
  { value: 'appointment', label: 'Appointment' }, { value: 'other', label: 'Other' },
]

export const INFO_SECTIONS: { value: string; label: string; icon: string }[] = [
  { value: 'emergency', label: 'In an emergency', icon: 'phone' },
  { value: 'health', label: 'Health and allergies', icon: 'heart-pulse' },
  { value: 'school', label: 'School', icon: 'school' },
  { value: 'home', label: 'At home', icon: 'home' },
  { value: 'sitter', label: 'For the sitter', icon: 'moon' },
  { value: 'other', label: 'Good to know', icon: 'info' },
]

export function money(minor: number, cur: string): string {
  if (!minor) return ''
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur || 'EUR' }).format(minor / 100) } catch { return (minor / 100).toFixed(2) + ' ' + cur }
}

export const PALETTE = ['#e8603c', '#2b7a9e', '#7c3aed', '#0c8a6a', '#d6336c', '#b0571d', '#4263eb', '#567f0c']
