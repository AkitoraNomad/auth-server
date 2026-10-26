import { useEffect, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import type { User } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'

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
    <div>
      <div className="flex items-center justify-between mb-4 gap-4">
        <div>
          <h1 className="text-2xl font-bold">Users</h1>
          <p className="text-sm text-base-content/60">
            Edit, disable and password-reset are planned for Phase 3.
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowForm((open) => !open)}>
          {showForm ? 'Cancel' : 'New User'}
        </button>
      </div>

      {error && <ErrorAlert message={error} onRetry={reload} />}

      {showForm && (
        <form onSubmit={handleCreate} className="card bg-base-200 p-4 mb-4 space-y-3">
          <input
            className="input input-bordered w-full"
            placeholder="Email"
            type="email"
            required
            autoComplete="off"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
          <input
            className="input input-bordered w-full"
            placeholder="Display Name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
          <input
            className="input input-bordered w-full"
            placeholder="Password"
            type="password"
            required
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <button className="btn btn-primary" type="submit" disabled={submitting}>
            {submitting ? 'Creating...' : 'Create'}
          </button>
        </form>
      )}

      {loading ? (
        <LoadingBlock />
      ) : users.length === 0 ? (
        <EmptyState title="No users" description="Create the first user account with the button above." />
      ) : (
        <div className="overflow-x-auto">
          <table className="table table-zebra">
            <thead>
              <tr>
                <th>ID</th>
                <th>Email</th>
                <th>Display Name</th>
                <th>Last Login</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.id}</td>
                  <td>{user.email}</td>
                  <td>{user.display_name || '-'}</td>
                  <td>{user.last_login_at || '-'}</td>
                  <td>
                    <div className="flex flex-wrap gap-1">
                      <button className="btn btn-error btn-xs" onClick={() => handleDelete(user.id, user.email)}>
                        Delete
                      </button>
                      <button className="btn btn-xs" disabled title="Editing is planned for Phase 3">
                        Edit
                      </button>
                      <button className="btn btn-xs" disabled title="Enable/disable is planned for Phase 3">
                        Disable
                      </button>
                      <button className="btn btn-xs" disabled title="Password reset is planned for Phase 3">
                        Reset password
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
