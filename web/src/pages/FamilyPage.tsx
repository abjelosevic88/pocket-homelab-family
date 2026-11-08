// Family: the people (with or without an account), what everyone should
// know (who to call, allergies, the doctor, the wifi) and the sitter sheet.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { fmtDate } from '../lib/format'
import { INFO_SECTIONS, ROLE_WORDS, announceChange, canEdit, useFamily, useOnChange } from '../lib/family'
import type { Info, Person } from '../lib/types'
import { Panel } from '../components/Bits'
import { InfoModal, PersonModal } from '../components/Forms'
import { useCrumb } from '../components/Layout'
import { MakeMenu } from '../components/MakeMenu'
import { Face } from '../components/People'
import { Icon } from '../components/icons'
import { Confirm, Field, Loading, Modal, errMsg, useToast } from '../components/ui'

export default function FamilyPage() {
  const { family, me, people, name, reload } = useFamily()
  const { toast } = useToast()
  const [info, setInfo] = useState<Info[] | null>(null)
  const [person, setPerson] = useState<Partial<Person> | null>(null)
  const [edit, setEdit] = useState<Partial<Info> | null>(null)
  const [famEdit, setFamEdit] = useState(false)
  const [leave, setLeave] = useState(false)
  const load = useCallback(() => { api.get<Info[]>(`${V1}/info`).then(setInfo).catch(() => setInfo([])) }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  if (!family || !me || !info) return <Loading />
  const grown = canEdit(me)
  const groups = INFO_SECTIONS.map(s => ({ ...s, items: info.filter(i => i.section === s.value) })).filter(g => g.items.length)
  return (
    <div className="family-page">
      <header className="page-head">
        <div><h2>{family.emoji} {family.name}</h2><p className="muted">{people.filter(p => !p.archived).length} people · {people.filter(p => p.user_id > 0).length} with their own account</p></div>
        <div className="row wrap">
          <Link to="/family/sheet" className="btn"><Icon name="clipboard" size={15} />Sitter sheet</Link>
          {me.manager && <button type="button" className="btn" onClick={() => setFamEdit(true)}><Icon name="edit" size={15} />Name</button>}
          {me.manager && <button type="button" className="btn primary" onClick={() => setPerson({})}><Icon name="plus" size={15} />Someone</button>}
        </div>
      </header>
      <div className="people-grid">
        {people.map(p => (
          <button key={p.id} type="button" className={`person-card ${p.archived ? 'is-off' : ''}`} onClick={() => (me.manager || p.id === me.id) && setPerson(p)}>
            <Face person={p} size={44} />
            <span className="grow">
              <span className="person-name">{p.name}{p.id === me.id && <span className="badge">you</span>}{p.manager && <span className="badge accent" title="Looks after the family settings">manages</span>}</span>
              <span className="muted small">{ROLE_WORDS[p.role]}{p.birthday && ` · 🎂 ${fmtDate(p.birthday.length === 10 ? p.birthday : '2000' + p.birthday.slice(1), false)}`}{p.class_name && ` · ${p.class_name}`}</span>
              <span className="small">{p.user_id > 0 ? <span className="ok">has an account</span> : p.waiting ? <span className="warn-text">waiting for {p.email} to sign in</span> : <span className="muted">no account</span>}{p.phone && <span className="muted"> · {p.phone}</span>}</span>
              {p.notes && <span className="person-notes small">{p.notes}</span>}
            </span>
          </button>
        ))}
      </div>
      <Panel title="What everyone should know" icon="info" className="info-panel" actions={grown && <button type="button" className="btn sm ghost" onClick={() => setEdit({})}><Icon name="plus" size={14} />Add</button>}>
        <div id="info" />
        {groups.length === 0 ? <p className="muted small pad">Who to call in an emergency, allergies, the doctor, the school office, the wifi — for the family, grandparents and the sitter.</p> : groups.map(g => (
          <section key={g.value} className="info-group">
            <h4 className="sub-h"><Icon name={g.icon} size={14} />{g.label}</h4>
            {g.items.map(i => (
              <button key={i.id} type="button" className="info-row" onClick={() => grown && setEdit(i)} disabled={!grown}>
                <span className="info-label">{i.label}{i.person_id > 0 && <span className="muted"> · {name(i.person_id)}</span>}</span>
                <span className="info-value">{i.value}{i.phone && <a href={`tel:${i.phone.replace(/\s/g, '')}`} className="info-phone" onClick={e => e.stopPropagation()}><Icon name="phone" size={13} />{i.phone}</a>}</span>
                {!i.on_sheet && <span className="muted small" title="Not on the sitter sheet"><Icon name="eye-off" size={13} /></span>}
              </button>
            ))}
          </section>
        ))}
      </Panel>
      <div className="row wrap mt">
        <span className="spacer" />
        <button type="button" className="btn ghost danger-text" onClick={() => setLeave(true)}>Leave this family</button>
      </div>
      {person && <PersonModal person={person} onClose={() => { setPerson(null); reload() }} />}
      {edit && <InfoModal info={edit} onClose={() => { setEdit(null); load() }} />}
      {famEdit && <FamilyModal onClose={() => setFamEdit(false)} />}
      {leave && <Confirm title={`Leave ${family.name}?`} message="You stop seeing it; someone who looks after it can add you again by email." confirmLabel="Leave"
        onCancel={() => setLeave(false)} onConfirm={async () => { try { await api.post(`${V1}/family/leave`); setLeave(false); await reload(); announceChange() } catch (e) { setLeave(false); toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

function FamilyModal({ onClose }: { onClose: () => void }) {
  const { family, reload } = useFamily()
  const { toast } = useToast()
  const [f, setF] = useState({ name: family?.name || '', emoji: family?.emoji || '🏡' })
  const [del, setDel] = useState(false)
  const save = async () => { try { await api.put(`${V1}/family`, f); await reload(); toast('Saved'); onClose() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal size="sm" title="The family" onClose={onClose} footer={<><button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete…</button><span className="spacer" /><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Emoji"><input className="emoji-input" value={f.emoji} onChange={e => setF({ ...f, emoji: [...e.target.value].slice(-2).join('') })} /></Field>
      </div>
      {del && <Confirm title={`Delete ${family?.name}?`} message="Everything in it goes for everyone: pick-ups, chores, lists, letters, the board and the info. This cannot be undone." confirmLabel="Delete for everyone"
        onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/family`); setDel(false); await reload(); onClose() }} />}
    </Modal>
  )
}

/** The sitter sheet: people, who to call, health, the house — to print or save. */
export function Sheet() {
  const { family, people, name } = useFamily()
  const { settings } = useApp()
  const [info, setInfo] = useState<Info[] | null>(null)
  useCrumb('Sitter sheet')
  useEffect(() => { api.get<Info[]>(`${V1}/info`).then(setInfo).catch(() => setInfo([])) }, [family?.id])
  if (!family || !info) return <Loading />
  const sheet = info.filter(i => i.on_sheet)
  return (
    <div className="sheet">
      <div className="row wrap no-print">
        <Link to="/family" className="back-link"><Icon name="chevron-left" size={15} />Family</Link>
        <span className="spacer" />
        <button type="button" className="btn" onClick={() => window.print()}><Icon name="file" size={15} />Print</button>
        <MakeMenu owner="sheet" ownerID={0} targets={['note']} label="Save" />
      </div>
      <h1 className="sheet-title">{family.emoji} {family.name}</h1>
      <section className="sheet-sec">
        <h2>People</h2>
        <table className="sheet-table"><tbody>
          {people.filter(p => !p.archived).map(p => <tr key={p.id}><th>{p.name}</th><td>{ROLE_WORDS[p.role]}{p.phone && ` · ${p.phone}`}{p.notes && <div className="sheet-note">{p.notes}</div>}</td></tr>)}
        </tbody></table>
      </section>
      {INFO_SECTIONS.map(s => {
        const items = sheet.filter(i => i.section === s.value)
        if (!items.length) return null
        return (
          <section key={s.value} className="sheet-sec">
            <h2>{s.label}</h2>
            <table className="sheet-table"><tbody>
              {items.map(i => <tr key={i.id}><th>{i.label}{i.person_id > 0 && <span className="muted"> ({name(i.person_id)})</span>}</th><td>{i.value}{i.phone && <b> · {i.phone}</b>}</td></tr>)}
            </tbody></table>
          </section>
        )
      })}
      <p className="muted small">From {settings?.title || 'Pocket Family'} · {new Date().toLocaleDateString()}</p>
    </div>
  )
}
