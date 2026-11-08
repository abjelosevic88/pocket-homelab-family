// The board: short notes for everyone (the fridge door), who has seen them.
import { useCallback, useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { ago, fmtDate } from '../lib/format'
import { announceChange, canEdit, useFamily, useOnChange } from '../lib/family'
import type { Post } from '../lib/types'
import { PostModal } from '../components/Forms'
import { Face } from '../components/People'
import { Icon } from '../components/icons'
import { Confirm, Empty, Loading, MenuButton, type MenuItem, errMsg, useToast } from '../components/ui'

export default function Board() {
  const { family, me, person, name, people } = useFamily()
  const { toast } = useToast()
  const [posts, setPosts] = useState<Post[] | null>(null)
  const [edit, setEdit] = useState<Post | 'new' | null>(null)
  const [del, setDel] = useState<Post | null>(null)
  const load = useCallback(() => { api.get<Post[]>(`${V1}/posts`).then(setPosts).catch(() => setPosts([])) }, [])
  useEffect(() => { load() }, [load, family?.id])
  useOnChange(load)
  // seen once looked at
  useEffect(() => { if (posts?.some(p => !p.seen)) { const t = setTimeout(() => api.post(`${V1}/posts/seen`).then(() => announceChange()).catch(() => {}), 1500); return () => clearTimeout(t) } }, [posts])
  if (!posts || !me) return <Loading />
  const accounts = people.filter(p => p.user_id > 0 && !p.archived)
  const pin = async (p: Post) => { try { await api.put(`${V1}/posts/${p.id}`, { pinned: !p.pinned }); load() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="board">
      <header className="page-head">
        <div><h2>The board</h2><p className="muted">Notes for everyone — like the fridge door. You see who has read them.</p></div>
        <button type="button" className="btn primary" onClick={() => setEdit('new')}><Icon name="pin" size={15} />A note</button>
      </header>
      {posts.length === 0 ? <Empty icon="pin" title="Nothing on the board">“Grandma comes on Sunday”, “the car is at the garage on Thursday” — put it up once, everyone sees it.</Empty> : (
        <div className="note-grid">
          {posts.map(p => {
            const author = person(p.person_id)
            const mine = p.person_id === me.id
            const seenBy = accounts.filter(a => a.id !== p.person_id && p.seen_by.includes(a.id))
            const notYet = accounts.filter(a => a.id !== p.person_id && !p.seen_by.includes(a.id))
            const menu = (): MenuItem[] => [
              { label: p.pinned ? 'Unpin' : 'Pin to the top', icon: 'pin', onSelect: () => pin(p) },
              ...(mine || canEdit(me) ? [{ label: 'Change', icon: 'edit', onSelect: () => setEdit(p) }, { label: 'Take it down', icon: 'trash', danger: true, onSelect: () => setDel(p) }] : []),
            ]
            return (
              <article key={p.id} className={`note-card ${p.pinned ? 'pinned' : ''} ${p.seen ? '' : 'unseen'}`} style={{ borderTopColor: author?.color }}>
                <header><Face person={author} size={26} /><span className="grow"><b>{name(p.person_id) || 'Someone'}</b><span className="muted small"> · {ago(p.created_at)}{p.until && ` · until ${fmtDate(p.until, false)}`}</span></span>{p.pinned && <Icon name="pin" size={14} className="accent-text" />}<MenuButton label="More" items={menu} align="right" className="btn sm ghost icon-only"><Icon name="more" size={16} /></MenuButton></header>
                <p className="note-body">{p.body}</p>
                {accounts.length > 1 && <footer className="muted small">{seenBy.length ? <><Icon name="check-double" size={13} />Seen by {seenBy.map(a => a.name.split(' ')[0]).join(', ')}</> : 'Not seen yet'}{notYet.length > 0 && seenBy.length > 0 && <span> · not yet: {notYet.map(a => a.name.split(' ')[0]).join(', ')}</span>}</footer>}
              </article>
            )
          })}
        </div>
      )}
      {edit && <PostModal post={edit === 'new' ? undefined : edit} onClose={() => { setEdit(null); load() }} />}
      {del && <Confirm title="Take this note down?" message="It goes for everyone." confirmLabel="Take down" onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/posts/${del.id}`); setDel(null); load(); announceChange() }} />}
    </div>
  )
}
