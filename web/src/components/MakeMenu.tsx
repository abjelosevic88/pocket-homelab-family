// "Also in…": make a task, a calendar event, a note, a payment, a shared
// cost, a contact or a chat message from something here. An app that is
// not connected is greyed with what to do, never hidden.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { MAKE, announceChange } from '../lib/family'
import type { Made, MakeOption, MakeTarget } from '../lib/types'
import { Icon } from './icons'
import { Field, MenuButton, Modal, errMsg, useToast } from './ui'
import { Select } from './Select'

let optionsCache: { at: number; p: Promise<Record<MakeTarget, MakeOption>> } | null = null
export function makeOptions(): Promise<Record<MakeTarget, MakeOption>> {
  if (!optionsCache || Date.now() - optionsCache.at > 30000) optionsCache = { at: Date.now(), p: api.get(`${V1}/make/options`) }
  return optionsCache.p
}

export function useMakeOptions() {
  const [opts, setOpts] = useState<Record<MakeTarget, MakeOption> | null>(null)
  useEffect(() => { makeOptions().then(setOpts).catch(() => {}) }, [])
  return opts
}

interface Props { owner: string; ownerID: number; targets: MakeTarget[]; made?: Made[]; amount?: number; currency?: string; label?: string; className?: string; onMade?: (m: Made) => void }

export function MakeMenu({ owner, ownerID, targets, made = [], amount, currency, label = 'Also in…', className = 'btn sm', onMade }: Props) {
  const opts = useMakeOptions()
  const { toast } = useToast()
  const [ask, setAsk] = useState<MakeTarget | null>(null)
  const make = async (target: MakeTarget, extra: Record<string, unknown> = {}) => {
    try {
      const m = await api.post<Made>(`${V1}/make`, { owner, owner_id: ownerID, target, ...extra })
      toast(m.url ? <span>Done — <a href={m.url} target="_blank" rel="noreferrer">open it</a></span> : 'Done')
      onMade?.(m)
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const items = () => targets.map(t => {
    const o = opts?.[t]
    const already = made.find(m => m.target === t)
    return {
      label: <span className="mk-item"><span>{MAKE[t].label}</span>{o && !o.on && <span className="mk-why">{o.fix || o.detail || o.name + ' is not connected'}</span>}{already && <span className="mk-why">already made</span>}</span>,
      icon: MAKE[t].icon, disabled: !!o && !o.on,
      onSelect: () => (t === 'payment' || t === 'expense' || t === 'chat') ? setAsk(t) : make(t),
    }
  })
  return (
    <>
      <MenuButton label={label || "Also in another app"} className={className} items={items} width={300}><Icon name="layers" size={14} />{label}</MenuButton>
      {ask && <MakeDetails target={ask} amount={amount} currency={currency} onClose={() => setAsk(null)} onGo={extra => { const t = ask; setAsk(null); make(t, extra) }} />}
    </>
  )
}

/** The madeline: what was made already, with links. */
export function MadeLinks({ made }: { made?: Made[] }) {
  if (!made?.length) return null
  return (
    <div className="made-links">
      {made.map(m => (
        <a key={m.id} href={m.url || undefined} target="_blank" rel="noreferrer" className="made-link"><Icon name={MAKE[m.target]?.icon || 'link'} size={13} />{m.title}</a>
      ))}
    </div>
  )
}

function MakeDetails({ target, amount, currency, onClose, onGo }: { target: MakeTarget; amount?: number; currency?: string; onClose: () => void; onGo: (extra: Record<string, unknown>) => void }) {
  const [choices, setChoices] = useState<{ value: string; label: string; hint?: string }[] | null>(null)
  const [pick, setPick] = useState('')
  const [sum, setSum] = useState(amount ? (amount / 100).toFixed(2) : '')
  const [err, setErr] = useState('')
  useEffect(() => {
    const url = target === 'payment' ? `${V1}/make/accounts` : target === 'expense' ? `${V1}/make/groups` : `${V1}/make/chats`
    api.get<{ id: number | string; name: string; currency?: string; members?: string; everyday?: boolean; kind?: string }[]>(url).then(list => {
      setChoices(list.map(x => ({ value: String(x.id), label: x.name, hint: x.members || x.currency || (x.kind === 'self' ? 'only you' : undefined) })))
      const first = list.find(x => x.everyday) || list[0]
      if (first) setPick(String(first.id))
    }).catch(e => setErr(errMsg(e)))
  }, [target])
  const title = target === 'payment' ? 'Book the payment in Pocket Money' : target === 'expense' ? 'Share the cost in Pocket Split' : 'Tell the family chat'
  const go = () => {
    const minor = Math.round(parseFloat(sum.replace(',', '.')) * 100) || 0
    if (target === 'payment') onGo({ account_id: Number(pick), amount: minor, currency })
    else if (target === 'expense') onGo({ group_id: Number(pick), amount: minor, currency })
    else onGo({ to: pick })
  }
  return (
    <Modal title={title} onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!pick} onClick={go}>Go</button></>}>
      {err ? <div className="callout danger"><Icon name="alert" size={15} /><span>{err}</span></div> : !choices ? <p className="muted">Loading…</p> : (
        <div className="stack">
          <Field label={target === 'payment' ? 'Paid from' : target === 'expense' ? 'Split group' : 'Which chat'}>
            <Select ariaLabel="Choose" value={pick} onChange={setPick} options={choices} />
          </Field>
          {target !== 'chat' && <Field label={`Amount${currency ? ` (${currency})` : ''}`}><input inputMode="decimal" value={sum} onChange={e => setSum(e.target.value)} placeholder="0.00" /></Field>}
          {target === 'expense' && <p className="muted small">You paid it; it is split evenly between everyone in the group.</p>}
        </div>
      )}
    </Modal>
  )
}
