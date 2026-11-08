// First visit with no family yet: make one (or wait to be added).
import React, { useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { PALETTE, useFamily } from '../lib/family'
import { Icon } from '../components/icons'
import { Field, errMsg, useToast } from '../components/ui'

export default function Welcome() {
  const { user } = useApp()
  const { reload } = useFamily()
  const { toast } = useToast()
  const words = (user?.name || '').trim().split(/\s+/).filter(Boolean)
  const [name, setName] = useState(words.length > 1 ? `The ${words[words.length - 1]} family` : 'Our family')
  const [me, setMe] = useState(user?.name || '')
  const [emoji, setEmoji] = useState('🏡')
  const [busy, setBusy] = useState(false)
  const make = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try { await api.post(`${V1}/families`, { name, emoji, my_name: me, my_color: PALETTE[1] }); await reload(); toast('Your family is ready — add the others under Family') } catch (er) { toast(errMsg(er), 'err') } finally { setBusy(false) }
  }
  return (
    <div className="welcome">
      <img src="/favicon.svg" alt="" width={72} height={72} />
      <h1>Pocket Family</h1>
      <p className="muted">One place for the whole household: who picks up the children, the week, chores, shopping, letters from school and what the sitter needs to know.</p>
      <form className="welcome-card stack" onSubmit={make}>
        <h2>Start your family</h2>
        <Field label="The family's name"><input value={name} onChange={e => setName(e.target.value)} required autoFocus /></Field>
        <div className="emoji-row">{['🏡', '👨‍👩‍👧‍👦', '🌻', '🐻', '⭐', '🏠', '🌳', '❤️'].map(x => <button key={x} type="button" className={`emoji-btn ${emoji === x ? 'on' : ''}`} onClick={() => setEmoji(x)}>{x}</button>)}</div>
        <Field label="Your name, as the family sees it"><input value={me} onChange={e => setMe(e.target.value)} placeholder="Ana" /></Field>
        <button className="btn primary lg" disabled={busy}><Icon name="plus" size={16} />Make the family</button>
      </form>
      <p className="muted small welcome-note"><Icon name="info" size={14} /><span>Someone else already made yours? Ask them to add you under <b>Family</b> with your email (<b>{user?.email}</b>) — it shows up here by itself.</span></p>
    </div>
  )
}
