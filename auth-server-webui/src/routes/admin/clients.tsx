import { useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import { useList } from '../../lib/use-list'
import { ADMIN_CLIENT_ID } from '../../lib/config'
import type { AuthClient, CreateClientPayload, ListQuery } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'
import { ListControls } from '../../components/ListControls'

const GRANTS = ['authorization_code', 'refresh_token', 'client_credentials']
const loadItems = (query: ListQuery) => api.getClients(query)
const blank = (): CreateClientPayload => ({ display_name: '', domain: '', secret: '', scopes: ['profile'], token_type: 1, is_public: false, redirect_uris: [], grant_types: ['authorization_code', 'refresh_token'] })

export default function ClientsPage() {
  const list = useList(loadItems)
  const { data: clients, loading, error, setError, reload } = list
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing] = useState<AuthClient | null>(null)
  const [form, setForm] = useState(blank)
  const [uris, setURIs] = useState('')
  const [scopes, setScopes] = useState('profile')
  const [busy, setBusy] = useState(false)
  const [createdSecret, setCreatedSecret] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const open = (client: AuthClient | null) => {
    setEditing(client); setError(null); setSuccess(null); setCreatedSecret(null)
    setForm(client ? { ...client, secret: '', token_type: client.token_type ?? 0 } : blank())
    setURIs(client ? (client.redirect_uris.length ? client.redirect_uris : [client.domain]).join('\n') : '')
    setScopes(client ? client.scopes.join(' ') : 'profile')
    setShowForm(true)
  }
  const run = async (action: () => Promise<void>, message: string) => {
    if (busy) return
    setBusy(true); setError(null); setSuccess(null); setCreatedSecret(null)
    try { await action(); setShowForm(false); setSuccess(message); reload() }
    catch (err) { setError(errorMessage(err)) }
    finally { setBusy(false) }
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const redirect_uris = uris.split(/\r?\n/).map((value) => value.trim()).filter(Boolean)
    const allowed = scopes.split(/[\s,]+/).filter(Boolean)
    if (!redirect_uris.length || !allowed.length || !form.grant_types.length) { setError('At least one redirect URI, scope and grant type is required.'); return }
    if (editing && !window.confirm('Save client changes and invalidate all existing grants for this client?')) return
    const data = { ...form, display_name: form.display_name.trim(), redirect_uris, domain: redirect_uris[0], scopes: allowed }
    void run(async () => {
      if (editing) await api.updateClient(editing.id, data)
      else { const result = await api.createClient(data); setCreatedSecret(result.secret || null) }
    }, editing ? 'Client updated. Existing grants have been invalidated.' : 'Client registered.')
  }
  const actions = (client: AuthClient) => {
    if (client.id === Number(ADMIN_CLIENT_ID)) return <span className="badge badge-primary badge-sm">Protected management client</span>
    return <div className="flex flex-wrap gap-1">
      <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => open(client)}>Edit</button>
      <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => {
        if (window.confirm(`${client.is_disabled ? 'Enable' : 'Disable'} ${client.display_name}? Old grants will remain invalid after re-enabling.`)) void run(() => api.setClientDisabled(client.id, !client.is_disabled), 'Client status updated.')
      }}>{client.is_disabled ? 'Enable' : 'Disable'}</button>
      {!client.is_public && <button className="btn btn-ghost btn-xs" disabled={busy} onClick={() => {
        if (window.confirm('Reset this client secret? Existing grants and the previous secret will stop working.')) void run(async () => { const result = await api.resetClientSecret(client.id); setCreatedSecret(result.secret) }, 'Secret reset. Copy the new secret now.')
      }}>Reset secret</button>}
      <button className="btn btn-ghost btn-xs text-error" disabled={busy} onClick={() => {
        if (window.confirm(`Delete ${client.display_name}? This cannot be undone.`)) void run(() => api.deleteClient(client.id), 'Client deleted.')
      }}>Delete</button>
    </div>
  }

  return <div className="space-y-5">
    <PageHeader eyebrow="Applications" title="OAuth2 clients" description="Edit client policies, control access and rotate secrets. Changes invalidate existing client grants."
      actions={<button className="btn btn-primary" disabled={busy} onClick={() => open(null)}>New client</button>} />
    <ListControls {...list} />
    {error && <ErrorAlert message={error} />}
    {success && <div className="alert alert-success" role="status">{success}</div>}
    {createdSecret && <div className="alert alert-info items-start" role="status"><div className="min-w-0 flex-1">
      <p className="font-semibold">Client secret</p><p>Copy it now. It will not be shown again.</p>
      <code className="mt-2 block break-all rounded bg-base-100 p-2 text-sm">{createdSecret}</code>
      <button className="btn btn-sm mt-2" onClick={() => { void navigator.clipboard.writeText(createdSecret).catch(() => setError('Clipboard is unavailable. Copy the secret manually.')) }}>Copy secret</button>
    </div><button className="btn btn-ghost btn-sm" onClick={() => setCreatedSecret(null)}>Dismiss</button></div>}
    {showForm && <section className="card border border-base-300 bg-base-100 shadow-sm"><div className="card-body">
      <h2 className="card-title text-base">{editing ? 'Edit client' : 'Register client'}</h2>
      <form onSubmit={submit} className="grid gap-4 md:grid-cols-2">
        <label className="flex flex-col gap-1 text-sm md:col-span-2">Display name<input className="input w-full" required maxLength={200} value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} /></label>
        <label className="flex flex-col gap-1 text-sm md:col-span-2">Redirect URIs (one per line)<textarea aria-label="Redirect URIs (one per line)" className="textarea w-full font-mono" rows={3} required value={uris} onChange={(event) => setURIs(event.target.value)} /></label>
        <label className="flex flex-col gap-1 text-sm md:col-span-2">Allowed scopes<input className="input w-full font-mono" required value={scopes} onChange={(event) => setScopes(event.target.value)} /></label>
        <fieldset className="md:col-span-2"><legend className="mb-2 text-sm">Grant types</legend><div className="flex flex-wrap gap-3">{GRANTS.map((grant) => <label key={grant} className="flex items-center gap-2 text-sm">
          <input className="checkbox checkbox-sm" type="checkbox" checked={form.grant_types.includes(grant)} disabled={form.is_public && grant === 'client_credentials'} onChange={() => setForm({ ...form, grant_types: form.grant_types.includes(grant) ? form.grant_types.filter((value) => value !== grant) : [...form.grant_types, grant] })} />{grant}
        </label>)}</div></fieldset>
        <label className="flex items-center gap-2 text-sm md:col-span-2"><input className="checkbox checkbox-sm" type="checkbox" checked={form.is_public} disabled={editing !== null} onChange={(event) => setForm({ ...form, is_public: event.target.checked, secret: '', grant_types: event.target.checked ? form.grant_types.filter((value) => value !== 'client_credentials') : form.grant_types })} />Public client (no secret). Client type is fixed after creation.</label>
        <label className="flex flex-col gap-1 text-sm">Token type<select aria-label="Token type" className="select w-full" value={form.token_type} onChange={(event) => setForm({ ...form, token_type: Number(event.target.value) })}><option value={0}>Opaque</option><option value={1}>JWT</option></select></label>
        {!editing && <label className="flex flex-col gap-1 text-sm">Client secret<input className="input w-full" disabled={form.is_public} autoComplete="off" value={form.secret} placeholder="Leave blank to generate" onChange={(event) => setForm({ ...form, secret: event.target.value })} /></label>}
        <div className="flex justify-end gap-2 md:col-span-2"><button className="btn btn-ghost" type="button" disabled={busy} onClick={() => setShowForm(false)}>Cancel</button><button className="btn btn-primary" disabled={busy}>{busy ? 'Saving...' : editing ? 'Save client' : 'Create client'}</button></div>
      </form>
    </div></section>}
    <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
      <div className="border-b border-base-300 p-4"><h2 className="font-semibold">Registered clients</h2></div>
      {loading ? <LoadingBlock label="Loading clients..." /> : clients.length === 0 ? <EmptyState title="No clients" description="No clients match these filters." /> : <>
        <div className="divide-y divide-base-300 md:hidden">{clients.map((client) => <article key={client.id} className="space-y-3 p-4">
          <h3 className="font-semibold">{client.display_name}</h3>
          <span className="badge badge-sm">{client.is_disabled ? 'Disabled' : 'Active'} · {client.is_public ? 'Public' : 'Confidential'}</span>
          <p className="break-all font-mono text-xs">{client.redirect_uris.join('\n')}</p><p className="break-words text-xs">{client.scopes.join(', ')} · {client.grant_types.join(', ')}</p>
          {actions(client)}
        </article>)}</div>
        <div className="hidden overflow-x-auto md:block"><table className="table"><thead><tr><th>ID</th><th>Client</th><th>Redirect URIs</th><th>Scopes / grants</th><th>Status / type</th><th>Actions</th></tr></thead><tbody>{clients.map((client) => <tr key={client.id}>
          <td className="font-mono text-xs">{client.id}</td><td className="font-medium">{client.display_name}</td>
          <td><ul>{client.redirect_uris.map((uri) => <li className="break-all font-mono text-xs" key={uri}>{uri}</li>)}</ul></td>
          <td className="text-xs">{client.scopes.join(', ')}<br />{client.grant_types.join(', ')}</td>
          <td>{client.is_disabled ? 'Disabled' : 'Active'}<br /><span className="text-xs">{client.is_public ? 'Public' : 'Confidential'} · {client.token_type === 1 ? 'JWT' : 'Opaque'}</span></td>
          <td>{actions(client)}</td>
        </tr>)}</tbody></table></div>
      </>}
    </section>
  </div>
}
