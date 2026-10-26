import { useEffect, useState } from 'react'
import { api, errorMessage } from '../../lib/api'
import type { User } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'

export default function SessionsPage() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let active = true
    api
      .getSessions()
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

  return (
    <div>
      <div className="mb-4">
        <h1 className="text-2xl font-bold">Sessions</h1>
        <p className="text-sm text-base-content/60">
          A real active-token/session index is planned for Phase 4. Until then this page only shows
          user records, and revoke is intentionally unavailable rather than returning a false success.
        </p>
      </div>

      <div className="alert alert-info mb-4">
        <span>Session listing and forced revocation are not implemented in Phase 1.</span>
      </div>

      {error && <ErrorAlert message={error} onRetry={reload} />}

      {loading ? (
        <LoadingBlock />
      ) : users.length === 0 ? (
        <EmptyState title="No users" description="There are no user records to display." />
      ) : (
        <div className="overflow-x-auto">
          <table className="table table-zebra">
            <thead>
              <tr>
                <th>User ID</th>
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
                    <button className="btn btn-xs" disabled title="Session revocation is planned for Phase 4">
                      Revoke
                    </button>
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
