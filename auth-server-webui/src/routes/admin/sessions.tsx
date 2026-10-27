import { useEffect, useState } from 'react'
import { api, errorMessage } from '../../lib/api'
import type { User } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'

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
    <div className="space-y-5">
      <PageHeader
        eyebrow="Governance"
        title="Sessions"
        description="Revoke all existing grants and SSO sessions for a user. Users can sign in again afterward."
      />

      <div className="alert alert-info alert-soft items-start" role="status">
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
          <circle cx="12" cy="12" r="9" />
          <path d="M12 11v5M12 8h.01" />
        </svg>
        <div>
          <p className="font-medium">User-wide sign out</p>
          <p className="mt-0.5 text-sm opacity-80">
            Revocation ends all existing server sessions and grants for the selected user.
            This table lists users, rather than individual active sessions.
          </p>
        </div>
      </div>

      {error && <ErrorAlert message={error} onRetry={reload} />}

      <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <div>
            <h2 className="font-semibold">User records</h2>
            <p className="text-sm text-base-content/50">Choose a user whose existing access should be revoked.</p>
          </div>
          <span className="badge badge-ghost badge-sm">{users.length} total</span>
        </div>

        {loading ? (
          <LoadingBlock label="Loading user records..." />
        ) : users.length === 0 ? (
          <div className="p-5">
            <EmptyState
              title="No user records"
              description="There are no user records to display for this endpoint."
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead className="bg-base-200/50 text-[11px] uppercase tracking-wide text-base-content/50">
                <tr>
                  <th className="hidden lg:table-cell">User ID</th>
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
                          className="btn btn-ghost btn-xs"
                          onClick={() => {
                            if (!window.confirm(`Revoke all sessions for ${user.email}?`)) return
                            void api.revokeUserSessions(user.id).then(reload).catch((err: unknown) => setError(errorMessage(err)))
                          }}
                        >
                          Revoke
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
