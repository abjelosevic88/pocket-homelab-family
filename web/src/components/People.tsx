// People: a coloured face, a chip, a picker of several, a Select of one.
import { useFamily } from '../lib/family'
import type { Person } from '../lib/types'
import { Select } from './Select'

export function Face({ person, size = 26, title }: { person?: Person; size?: number; title?: string }) {
  if (!person) return <span className="face face-empty" style={{ width: size, height: size, fontSize: size * 0.45 }} title={title || 'Nobody yet'} aria-hidden="true">?</span>
  const initial = (person.name.trim()[0] || '?').toUpperCase()
  return (
    <span className="face" style={{ width: size, height: size, fontSize: person.emoji ? size * 0.55 : size * 0.42, background: person.color || 'var(--accent)' }} title={title || person.name} aria-hidden="true">
      {person.emoji || initial}
    </span>
  )
}

export function PersonChip({ id, small }: { id?: number; small?: boolean }) {
  const { person } = useFamily()
  const p = person(id)
  return (
    <span className={`pchip ${small ? 'sm' : ''} ${p ? '' : 'nobody'}`}>
      <Face person={p} size={small ? 18 : 22} />
      <span className="ellipsis">{p ? p.name.split(' ')[0] : 'Nobody yet'}</span>
    </span>
  )
}

export function Faces({ ids, size = 22, max = 4 }: { ids: number[]; size?: number; max?: number }) {
  const { person } = useFamily()
  const shown = ids.slice(0, max)
  return (
    <span className="faces" aria-label={ids.map(id => person(id)?.name).filter(Boolean).join(', ')}>
      {shown.map(id => <Face key={id} person={person(id)} size={size} />)}
      {ids.length > max && <span className="faces-more">+{ids.length - max}</span>}
    </span>
  )
}

/** Toggle several people on and off. */
export function PeoplePicker({ value, onChange, filter, label = 'People' }: { value: number[]; onChange: (v: number[]) => void; filter?: (p: Person) => boolean; label?: string }) {
  const { people } = useFamily()
  const list = people.filter(p => !p.archived && (!filter || filter(p)))
  const toggle = (id: number) => onChange(value.includes(id) ? value.filter(x => x !== id) : [...value, id])
  return (
    <div className="ppick" role="group" aria-label={label}>
      {list.map(p => (
        <button key={p.id} type="button" className={`ppick-btn ${value.includes(p.id) ? 'on' : ''}`} aria-pressed={value.includes(p.id)} onClick={() => toggle(p.id)}>
          <Face person={p} size={22} />{p.name.split(' ')[0]}
        </button>
      ))}
    </div>
  )
}

/** One person (or nobody). */
export function PersonSelect({ value, onChange, filter, nobody = 'Nobody yet', ariaLabel = 'Who' }: { value: number; onChange: (v: number) => void; filter?: (p: Person) => boolean; nobody?: string; ariaLabel?: string }) {
  const { people } = useFamily()
  const list = people.filter(p => !p.archived && (!filter || filter(p)))
  return (
    <Select ariaLabel={ariaLabel} value={String(value || 0)} onChange={v => onChange(Number(v))}
      options={[{ value: '0', label: nobody }, ...list.map(p => ({ value: String(p.id), label: p.name, hint: p.role === 'helper' ? 'helps out' : undefined }))]} />
  )
}

export const drivers = (p: Person) => p.role !== 'child'
export const children = (p: Person) => p.role === 'child'
