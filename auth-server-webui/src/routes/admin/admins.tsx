import { useEffect, useState, type FormEvent } from 'react'
import { api, errorMessage } from '../../lib/api'
import type { AdminEntry } from '../../types'
import { EmptyState, ErrorAlert, LoadingBlock } from '../../components/Feedback'

export default function AdminsPage() {
  const [admins, setAdmins] = useState<AdminEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [userId, setUserId] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    let active = true
    api
      .getAdmins()
      .then((data) => {
        if (active) {
          setAdmins(data)
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
    <div>
      <div className="mb-4">
        <h1 className="text-2xl font-bold">Administrators</h1>
        <p className="text-sm text-base-content/60">
          Membership is checked on every API request, so removing an administrator takes effect
          immediately. Last-administrator protection is planned for Phase 3.
        </p>
      </div>

      {error && <ErrorAlert message={error} onRetry={reload} />}

      <form onSubmit={handleAdd} className="flex flex-wrap gap-2 mb-4">
        <input
          className="input input-bordered"
          type="number"
          min={1}
          placeholder="User ID"
          required
          value={userId}
          onChange={(event) => setUserId(event.target.value)}
        />
        <button className="btn btn-primary" type="submit" disabled={submitting}>
          {submitting ? 'Adding...' : 'Add administrator'}
        </button>
      </form>

      {loading ? (
        <LoadingBlock />
      ) : admins.length === 0 ? (
        <EmptyState title="No administrators" description="Use the form above to grant management access to a user ID." />
      ) : (
        <div className="overflow-x-auto">
          <table className="table table-zebra">
            <thead>
              <tr>
                <th>Admin ID</th>
                <th>User ID</th>
                <th>Email</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {admins.map((admin) => (
                <tr key={admin.id}>
                  <td>{admin.id}</td>
                  <td>{admin.user_id}</td>
                  <td>{admin.email}</td>
                  <td>
                    <button className="btn btn-error btn-xs" onClick={() => handleRemove(admin)}>
                      Remove
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
