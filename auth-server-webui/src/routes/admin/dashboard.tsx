import { Link } from 'react-router'
import { useAuth } from '../../lib/auth-context'

export default function Dashboard() {
  const { session, hasScope } = useAuth()

  const cards = [
    {
      to: '/users',
      title: 'Users',
      description: 'Create, inspect and remove user accounts.',
      visible: hasScope('manage:system'),
    },
    {
      to: '/clients',
      title: 'OAuth2 Clients',
      description: 'Register clients, redirect URIs, scopes and grant types.',
      visible: hasScope('manage:clients'),
    },
    {
      to: '/admins',
      title: 'Administrators',
      description: 'Grant or revoke management access.',
      visible: hasScope('manage:system'),
    },
    {
      to: '/sessions',
      title: 'Sessions',
      description: 'Session/token inventory (Phase 4).',
      visible: hasScope('manage:system'),
    },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-base-content/70 mt-1">
          Signed in as {session?.user?.display_name || session?.user?.email || 'administrator'}.
        </p>
        <div className="flex flex-wrap gap-2 mt-3">
          {(session?.scopes || []).map((scope) => (
            <span key={scope} className="badge badge-outline">
              {scope}
            </span>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        {cards
          .filter((card) => card.visible)
          .map((card) => (
            <div key={card.to} className="card bg-base-200 shadow">
              <div className="card-body">
                <h2 className="card-title">{card.title}</h2>
                <p className="text-sm text-base-content/70">{card.description}</p>
                <div className="card-actions justify-end">
                  <Link className="btn btn-primary btn-sm" to={card.to}>
                    Manage
                  </Link>
                </div>
              </div>
            </div>
          ))}
      </div>
    </div>
  )
}
