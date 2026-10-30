import { useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import { useList } from '../../lib/use-list'
import { ADMIN_CLIENT_ID } from '../../lib/config'
import type { AuthClient } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'

const GRANT_OPTIONS = [
  { value: 'authorization_code', label: 'Authorization code' },
  { value: 'refresh_token', label: 'Refresh token' },
  { value: 'client_credentials', label: 'Client credentials' },
]

const DEFAULT_GRANTS = ['authorization_code', 'refresh_token']

const loadItems = () => api.getClients()

export default function ClientsPage() {
  const { data: clients, loading, error, setError, reload } = useList(loadItems)

  const [showForm, setShowForm] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [name, setName] = useState('')
  const [redirectURIsText, setRedirectURIsText] = useState('')
  const [scopesText, setScopesText] = useState('profile')
  const [grants, setGrants] = useState<string[]>(DEFAULT_GRANTS)
  const [isPublic, setIsPublic] = useState(false)
  const [tokenType, setTokenType] = useState(1)
  const [secret, setSecret] = useState('')
  const [createdSecret, setCreatedSecret] = useState<string | null>(null)

  const resetForm = () => {
    setName('')
    setRedirectURIsText('')
    setScopesText('profile')
    setGrants(DEFAULT_GRANTS)
    setIsPublic(false)
    setTokenType(1)
    setSecret('')
  }

  const toggleGrant = (grant: string) => {
    setGrants((current) =>
      current.includes(grant) ? current.filter((item) => item !== grant) : [...current, grant],
    )
  }

  const handleCreate = async (event: FormEvent) => {
    event.preventDefault()
    const redirectURIs = redirectURIsText
      .split(/\r?\n/)
      .map((value) => value.trim())
      .filter(Boolean)
    const scopes = scopesText
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean)

    if (redirectURIs.length === 0) {
      setError('At least one redirect URI is required.')
      return
    }
    if (scopes.length === 0) {
      setError('At least one scope is required.')
      return
    }
    if (grants.length === 0) {
      setError('At least one grant type is required.')
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      const result = await api.createClient({
        display_name: name.trim(),
        domain: redirectURIs[0],
        secret: isPublic ? '' : secret,
        scopes,
        token_type: tokenType,
        is_public: isPublic,
        redirect_uris: redirectURIs,
        grant_types: grants,
      })
      setCreatedSecret(result.secret || null)
      setShowForm(false)
      resetForm()
      reload()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (client: AuthClient) => {
    if (client.id === Number(ADMIN_CLIENT_ID)) {
      setError('The management client cannot be deleted.')
      return
    }
    if (!window.confirm(`Delete client ${client.display_name || client.id}?`)) {
      return
    }
    setError(null)
    try {
      await api.deleteClient(client.id)
      reload()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Applications"
        title="OAuth2 clients"
        description="Register clients and manage redirect URIs, scopes and grant types. Secrets are shown once at creation time."
        actions={
          <button
            className={`btn ${showForm ? 'btn-ghost' : 'btn-primary'}`}
            onClick={() => setShowForm((open) => !open)}
          >
            {showForm ? 'Cancel' : 'New client'}
          </button>
        }
      />

      {error && <ErrorAlert message={error} onRetry={reload} />}

      {createdSecret && (
        <div className="alert alert-success items-start" role="status">
          <svg
            aria-hidden="true"
            className="mt-0.5 h-5 w-5 shrink-0"
            fill="none"
            stroke="currentColor"
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="1.8"
            viewBox="0 0 24 24"
          >
            <path d="m5 12 4 4L19 6" />
          </svg>
          <div className="min-w-0 flex-1">
            <p className="font-semibold">Client secret created</p>
            <p className="mt-0.5 text-sm opacity-80">Copy it now. It will not be shown again.</p>
            <code className="mt-2 block break-all rounded-lg bg-base-100/50 p-2 text-sm">
              {createdSecret}
            </code>
          </div>
          <div className="flex flex-wrap gap-1">
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => {
                void navigator.clipboard.writeText(createdSecret).catch(() => undefined)
              }}
            >
              Copy
            </button>
            <button className="btn btn-ghost btn-sm" onClick={() => setCreatedSecret(null)}>
              Dismiss
            </button>
          </div>
        </div>
      )}

      {showForm && (
        <section className="card border border-base-300 bg-base-100 shadow-sm">
          <div className="card-body">
            <div>
              <h2 className="card-title text-base">Register client</h2>
              <p className="text-sm text-base-content/50">
                Redirect URIs use exact string matching. Leave the secret blank to generate one.
              </p>
            </div>

            <form onSubmit={handleCreate} className="grid gap-4 md:grid-cols-2">
              <label className="form-control w-full md:col-span-2">
                <span className="label-text mb-1.5 font-medium">Display name</span>
                <input
                  className="input input-bordered w-full"
                  placeholder="My application"
                  required
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </label>

              <label className="form-control w-full md:col-span-2">
                <span className="label-text mb-1.5 font-medium">
                  Redirect URIs <span className="font-normal text-base-content/40">(one per line)</span>
                </span>
                <textarea
                  className="textarea textarea-bordered w-full font-mono text-sm"
                  rows={3}
                  required
                  placeholder="http://localhost:5173/admin-page/callback"
                  value={redirectURIsText}
                  onChange={(event) => setRedirectURIsText(event.target.value)}
                />
              </label>

              <label className="form-control w-full md:col-span-2">
                <span className="label-text mb-1.5 font-medium">
                  Allowed scopes <span className="font-normal text-base-content/40">(comma or space separated)</span>
                </span>
                <input
                  className="input input-bordered w-full font-mono text-sm"
                  required
                  value={scopesText}
                  onChange={(event) => setScopesText(event.target.value)}
                />
              </label>

              <fieldset className="md:col-span-2">
                <legend className="label-text mb-2 font-medium">Grant types</legend>
                <div className="grid gap-2 rounded-2xl border border-base-300 bg-base-200/40 p-3 sm:grid-cols-2">
                  {GRANT_OPTIONS.map((option) => (
                    <label
                      key={option.value}
                      className="flex cursor-pointer items-center gap-2 rounded-xl bg-base-100 px-3 py-2 text-sm"
                    >
                      <input
                        type="checkbox"
                        className="checkbox checkbox-sm"
                        checked={grants.includes(option.value)}
                        disabled={isPublic && option.value === 'client_credentials'}
                        onChange={() => toggleGrant(option.value)}
                      />
                      {option.label}
                    </label>
                  ))}
                </div>
              </fieldset>

              <label className="flex cursor-pointer items-start gap-3 rounded-2xl border border-base-300 bg-base-200/40 p-4 md:col-span-2">
                <input
                  type="checkbox"
                  className="checkbox checkbox-sm mt-0.5"
                  checked={isPublic}
                  onChange={(event) => {
                    const next = event.target.checked
                    setIsPublic(next)
                    if (next) {
                      setGrants((current) => current.filter((grant) => grant !== 'client_credentials'))
                      setSecret('')
                    }
                  }}
                />
                <span>
                  <span className="block text-sm font-medium">Public client</span>
                  <span className="mt-0.5 block text-xs leading-5 text-base-content/50">
                    PKCE only, with no client secret. Client credentials are disabled.
                  </span>
                </span>
              </label>

              <label className="form-control w-full">
                <span className="label-text mb-1.5 font-medium">Token type</span>
                <select
                  className="select select-bordered w-full"
                  value={tokenType}
                  onChange={(event) => setTokenType(Number(event.target.value))}
                >
                  <option value={0}>Opaque</option>
                  <option value={1}>JWT</option>
                </select>
              </label>
              <label className="form-control w-full">
                <span className="label-text mb-1.5 font-medium">Client secret</span>
                <input
                  className="input input-bordered w-full"
                  disabled={isPublic}
                  placeholder={isPublic ? 'Not used for public clients' : 'Leave blank to generate'}
                  value={secret}
                  onChange={(event) => setSecret(event.target.value)}
                />
              </label>

              <div className="flex justify-end gap-2 md:col-span-2">
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => setShowForm(false)}
                >
                  Cancel
                </button>
                <button className="btn btn-primary" type="submit" disabled={submitting}>
                  {submitting ? 'Creating...' : 'Create client'}
                </button>
              </div>
            </form>
          </div>
        </section>
      )}

      <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <div>
            <h2 className="font-semibold">Registered clients</h2>
            <p className="text-sm text-base-content/50">Applications allowed to use this OAuth2 server.</p>
          </div>
          <span className="badge badge-ghost badge-sm">{clients.length} total</span>
        </div>

        {loading ? (
          <LoadingBlock label="Loading clients..." />
        ) : clients.length === 0 ? (
          <div className="p-5">
            <EmptyState
              title="No clients"
              description="Register a client to allow an application to use this server."
            />
          </div>
        ) : (
          <>
            <div className="divide-y divide-base-300 md:hidden">
              {clients.map((client) => {
                const isManagementClient = client.id === Number(ADMIN_CLIENT_ID)
                const redirectURIs =
                  client.redirect_uris?.length > 0
                    ? client.redirect_uris
                    : [client.domain].filter(Boolean)

                return (
                  <div
                    key={client.id}
                    className={`space-y-3 p-4 ${isManagementClient ? 'bg-primary/5' : ''}`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-medium">{client.display_name || '—'}</p>
                        {isManagementClient && (
                          <span className="badge badge-primary badge-xs mt-1">Management</span>
                        )}
                      </div>
                      <div className="flex shrink-0 flex-wrap justify-end gap-1">
                        <span
                          className={`badge badge-xs ${
                            client.is_public ? 'badge-success' : 'badge-neutral'
                          }`}
                        >
                          {client.is_public ? 'Public' : 'Confidential'}
                        </span>
                        <span className="badge badge-ghost badge-xs">
                          {client.token_type === 1 ? 'JWT' : 'Opaque'}
                        </span>
                      </div>
                    </div>

                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-wide text-base-content/40">
                        Redirect URIs
                      </p>
                      <ul className="mt-1 space-y-1">
                        {redirectURIs.map((uri) => (
                          <li key={uri}>
                            <code className="break-all text-xs text-base-content/65">{uri}</code>
                          </li>
                        ))}
                      </ul>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <p className="text-[10px] font-semibold uppercase tracking-wide text-base-content/40">
                          Scopes
                        </p>
                        <div className="mt-1 flex flex-wrap gap-1">
                          {(client.scopes || []).map((scope) => (
                            <span key={scope} className="badge badge-outline badge-xs font-mono">
                              {scope}
                            </span>
                          ))}
                        </div>
                      </div>
                      <div>
                        <p className="text-[10px] font-semibold uppercase tracking-wide text-base-content/40">
                          Grants
                        </p>
                        <p className="mt-1 break-words text-xs leading-5 text-base-content/60">
                          {(client.grant_types || []).join(', ')}
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center justify-between border-t border-base-300/70 pt-3">
                      <span className="font-mono text-xs text-base-content/40">ID {client.id}</span>
                      {isManagementClient ? (
                        <span className="badge badge-ghost badge-xs">Protected</span>
                      ) : (
                        <button
                          className="btn btn-ghost btn-xs text-error"
                          onClick={() => handleDelete(client)}
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>

            <div className="hidden overflow-x-auto md:block">
            <table className="table min-w-[1100px]">
              <thead className="bg-base-200/50 text-[11px] uppercase tracking-wide text-base-content/50">
                <tr>
                  <th>ID</th>
                  <th>Client</th>
                  <th>Redirect URIs</th>
                  <th>Scopes</th>
                  <th>Grants</th>
                  <th>Type</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {clients.map((client) => {
                  const isManagementClient = client.id === Number(ADMIN_CLIENT_ID)
                  const redirectURIs =
                    client.redirect_uris?.length > 0
                      ? client.redirect_uris
                      : [client.domain].filter(Boolean)

                  return (
                    <tr
                      key={client.id}
                      className={isManagementClient ? 'bg-primary/5' : 'hover'}
                    >
                      <td className="font-mono text-xs text-base-content/50">{client.id}</td>
                      <td>
                        <div className="font-medium">{client.display_name || '—'}</div>
                        {isManagementClient && (
                          <span className="badge badge-primary badge-xs mt-1">Management</span>
                        )}
                      </td>
                      <td>
                        <ul className="space-y-1 text-xs">
                          {redirectURIs.map((uri) => (
                            <li key={uri}>
                              <code className="whitespace-nowrap text-base-content/65">{uri}</code>
                            </li>
                          ))}
                        </ul>
                      </td>
                      <td>
                        <div className="flex flex-wrap gap-1">
                          {(client.scopes || []).map((scope) => (
                            <span key={scope} className="badge badge-outline badge-xs font-mono">
                              {scope}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td>
                        <div className="flex flex-wrap gap-1">
                          {(client.grant_types || []).map((grant) => (
                            <span key={grant} className="badge badge-ghost badge-xs">
                              {grant}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td>
                        <div className="flex flex-wrap gap-1">
                          <span
                            className={`badge badge-xs ${
                              client.is_public ? 'badge-success' : 'badge-neutral'
                            }`}
                          >
                            {client.is_public ? 'Public' : 'Confidential'}
                          </span>
                          <span className="badge badge-ghost badge-xs">
                            {client.token_type === 1 ? 'JWT' : 'Opaque'}
                          </span>
                        </div>
                      </td>
                      <td>
                        <div className="flex justify-end">
                          {isManagementClient ? (
                            <span className="badge badge-ghost badge-xs">Protected</span>
                          ) : (
                            <button
                              className="btn btn-ghost btn-xs text-error"
                              title="Delete client"
                              onClick={() => handleDelete(client)}
                            >
                              Delete
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
            </div>
          </>
        )}
      </section>
    </div>
  )
}
