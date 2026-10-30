import { UsersIcon, ClientsIcon, AdminsIcon, SessionsIcon } from '../../components/NavigationIcons'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { useAuth } from '../../lib/auth-context'
import { ADMIN_CLIENT_ID } from '../../lib/config'

type AreaCard = {
  to: string
  title: string
  description: string
  scope: string
  icon: ReactNode
  tone: string
  phase?: string
  cta: string
}

const scopeInfo: Record<string, { label: string; description: string }> = {
  'manage:system': {
    label: 'System administration',
    description: 'Users, administrators and session inventory.',
  },
  'manage:clients': {
    label: 'Client management',
    description: 'OAuth2 clients, redirect URIs, scopes and grant types.',
  },
  profile: {
    label: 'Profile',
    description: 'Basic account profile information.',
  },
  avatar: {
    label: 'Avatar',
    description: 'Account avatar image.',
  },
}

function formatExpiry(timestamp: number): string {
  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) {
    return 'Unknown'
  }
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function ArrowIcon({ className = 'h-4 w-4' }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="2"
      viewBox="0 0 24 24"
    >
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  )
}

export default function Dashboard() {
  const { session, hasScope } = useAuth()

  if (!session) {
    return null
  }

  const displayName = session.user?.display_name?.trim() || session.user?.email || 'Administrator'
  const email = session.user?.email || 'No email on this session'
  const initials = displayName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join('')
    .toUpperCase()

  const scopes = Array.from(new Set(session.scopes))
  const fullAccess = hasScope('manage:system') && hasScope('manage:clients')
  const expiry = formatExpiry(session.expiresAt)

  const cards: AreaCard[] = [
    {
      to: '/users',
      title: 'Users',
      description: 'Create and remove accounts, and inspect login activity.',
      scope: 'manage:system',
      icon: <UsersIcon className="h-5 w-5" />,
      tone: 'bg-primary/10 text-primary',
      cta: 'Open',
    },
    {
      to: '/clients',
      title: 'OAuth2 clients',
      description: 'Register clients and manage redirect URIs, scopes and grants.',
      scope: 'manage:clients',
      icon: <ClientsIcon className="h-5 w-5" />,
      tone: 'bg-secondary/10 text-secondary',
      cta: 'Open',
    },
    {
      to: '/admins',
      title: 'Administrators',
      description: 'Grant or revoke access to this management console.',
      scope: 'manage:system',
      icon: <AdminsIcon className="h-5 w-5" />,
      tone: 'bg-accent/10 text-accent',
      cta: 'Open',
    },
    {
      to: '/sessions',
      title: 'Sessions',
      description: 'Active session and token inventory for this server.',
      scope: 'manage:system',
      icon: <SessionsIcon className="h-5 w-5" />,
      tone: 'bg-warning/10 text-warning',
      phase: 'Phase 4',
      cta: 'Review',
    },
  ]

  const visibleCards = cards.filter((card) => hasScope(card.scope))
  const scopeDetails = scopes.map((scope) => ({
    name: scope,
    ...(scopeInfo[scope] || {
      label: scope,
      description: 'Additional scope granted to this session.',
    }),
  }))

  return (
    <div className="space-y-6">
      <section className="relative overflow-hidden rounded-3xl border border-base-300 bg-base-100 p-6 shadow-sm md:p-8">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -right-24 -top-24 h-72 w-72 rounded-full bg-primary/10 blur-3xl"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -bottom-32 left-1/3 h-64 w-64 rounded-full bg-secondary/10 blur-3xl"
        />

        <div className="relative">
          <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 items-center gap-4">
              <div className="avatar avatar-placeholder shrink-0">
                <div className="w-14 rounded-2xl bg-primary text-primary-content">
                  <span className="text-lg font-semibold" aria-hidden="true">
                    {initials || 'A'}
                  </span>
                </div>
              </div>
              <div className="min-w-0">
                <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-base-content/50">
                  Administration
                </p>
                <h1 className="break-words text-2xl font-bold leading-tight tracking-tight md:text-3xl">
                  Welcome back, {displayName}
                </h1>
                <p className="truncate text-sm text-base-content/60">{email}</p>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              <span className="badge badge-success badge-sm gap-1.5">
                <span className="h-1.5 w-1.5 rounded-full bg-success-content" />
                Session active
              </span>
              <span className="badge badge-ghost badge-sm">Management client #{ADMIN_CLIENT_ID}</span>
            </div>
          </div>

          <div className="mt-5 flex flex-wrap items-center gap-2 border-t border-base-300/70 pt-4">
            <span className="text-xs text-base-content/50">Granted scopes</span>
            {scopes.map((scope) => (
              <span key={scope} className="badge badge-outline badge-sm font-mono text-[11px]">
                {scope}
              </span>
            ))}
          </div>
        </div>
      </section>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-2xl border border-base-300 bg-base-100 p-4 shadow-sm">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-base-content/50">
            Granted scopes
          </p>
          <p className="mt-2 text-2xl font-semibold tracking-tight">{scopes.length}</p>
          <p className="mt-1 text-xs text-base-content/50">Capabilities attached to this session.</p>
        </div>
        <div className="rounded-2xl border border-base-300 bg-base-100 p-4 shadow-sm">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-base-content/50">
            Access level
          </p>
          <p className="mt-2 text-2xl font-semibold tracking-tight">
            {fullAccess ? 'Full management' : 'Limited'}
          </p>
          <p className="mt-1 text-xs text-base-content/50">
            {fullAccess
              ? 'System and client administration.'
              : 'Only granted management areas are available.'}
          </p>
        </div>
        <div className="rounded-2xl border border-base-300 bg-base-100 p-4 shadow-sm">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-base-content/50">
            Session expires
          </p>
          <p className="mt-2 text-xl font-semibold tracking-tight">{expiry}</p>
          <p className="mt-1 text-xs text-base-content/50">Access token refresh is automatic.</p>
        </div>
      </div>

      <section>
        <div className="mb-3 flex items-end justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold">Management areas</h2>
            <p className="text-sm text-base-content/50">
              Choose where you want to work. Only permitted areas are shown.
            </p>
          </div>
          <span className="badge badge-ghost badge-sm shrink-0">{visibleCards.length} available</span>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          {visibleCards.map((card) => (
            <Link
              key={card.to}
              to={card.to}
              className="group card border border-base-300 bg-base-100 shadow-sm transition duration-200 hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
            >
              <div className="card-body gap-4 p-5">
                <div className="flex items-start justify-between gap-3">
                  <span className={`inline-flex rounded-2xl p-3 ${card.tone}`}>{card.icon}</span>
                  {card.phase && <span className="badge badge-warning badge-sm">{card.phase}</span>}
                </div>

                <div>
                  <h3 className="card-title text-base">{card.title}</h3>
                  <p className="mt-1 text-sm leading-6 text-base-content/60">{card.description}</p>
                </div>

                <div className="mt-auto flex items-center justify-between gap-3 border-t border-base-300/70 pt-3">
                  <span className="badge badge-ghost badge-xs font-mono">{card.scope}</span>
                  <span className="inline-flex items-center gap-1 text-sm font-medium text-primary">
                    {card.cta}
                    <ArrowIcon className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
                  </span>
                </div>
              </div>
            </Link>
          ))}
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-3">
        <section className="card border border-base-300 bg-base-100 shadow-sm lg:col-span-2">
          <div className="card-body">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="card-title text-base">Granted access</h2>
                <p className="mt-1 text-sm text-base-content/50">
                  Scopes attached to the current access token.
                </p>
              </div>
              <span className="badge badge-ghost badge-sm">{scopeDetails.length} scopes</span>
            </div>

            {scopeDetails.length === 0 ? (
              <p className="py-4 text-sm text-base-content/50">No scopes are attached to this session.</p>
            ) : (
              <div className="mt-2 grid gap-3 sm:grid-cols-2">
                {scopeDetails.map((scope) => (
                  <div
                    key={scope.name}
                    className="rounded-2xl border border-base-300 bg-base-200/40 p-3"
                  >
                    <div className="flex items-center gap-2">
                      <code className="text-xs font-semibold">{scope.name}</code>
                      <span className="badge badge-success badge-xs">granted</span>
                    </div>
                    <p className="mt-2 text-sm font-medium">{scope.label}</p>
                    <p className="mt-0.5 text-xs leading-5 text-base-content/50">
                      {scope.description}
                    </p>
                  </div>
                ))}
              </div>
            )}

            <p className="mt-auto border-t border-base-300/70 pt-3 text-xs leading-5 text-base-content/50">
              Scope checks are enforced by the management API on every request.
            </p>
          </div>
        </section>

        <section className="card border border-base-300 bg-base-100 shadow-sm">
          <div className="card-body">
            <h2 className="card-title text-base">Session</h2>
            <p className="text-sm text-base-content/50">Current management session details.</p>

            <dl className="mt-2 divide-y divide-base-300/70 text-sm">
              <div className="flex flex-col gap-1 py-3 first:pt-0">
                <dt className="text-xs font-medium uppercase tracking-wide text-base-content/40">
                  Account
                </dt>
                <dd className="break-all">{email}</dd>
              </div>
              <div className="flex flex-col gap-1 py-3">
                <dt className="text-xs font-medium uppercase tracking-wide text-base-content/40">
                  Management client
                </dt>
                <dd>
                  #{ADMIN_CLIENT_ID} <span className="text-base-content/50">· public PKCE</span>
                </dd>
              </div>
              <div className="flex flex-col gap-1 py-3 last:pb-0">
                <dt className="text-xs font-medium uppercase tracking-wide text-base-content/40">
                  Expires
                </dt>
                <dd>{expiry}</dd>
              </div>
            </dl>

            <p className="mt-auto border-t border-base-300/70 pt-3 text-xs leading-5 text-base-content/50">
              Tokens stay in memory and are refreshed before expiry. Sign out clears this session.
            </p>
          </div>
        </section>
      </div>
    </div>
  )
}
