import { useEffect, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import type { User } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [showForm, setShowForm] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')

  useEffect(() => {
    let active = true
    api
      .getUsers()
      .then((data) => {
        if (active) {
          setUsers(data)
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

  const handleCreate = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await api.createUser(email.trim(), displayName.trim(), password)
      setShowForm(false)
      setEmail('')
      setDisplayName('')
      setPassword('')
      reload()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number, userEmail: string) => {
    if (!window.confirm(`Delete user ${userEmail}? This cannot be undone.`)) {
      return
    }
    setError(null)
    try {
      await api.deleteUser(id)
      reload()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Identity"
        title="Users"
        description="Create and remove user accounts. Editing, disabling and password reset are planned for Phase 3."
        actions={
          <button
            className={`btn ${showForm ? 'btn-ghost' : 'btn-primary'}`}
            onClick={() => setShowForm((open) => !open)}
          >
            {showForm ? 'Cancel' : 'New user'}
          </button>
        }
      />

      {error && <ErrorAlert message={error} onRetry={reload} />}

      {showForm && (
        <section className="card border border-base-300 bg-base-100 shadow-sm">
          <div className="card-body">
            <div>
              <h2 className="card-title text-base">Create user</h2>
              <p className="text-sm text-base-content/50">
                The account can sign in immediately after creation.
              </p>
            </div>

            <form onSubmit={handleCreate} className="grid gap-4 md:grid-cols-2">
              <label className="form-control w-full">
                <span className="label-text mb-1.5 font-medium">Email</span>
                <input
                  className="input input-bordered w-full"
                  placeholder="name@example.com"
                  type="email"
                  required
                  autoComplete="off"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </label>
              <label className="form-control w-full">
                <span className="label-text mb-1.5 font-medium">Display name</span>
                <input
                  className="input input-bordered w-full"
                  placeholder="Optional display name"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                />
              </label>
              <label className="form-control w-full md:col-span-2">
                <span className="label-text mb-1.5 font-medium">Password</span>
                <input
                  className="input input-bordered w-full"
                  placeholder="Initial password"
                  type="password"
                  required
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
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
                  {submitting ? 'Creating...' : 'Create user'}
                </button>
              </div>
            </form>
          </div>
        </section>
      )}

      <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <div>
            <h2 className="font-semibold">All users</h2>
            <p className="text-sm text-base-content/50">Accounts available to this server.</p>
          </div>
          <span className="badge badge-ghost badge-sm">{users.length} total</span>
        </div>

        {loading ? (
          <LoadingBlock label="Loading users..." />
        ) : users.length === 0 ? (
          <div className="p-5">
            <EmptyState
              title="No users"
              description="Create the first user account with the New user button above."
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead className="bg-base-200/50 text-[11px] uppercase tracking-wide text-base-content/50">
                <tr>
                  <th className="hidden lg:table-cell">ID</th>
                  <th>Email</th>
                  <th className="hidden md:table-cell">Display name</th>
                  <th className="hidden md:table-cell">Last login</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id} className="hover">
                    <td className="hidden font-mono text-xs text-base-content/50 lg:table-cell">
                      {user.id}
                    </td>
                    <td className="font-medium">
                      {user.email}
                      <span className="mt-0.5 block text-xs font-normal text-base-content/50 md:hidden">
                        {user.display_name || '—'}
                      </span>
                    </td>
                    <td className="hidden md:table-cell">
                      {user.display_name || <span className="text-base-content/40">—</span>}
                    </td>
                    <td className="hidden whitespace-nowrap text-sm text-base-content/60 md:table-cell">
                      {user.last_login_at || '—'}
                    </td>
                    <td>
                      <div className="flex justify-end">
                        <button
                          className="btn btn-ghost btn-xs text-error"
                          onClick={() => handleDelete(user.id, user.email)}
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}
