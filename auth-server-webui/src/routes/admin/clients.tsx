import { useEffect, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import { ADMIN_CLIENT_ID } from '../../lib/config'
import type { AuthClient } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'

const GRANT_OPTIONS = [
  { value: 'authorization_code', label: 'Authorization code' },
  { value: 'refresh_token', label: 'Refresh token' },
  { value: 'client_credentials', label: 'Client credentials' },
  { value: 'implicit', label: 'Implicit' },
]

const DEFAULT_GRANTS = ['authorization_code', 'refresh_token']

export default function ClientsPage() {
  const [clients, setClients] = useState<AuthClient[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

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

  useEffect(() => {
    let active = true
    api
      .getClients()
      .then((data) => {
        if (active) {
          setClients(data)
        }
      })
      .catch((err: unknown) => {
        if (active) {
          setError(errorMessage(err))
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false)
        }
      })
    return () => {
      active = false
    }
  }, [reloadKey])

  const reload = () => {
    setLoading(true)
    setError(null)
    setReloadKey((key) => key + 1)
  }

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
    <div>
      <div className="flex items-center justify-between mb-4 gap-4">
        <div>
          <h1 className="text-2xl font-bold">OAuth2 Clients</h1>
          <p className="text-sm text-base-content/60">
            Secrets are shown once when created. Editing and disabling are planned for Phase 3.
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowForm((open) => !open)}>
          {showForm ? 'Cancel' : 'New Client'}
        </button>
      </div>

      {error && <ErrorAlert message={error} onRetry={reload} />}

      {createdSecret && (
        <div className="alert alert-success mb-4 items-start">
          <div className="min-w-0">
            <p className="font-semibold">Client secret (shown once)</p>
            <code className="break-all text-sm">{createdSecret}</code>
          </div>
          <div className="flex gap-2">
            <button
              className="btn btn-sm"
              onClick={() => {
                void navigator.clipboard.writeText(createdSecret).catch(() => undefined)
              }}
            >
              Copy
            </button>
            <button className="btn btn-sm btn-ghost" onClick={() => setCreatedSecret(null)}>
              Dismiss
            </button>
          </div>
        </div>
      )}

      {showForm && (
        <form onSubmit={handleCreate} className="card bg-base-200 p-4 mb-4 space-y-3">
          <input
            className="input input-bordered w-full"
            placeholder="Display name"
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <label className="form-control">
            <span className="label-text mb-1">Redirect URIs (one per line, exact match)</span>
            <textarea
              className="textarea textarea-bordered w-full"
              rows={3}
              required
              placeholder="http://localhost:5173/admin-page/callback"
              value={redirectURIsText}
              onChange={(event) => setRedirectURIsText(event.target.value)}
            />
          </label>
          <label className="form-control">
            <span className="label-text mb-1">Allowed scopes (comma or space separated)</span>
            <input
              className="input input-bordered w-full"
              required
              value={scopesText}
              onChange={(event) => setScopesText(event.target.value)}
            />
          </label>

          <fieldset className="space-y-2">
            <legend className="label-text mb-1">Grant types</legend>
            <div className="flex flex-wrap gap-4">
              {GRANT_OPTIONS.map((option) => (
                <label key={option.value} className="label cursor-pointer gap-2">
                  <input
                    type="checkbox"
                    className="checkbox checkbox-sm"
                    checked={grants.includes(option.value)}
                    disabled={isPublic && option.value === 'client_credentials'}
                    onChange={() => toggleGrant(option.value)}
                  />
                  <span>{option.label}</span>
                </label>
              ))}
            </div>
          </fieldset>

          <label className="label cursor-pointer justify-start gap-2">
            <input
              type="checkbox"
              className="checkbox"
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
            <span>Public client (PKCE, no client secret)</span>
          </label>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <label className="form-control">
              <span className="label-text mb-1">Token type</span>
              <select
                className="select select-bordered"
                value={tokenType}
                onChange={(event) => setTokenType(Number(event.target.value))}
              >
                <option value={0}>Opaque</option>
                <option value={1}>JWT</option>
              </select>
            </label>
            <label className="form-control">
              <span className="label-text mb-1">Secret (blank = generate)</span>
              <input
                className="input input-bordered"
                disabled={isPublic}
                placeholder={isPublic ? 'Not used for public clients' : 'Leave blank to generate'}
                value={secret}
                onChange={(event) => setSecret(event.target.value)}
              />
            </label>
          </div>

          <button className="btn btn-primary" type="submit" disabled={submitting}>
            {submitting ? 'Creating...' : 'Create client'}
          </button>
        </form>
      )}

      {loading ? (
        <LoadingBlock />
      ) : clients.length === 0 ? (
        <EmptyState title="No clients" description="Register a client to allow an application to use this server." />
      ) : (
        <div className="overflow-x-auto">
          <table className="table table-zebra">
            <thead>
              <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Redirect URIs</th>
                <th>Scopes</th>
                <th>Grants</th>
                <th>Type</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {clients.map((client) => (
                <tr key={client.id}>
                  <td>{client.id}</td>
                  <td>{client.display_name || '-'}</td>
                  <td>
                    <ul className="text-xs space-y-1">
                      {(client.redirect_uris || []).map((uri) => (
                        <li key={uri}>
                          <code className="break-all">{uri}</code>
                        </li>
                      ))}
                    </ul>
                  </td>
                  <td>
                    <div className="flex flex-wrap gap-1">
                      {(client.scopes || []).map((scope) => (
                        <span key={scope} className="badge badge-outline badge-sm">
                          {scope}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="text-xs">{(client.grant_types || []).join(', ')}</td>
                  <td>
                    <span className={`badge ${client.is_public ? 'badge-success' : 'badge-neutral'}`}>
                      {client.is_public ? 'Public' : 'Confidential'}
                    </span>
                    <span className="badge badge-ghost ml-1">
                      {client.token_type === 1 ? 'JWT' : 'Opaque'}
                    </span>
                  </td>
                  <td>
                    <div className="flex flex-wrap gap-1">
                      <button
                        className="btn btn-error btn-xs"
                        disabled={client.id === Number(ADMIN_CLIENT_ID)}
                        title={
                          client.id === Number(ADMIN_CLIENT_ID)
                            ? 'The management client cannot be deleted'
                            : 'Delete client'
                        }
                        onClick={() => handleDelete(client)}
                      >
                        Delete
                      </button>
                      <button className="btn btn-xs" disabled title="Editing is planned for Phase 3">
                        Edit
                      </button>
                      <button className="btn btn-xs" disabled title="Secret rotation is planned for Phase 3">
                        Rotate secret
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
