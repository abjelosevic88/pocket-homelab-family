// Shopping: shared lists everyone adds to, in the order of a shop, with
// "I'm at the shop" so the others can add what they need now.
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { ago } from '../lib/format'
import { announceChange, canEdit, useFamily, useOnChange } from '../lib/family'
import type { Item, List } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { useApp } from '../lib/app-context'
import { Face } from '../components/People'
import { Icon } from '../components/icons'
import { Confirm, Loading, MenuButton, type MenuItem, Modal, errMsg, useToast } from '../components/ui'

export default function Shopping() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const { family } = useFamily()
  const [lists, setLists] = useState<List[] | null>(null)
  const load = useCallback(() => { api.get<List[]>(`${V1}/lists`).then(setLists).catch(() => setLists([])) }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  useEffect(() => { if (lists && !id && lists.length) navigate(`/shopping/${lists[0].id}${sp.get('add') ? '?add=1' : ''}`, { replace: true }) }, [lists, id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!lists) return <Loading />
  const cur = lists.find(l => String(l.id) === id)
  return (
    <div className="shopping">
      <div className="list-tabs" role="tablist">
        {lists.map(l => (
          <Link key={l.id} to={`/shopping/${l.id}`} role="tab" aria-selected={l.id === cur?.id} className={`list-tab ${l.id === cur?.id ? 'on' : ''}`}>
            <span>{l.emoji || '🛒'}</span>{l.name}{l.open > 0 && <span className="list-tab-n">{l.open}</span>}
          </Link>
        ))}
        <NewList onMade={l => { load(); navigate(`/shopping/${l.id}`) }} />
      </div>
      {cur ? <ListView key={cur.id} list={cur} lists={lists} onChanged={load} /> : <Loading />}
    </div>
  )
}

function NewList({ onMade }: { onMade: (l: List) => void }) {
  const { me } = useFamily()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const { toast } = useToast()
  if (!canEdit(me)) return null
  const make = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const l = await api.post<List>(`${V1}/lists`, { name }); setOpen(false); setName(''); onMade(l) } catch (er) { toast(errMsg(er), 'err') }
  }
  return (
    <>
      <button type="button" className="list-tab add" onClick={() => setOpen(true)} aria-label="A new list"><Icon name="plus" size={14} /></button>
      {open && <Modal size="sm" title="A new list" onClose={() => setOpen(false)} footer={<><button type="button" className="btn" onClick={() => setOpen(false)}>Cancel</button><button type="submit" form="nl" className="btn primary">Make</button></>}>
        <form id="nl" onSubmit={make}><input value={name} onChange={e => setName(e.target.value)} placeholder="Pharmacy, DIY shop, the market" autoFocus required /></form>
      </Modal>}
    </>
  )
}

