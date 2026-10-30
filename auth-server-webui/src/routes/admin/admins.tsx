import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import type { AdminEntry } from '../../types'
import { api, errorMessage } from '../../lib/api'
import { useList } from '../../lib/use-list'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'
import { PageHeader } from '../../components/PageHeader'

const loadItems = () => api.getAdmins()

export default function AdminsPage() {
  const { data: admins, loading, error, setError, reload } = useList(loadItems)
  const [userId, setUserId] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleAdd = async (event: FormEvent) => {
    event.preventDefault()
    const id = Number(userId)
    if (!Number.isInteger(id) || id <= 0) {
      setError('Enter a valid numeric user ID.')
      return
    }
    setSubmitting(true)
    setError(null)
    try {
      await api.addAdmin(id)
      setUserId('')
      reload()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleRemove = async (admin: AdminEntry) => {
    if (!window.confirm(`Remove administrator access for ${admin.email || admin.user_id}?`)) {
      return
    }
    setError(null)
    try {
      await api.removeAdmin(admin.user_id)
      reload()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="Governance"
        title="Administrators"
        description="Grant or revoke management access. Membership is checked on every API request, so changes take effect immediately. The last administrator cannot be removed."
        actions={
          <Link to="/users" className="btn btn-outline">
            Browse users
          </Link>
        }
      />

      {error && <ErrorAlert message={error} onRetry={reload} />}

      <section className="card border border-base-300 bg-base-100 shadow-sm">
        <div className="card-body">
          <div>
            <h2 className="card-title text-base">Grant administrator access</h2>
            <p className="text-sm text-base-content/50">
              Add an existing user by ID. Administrator access is separate from OAuth2 scopes.
            </p>
          </div>

          <form onSubmit={handleAdd} className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <label className="form-control w-full sm:max-w-xs">
              <span className="label-text mb-1.5 font-medium">User ID</span>
              <input
                className="input input-bordered w-full"
                type="number"
                min={1}
                placeholder="e.g. 3"
                required
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
              />
            </label>
            <button className="btn btn-primary" type="submit" disabled={submitting}>
              {submitting ? 'Adding...' : 'Add administrator'}
            </button>
          </form>
        </div>
      </section>

      <section className="card overflow-hidden border border-base-300 bg-base-100 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <div>
            <h2 className="font-semibold">Administrator accounts</h2>
            <p className="text-sm text-base-content/50">Users with access to this management console.</p>
          </div>
          <span className="badge badge-ghost badge-sm">{admins.length} total</span>
        </div>

        {loading ? (
          <LoadingBlock label="Loading administrators..." />
        ) : admins.length === 0 ? (
          <div className="p-5">
            <EmptyState
              title="No administrators"
              description="Use the form above to grant management access to an existing user ID."
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead className="bg-base-200/50 text-[11px] uppercase tracking-wide text-base-content/50">
                <tr>
                  <th className="hidden sm:table-cell">Admin ID</th>
                  <th className="hidden sm:table-cell">User ID</th>
                  <th>Email</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {admins.map((admin) => (
                  <tr key={admin.id} className="hover">
                    <td className="hidden font-mono text-xs text-base-content/50 sm:table-cell">
                      {admin.id}
                    </td>
                    <td className="hidden font-mono text-xs text-base-content/65 sm:table-cell">
                      {admin.user_id}
                    </td>
                    <td className="font-medium">
                      {admin.email}
                      <span className="mt-0.5 block text-xs font-normal text-base-content/50 sm:hidden">
                        User ID {admin.user_id}
                      </span>
                    </td>
                    <td>
                      <div className="flex justify-end">
                        <button
                          className="btn btn-ghost btn-xs text-error"
                          onClick={() => handleRemove(admin)}
                        >
                          Remove access
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
