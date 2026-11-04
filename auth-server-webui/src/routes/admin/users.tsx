import { useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import { useList } from '../../lib/use-list'
import type { ListQuery, User } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'
import { ListControls } from '../../components/ListControls'

const loadItems = (query: ListQuery) => api.getUsers(query)
type Editor = { mode: 'create' } | { mode: 'edit' | 'password'; user: User }

export default function UsersPage() {
  const list = useList(loadItems)
  const { data: users, loading, error, setError, reload } = list
  const [editor, setEditor] = useState<Editor | null>(null)
  const [busy, setBusy] = useState(false)
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [success, setSuccess] = useState<string | null>(null)

  const open = (next: Editor) => {
    setEditor(next)
    setEmail(next.mode === 'create' ? '' : next.user.email)
    setDisplayName(next.mode === 'create' ? '' : next.user.display_name)
    setPassword('')
    setError(null)
    setSuccess(null)
  }
  const run = async (action: () => Promise<void>, message: string) => {
    if (busy) return
    setBusy(true); setError(null); setSuccess(null)
    try {
      await action()
      setEditor(null); setPassword(''); setSuccess(message); reload()
    } catch (err) { setError(errorMessage(err)) }
    finally { setBusy(false) }
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!editor) return
    if (editor.mode === 'create') {
      void run(() => api.createUser(email.trim(), displayName.trim(), password), 'User created.')
    } else if (editor.mode === 'edit') {
      if (email.trim().toLowerCase() !== editor.user.email && !window.confirm('Changing the email ends this user’s existing grants and SSO sessions. Continue?')) return
      void run(() => api.updateUser(editor.user.id, { email, display_name: displayName }), 'User updated.')
    } else {
      if (!window.confirm('Reset the password and end all existing grants and SSO sessions for this user?')) return
      void run(() => api.resetUserPassword(editor.user.id, password), 'Password reset. Existing grants and sessions have been invalidated.')
    }
  }
  const toggle = (user: User) => {
    if (!window.confirm(`${user.is_disabled ? 'Enable' : 'Disable'} ${user.email}? Existing grants and SSO sessions will remain invalid after re-enabling.`)) return
    void run(() => api.updateUser(user.id, { is_disabled: !user.is_disabled }), 'Account status updated.')
  }

  return <div className="space-y-5">
    <PageHeader eyebrow="Identity" title="Users" description="Manage account details, sign-in access and passwords. Security changes invalidate existing grants and SSO sessions."
      actions={<button className="btn btn-primary" disabled={busy} onClick={() => open({ mode: 'create' })}>New user</button>} />
    <ListControls {...list} />
    {error && <ErrorAlert message={error} />}
    {success && <div className="alert alert-success" role="status">{success}</div>}
    {editor && <section className="card border border-base-300 bg-base-100 shadow-sm"><div className="card-body">
      <h2 className="card-title text-base">{editor.mode === 'create' ? 'Create user' : editor.mode === 'edit' ? 'Edit user' : 'Reset password'}</h2>
      {editor.mode === 'password' && <p className="text-sm">{editor.user.email}. Only accounts with existing email/password sign-in can be reset.</p>}
      <form onSubmit={submit} className="grid gap-4 md:grid-cols-2">
        {editor.mode !== 'password' && <>
          <label className="flex flex-col gap-1 text-sm">Email
            <input className="input w-full" type="email" required maxLength={254} autoComplete="off" value={email} onChange={(event) => setEmail(event.target.value)} />
          </label>
          <label className="flex flex-col gap-1 text-sm">Display name
            <input className="input w-full" maxLength={200} value={displayName} onChange={(event) => setDisplayName(event.target.value)} />
          </label>
        </>}
        {editor.mode !== 'edit' && <label className="flex flex-col gap-1 text-sm md:col-span-2">Password
          <input aria-label="Password" aria-describedby="password-help" className="input w-full" type="password" required minLength={12} autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} />
          <span id="password-help" className="text-xs text-base-content/60">At least 12 characters.</span>
        </label>}
        <div className="flex justify-end gap-2 md:col-span-2">
          <button type="button" className="btn btn-ghost" disabled={busy} onClick={() => { setEditor(null); setPassword('') }}>Cancel</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? 'Saving...' : editor.mode === 'create' ? 'Create user' : editor.mode === 'edit' ? 'Save user' : 'Reset password'}</button>
        </div>
      </form>
    </div></section>}
    <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
      <div className="border-b border-base-300 p-4"><h2 className="font-semibold">User accounts</h2></div>
      {loading ? <LoadingBlock label="Loading users..." /> : users.length === 0 ? <EmptyState title="No users" description="No accounts match these filters." /> :
        <div className="overflow-x-auto"><table className="table">
          <thead><tr><th className="hidden lg:table-cell">ID</th><th>Email</th><th className="hidden md:table-cell">Display name</th><th>Status</th><th className="hidden md:table-cell">Last login</th><th>Actions</th></tr></thead>
          <tbody>{users.map((user) => <tr key={user.id}>
            <td className="hidden font-mono text-xs lg:table-cell">{user.id}</td>
            <td className="break-all font-medium">{user.email}</td>
            <td className="hidden md:table-cell">{user.display_name || '—'}</td>
            <td><span className={`badge badge-sm ${user.is_disabled ? 'badge-warning' : 'badge-success'}`}>{user.is_disabled ? 'Disabled' : 'Active'}</span></td>
            <td className="hidden md:table-cell">{user.last_login_at || '—'}</td>
            <td><div className="flex flex-wrap gap-1">
              <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => open({ mode: 'edit', user })}>Edit</button>
              <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => toggle(user)}>{user.is_disabled ? 'Enable' : 'Disable'}</button>
              <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => open({ mode: 'password', user })}>Reset password</button>
              <button className="btn btn-ghost btn-xs text-error" disabled={busy} onClick={() => {
                if (window.confirm(`Delete ${user.email}? This cannot be undone.`)) void run(() => api.deleteUser(user.id), 'User deleted.')
              }}>Delete</button>
            </div></td>
          </tr>)}</tbody>
        </table></div>}
    </section>
  </div>
}