function ListView({ list, lists, onChanged }: { list: List; lists: List[]; onChanged: () => void }) {
  const { me, person, name } = useFamily()
  const { toast } = useToast()
  const [sp, setSp] = useSearchParams()
  const [items, setItems] = useState<Item[] | null>(null)
  const [text, setText] = useState('')
  const [recent, setRecent] = useState<string[]>([])
  const [del, setDel] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const { settings } = useApp()
  useCrumb(list.name)
  const load = useCallback(() => { api.get<{ items: Item[] }>(`${V1}/lists/${list.id}/items`).then(r => setItems(r.items)).catch(() => setItems([])) }, [list.id])
  useEffect(() => { load(); api.get<string[]>(`${V1}/items/recent`).then(setRecent).catch(() => {}) }, [load])
  useOnChange(load)
  useEffect(() => { if (sp.get('add')) { input.current?.focus(); setSp({}, { replace: true }) } }, [sp, setSp])
  const add = async (names: string) => {
    if (!names.trim()) return
    try { const r = await api.post<{ added: number }>(`${V1}/lists/${list.id}/items`, { name: names }); setText(''); if (!r.added) toast('Already on the list'); load(); onChanged(); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const toggle = async (it: Item) => {
    setItems(xs => xs?.map(x => x.id === it.id ? { ...x, done: !x.done } : x) || null)
    try { await api.put(`${V1}/items/${it.id}`, { done: !it.done }); onChanged() } catch (e) { toast(errMsg(e), 'err'); load() }
  }
  const shopping = async (on: boolean) => {
    try { await api.post(`${V1}/lists/${list.id}/shopping`, { on }); toast(on ? 'The others are told — happy shopping' : 'Thanks!'); onChanged() } catch (e) { toast(errMsg(e), 'err') }
  }
  const fromPlan = async () => {
    try { const r = await api.post<{ added: number; meals: string[] }>(`${V1}/lists/${list.id}/from-plan`, {}); toast(r.meals.length ? `${r.added} things for ${r.meals.length} planned ${r.meals.length === 1 ? 'meal' : 'meals'}` : 'No planned meals with recipes this week'); load(); onChanged() } catch (e) { toast(errMsg(e), 'err') }
  }
  const clear = async () => { await api.post(`${V1}/lists/${list.id}/clear`); load(); onChanged() }
  if (!items) return <Loading />
  const open = items.filter(i => !i.done)
  const done = items.filter(i => i.done)
  const groups = new Map<string, Item[]>()
  for (const it of open) if (it.aisle) groups.set(it.aisle, [...(groups.get(it.aisle) || []), it])
  const other = open.filter(it => !it.aisle)
  if (other.length) groups.set('Other', other)
  const shopper = person(list.shopper_id)
  const kitchenOn = !!settings?.kitchen_via
  const menu = (): MenuItem[] => [
    { label: kitchenOn ? 'Ingredients for the meal plan' : 'Ingredients for the meal plan (needs Pocket Kitchen)', icon: 'utensils', onSelect: fromPlan, disabled: !kitchenOn },
    { label: 'Clear what was bought', icon: 'check-double', onSelect: clear, disabled: !done.length },
    ...(canEdit(me) && lists.length > 1 ? ['sep' as const, { label: 'Remove this list', icon: 'trash', danger: true, onSelect: () => setDel(true) }] : []),
  ]
  const suggestions = recent.filter(r => !open.some(i => i.name.toLowerCase() === r.toLowerCase())).slice(0, 10)
  return (
    <div className="listview">
      <div className={`shop-bar ${shopper ? 'on' : ''}`}>
        {shopper ? <>
          <Face person={shopper} size={26} />
          <span className="grow">{shopper.id === me?.id ? 'You are at the shop' : `${shopper.name.split(' ')[0]} is at the shop`}<span className="muted small"> · since {ago(list.shopper_since)}</span></span>
          {shopper.id === me?.id && <button type="button" className="btn sm" onClick={() => shopping(false)}>Done shopping</button>}
        </> : <>
          <Icon name="cart" size={18} />
          <span className="grow muted">{open.length ? `${open.length} to get` : 'Nothing to get'}</span>
          <button type="button" className="btn sm primary" onClick={() => shopping(true)}>I'm at the shop</button>
        </>}
        <MenuButton label="More" items={menu} align="right" className="btn sm ghost icon-only"><Icon name="more" size={16} /></MenuButton>
      </div>
      <form className="add-item" onSubmit={e => { e.preventDefault(); add(text) }}>
        <input ref={input} value={text} onChange={e => setText(e.target.value)} placeholder="Add — “2 milk”, “bread”, or paste a list" aria-label="Add to the list" />
        <button className="btn primary" disabled={!text.trim()}><Icon name="plus" size={15} />Add</button>
      </form>
      {suggestions.length > 0 && <div className="sugg">{suggestions.map(s => <button key={s} type="button" className="sugg-btn" onClick={() => add(s)}>+ {s}</button>)}</div>}
      {open.length === 0 && <p className="muted pad">The list is empty.</p>}
      {[...groups.entries()].map(([aisle, list]) => (
        <section key={aisle} className="aisle">
          <h4 className="aisle-h">{aisle}</h4>
          {list.map(it => <ItemRow key={it.id} it={it} onToggle={() => toggle(it)} name={name} />)}
        </section>
      ))}
      {done.length > 0 && (
        <section className="aisle done">
          <h4 className="aisle-h">Got it</h4>
          {done.map(it => <ItemRow key={it.id} it={it} onToggle={() => toggle(it)} name={name} />)}
        </section>
      )}
      {del && <Confirm title={`Remove “${list.name}”?`} message="Its items go too." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/lists/${list.id}`); setDel(false); onChanged(); window.location.assign('/shopping') }} />}
    </div>
  )
}

function ItemRow({ it, onToggle, name }: { it: Item; onToggle: () => void; name: (id?: number) => string }) {
  return (
    <div className={`item ${it.done ? 'is-done' : ''}`}>
      <button type="button" className="tick-box" onClick={onToggle} aria-label={it.done ? `Not got: ${it.name}` : `Got: ${it.name}`} aria-pressed={it.done}><Icon name="check" size={14} strokeWidth={2.4} /></button>
      <span className="item-name">{it.name}{it.qty && <span className="item-qty"> {it.qty}</span>}</span>
      <span className="muted small item-who">{it.done ? (it.done_by ? name(it.done_by) : '') : it.note || (it.added_by ? name(it.added_by) : '')}</span>
    </div>
  )
}
