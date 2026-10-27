import { useEffect, useRef, type ReactNode } from 'react'
import {
  Link,
  Navigate,
  NavLink,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router'
import { AuthProvider } from './lib/auth'
import { useAuth } from './lib/auth-context'
import Dashboard from './routes/admin/dashboard'
import UsersPage from './routes/admin/users'
import ClientsPage from './routes/admin/clients'
import AdminsPage from './routes/admin/admins'
import SessionsPage from './routes/admin/sessions'

function DashboardIcon() {
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px]"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
    >
      <rect height="7" rx="2" width="7" x="3" y="3" />
      <rect height="7" rx="2" width="7" x="14" y="3" />
      <rect height="7" rx="2" width="7" x="3" y="14" />
      <rect height="7" rx="2" width="7" x="14" y="14" />
    </svg>
  )
}

function UsersIcon() {
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px]"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
    >
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </svg>
  )
}

function ClientsIcon() {
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px]"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
    >
      <rect height="16" rx="3" width="18" x="3" y="4" />
      <path d="M3 9h18" />
      <path d="M7 6.5h.01M10 6.5h.01" />
    </svg>
  )
}

function AdminsIcon() {
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px]"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
    >
      <path d="M12 3 19 6v5c0 4.5-2.9 7.7-7 10-4.1-2.3-7-5.5-7-10V6l7-3Z" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  )
}

function SessionsIcon() {
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px]"
      fill="none"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
    >
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </svg>
  )
}

function Spinner({ label }: { label: string }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-base-100 px-4">
      <span className="loading loading-spinner loading-lg text-primary" />
      <p className="text-sm text-base-content/60">{label}</p>
    </div>
  )
}

function AutoLogin({ returnTo }: { returnTo: string }) {
  const { login } = useAuth()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    login(returnTo)
  }, [login, returnTo])

  return <Spinner label="Redirecting to the authorization server..." />
}

function RequireAuth() {
  const { status } = useAuth()
  const location = useLocation()

  if (status === 'authenticated') {
    return <Outlet />
  }
  if (status === 'error') {
    return <Navigate to="/login" replace />
  }
  return <AutoLogin returnTo={location.pathname + location.search} />
}

function BrandMark({ className = 'h-10 w-10 text-lg' }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`flex shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-content shadow-sm ${className}`}
    >
      🐯
    </span>
  )
}

function LoginPage() {
  const { status, error, login } = useAuth()

  if (status === 'authenticated') {
    return <Navigate to="/" replace />
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-base-200 px-4 py-10">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -left-24 -top-24 h-80 w-80 rounded-full bg-primary/10 blur-3xl"
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -bottom-32 -right-20 h-80 w-80 rounded-full bg-secondary/10 blur-3xl"
      />

      <div className="relative w-full max-w-md">
        <div className="mb-5 flex items-center justify-center gap-3">
          <BrandMark className="h-11 w-11 text-xl" />
          <div>
            <p className="text-sm font-semibold">Auth Server</p>
            <p className="text-xs uppercase tracking-[0.18em] text-base-content/45">Admin console</p>
          </div>
        </div>

        <section className="card border border-base-300 bg-base-100 shadow-xl">
          <div className="card-body p-6 sm:p-8">
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-base-content/45">
                Restricted access
              </p>
              <h1 className="mt-1 text-2xl font-bold tracking-tight">Sign in to continue</h1>
              <p className="mt-2 text-sm leading-6 text-base-content/60">
                Use your authorization server account to manage users, clients and administrators.
              </p>
            </div>

            {error && <div className="alert alert-error mt-2 text-sm">{error}</div>}

            <button className="btn btn-primary mt-2 w-full" onClick={() => login('/')}>
              Sign in with Auth Server
            </button>

            <p className="text-center text-xs leading-5 text-base-content/45">
              You will be redirected to the authorization server. Management scopes are required.
            </p>
          </div>
        </section>
      </div>
    </div>
  )
}

function CallbackPage() {
  const { handleCallback } = useAuth()
  const navigate = useNavigate()
  const attempted = useRef(false)

  useEffect(() => {
    if (attempted.current) {
      return
    }
    attempted.current = true

    const params = new URLSearchParams(window.location.search)
    const code = params.get('code') || ''
    const state = params.get('state') || ''
    const oauthError = params.get('error')
    const errorDescription = params.get('error_description')

    // Remove code/state from the address bar before exchanging it, so a refresh
    // cannot replay the authorization code.
    window.history.replaceState({}, '', window.location.pathname)

    void handleCallback(code, state, oauthError, errorDescription).then((returnTo) => {
      navigate(returnTo || '/login', { replace: true })
    })
  }, [handleCallback, navigate])

  return <Spinner label="Completing sign in..." />
}

function Forbidden({ required }: { required?: string }) {
  const { logout } = useAuth()
  const navigate = useNavigate()

  return (
    <div className="flex min-h-[60vh] items-center justify-center px-4">
      <section className="card w-full max-w-lg border border-base-300 bg-base-100 shadow-sm">
        <div className="card-body items-center py-10 text-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-warning/10 text-warning">
            <svg
              aria-hidden="true"
              className="h-6 w-6"
              fill="none"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="1.8"
              viewBox="0 0 24 24"
            >
              <rect height="11" rx="2" width="16" x="4" y="10" />
              <path d="M8 10V7a4 4 0 0 1 8 0v3" />
            </svg>
          </div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-base-content/45">
              403 — Forbidden
            </p>
            <h1 className="mt-1 text-2xl font-bold tracking-tight">Permission required</h1>
            <p className="mt-2 text-sm leading-6 text-base-content/60">
              Your account is signed in, but it does not have access to this management area.
            </p>
          </div>
          {required && (
            <p className="text-sm text-base-content/60">
              Required scope: <code className="badge badge-ghost badge-sm font-mono">{required}</code>
            </p>
          )}
          <div className="flex flex-wrap justify-center gap-2">
            <Link to="/" className="btn btn-primary btn-sm">
              Back to dashboard
            </Link>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => {
                logout()
                navigate('/login', { replace: true })
              }}
            >
              Sign out
            </button>
          </div>
        </div>
      </section>
    </div>
  )
}

function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-base-200 px-4">
      <section className="card w-full max-w-lg border border-base-300 bg-base-100 text-center shadow-sm">
        <div className="card-body items-center py-10">
          <BrandMark className="h-11 w-11 text-xl" />
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-base-content/45">
              404 — Not found
            </p>
            <h1 className="mt-1 text-2xl font-bold tracking-tight">Page not found</h1>
            <p className="mt-2 text-sm leading-6 text-base-content/60">
              This management console page does not exist or has moved.
            </p>
          </div>
          <Link to="/" className="btn btn-primary btn-sm">
            Go to dashboard
          </Link>
        </div>
      </section>
    </div>
  )
}

function ScopeGate({ scope, children }: { scope: string; children: ReactNode }) {
  const { hasScope } = useAuth()
  return hasScope(scope) ? <>{children}</> : <Forbidden required={scope} />
}

function AnyScopeGate({ children }: { children: ReactNode }) {
  const { hasScope } = useAuth()
  const allowed = hasScope('manage:system') || hasScope('manage:clients')
  return allowed ? <>{children}</> : <Forbidden required="manage:system or manage:clients" />
}

export function AdminLayout() {
  const { session, logout, hasScope } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  if (!session) {
    return null
  }

  const navItems = [
    {
      to: '/',
      label: 'Dashboard',
      icon: <DashboardIcon />,
      end: true,
      group: 'overview',
      visible: true,
    },
    {
      to: '/users',
      label: 'Users',
      icon: <UsersIcon />,
      end: false,
      group: 'management',
      visible: hasScope('manage:system'),
    },
    {
      to: '/clients',
      label: 'OAuth2 clients',
      icon: <ClientsIcon />,
      end: false,
      group: 'management',
      visible: hasScope('manage:clients'),
    },
    {
      to: '/admins',
      label: 'Administrators',
      icon: <AdminsIcon />,
      end: false,
      group: 'management',
      visible: hasScope('manage:system'),
    },
    {
      to: '/sessions',
      label: 'Sessions',
      icon: <SessionsIcon />,
      end: false,
      group: 'management',
      visible: hasScope('manage:system'),
    },
  ]

  const current = navItems.find((item) =>
    item.end ? location.pathname === item.to : location.pathname.startsWith(item.to),
  )
  const displayName = session.user?.display_name?.trim() || session.user?.email || 'Administrator'
  const email = session.user?.email || ''
  const initials = displayName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0])
    .join('')
    .toUpperCase()

  const closeDrawer = () => {
    const drawer = document.getElementById('main-drawer') as HTMLInputElement | null
    if (drawer) {
      drawer.checked = false
    }
  }

  const signOut = () => {
    logout()
    navigate('/login', { replace: true })
  }

  const visibleNav = navItems.filter((item) => item.visible)

  return (
    <div className="drawer lg:drawer-open">
      <input id="main-drawer" type="checkbox" className="drawer-toggle" />
      <div className="drawer-content flex min-h-screen flex-col bg-base-100">
        <header className="sticky top-0 z-20 border-b border-base-300 bg-base-100/90 backdrop-blur">
          <div className="flex h-16 items-center gap-3 px-4 md:px-6">
            <label
              htmlFor="main-drawer"
              aria-label="Open navigation"
              className="btn btn-square btn-ghost btn-sm lg:hidden"
            >
              <svg
                aria-hidden="true"
                className="h-5 w-5"
                fill="none"
                stroke="currentColor"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                viewBox="0 0 24 24"
              >
                <path d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </label>

            <div className="min-w-0 flex-1">
              <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-base-content/40">
                Admin console
              </p>
              <p className="truncate text-sm font-semibold">{current?.label || 'Not found'}</p>
            </div>

            <span className="hidden badge badge-ghost badge-sm sm:inline-flex">
              {session.scopes.length} scopes
            </span>
            <span className="hidden max-w-[220px] truncate text-sm text-base-content/55 xl:inline">
              {email}
            </span>
            <button className="btn btn-ghost btn-sm" onClick={signOut}>
              Sign out
            </button>
          </div>
        </header>

        <main className="flex-1 p-4 md:p-6 lg:p-8">
          <div className="mx-auto w-full max-w-[1400px]">
            <Outlet />
          </div>
        </main>
      </div>

      <div className="drawer-side z-40">
        <label htmlFor="main-drawer" aria-label="Close navigation" className="drawer-overlay" />
        <aside className="flex min-h-full w-72 flex-col border-r border-base-300 bg-base-200">
          <div className="flex h-16 items-center border-b border-base-300 px-5">
            <Link to="/" className="flex min-w-0 items-center gap-3" onClick={closeDrawer}>
              <BrandMark className="h-9 w-9 text-base" />
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold">Auth Server</span>
                <span className="block text-xs uppercase tracking-[0.16em] text-base-content/45">
                  Admin console
                </span>
              </span>
            </Link>
          </div>

          <nav className="flex-1 overflow-y-auto p-3" aria-label="Management navigation">
            <p className="px-3 pb-2 pt-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-base-content/40">
              Overview
            </p>
            <div className="space-y-1">
              {visibleNav
                .filter((item) => item.group === 'overview')
                .map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
                    onClick={closeDrawer}
                    className={({ isActive }) =>
                      `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
                        isActive
                          ? 'bg-primary text-primary-content shadow-sm'
                          : 'text-base-content/65 hover:bg-base-100 hover:text-base-content'
                      }`
                    }
                  >
                    <span className="shrink-0">{item.icon}</span>
                    <span className="truncate">{item.label}</span>
                  </NavLink>
                ))}
            </div>

            {visibleNav.some((item) => item.group === 'management') && (
              <>
                <p className="px-3 pb-2 pt-5 text-[10px] font-semibold uppercase tracking-[0.18em] text-base-content/40">
                  Management
                </p>
                <div className="space-y-1">
                  {visibleNav
                    .filter((item) => item.group === 'management')
                    .map((item) => (
                      <NavLink
                        key={item.to}
                        to={item.to}
                        end={item.end}
                        onClick={closeDrawer}
                        className={({ isActive }) =>
                          `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
                            isActive
                              ? 'bg-primary text-primary-content shadow-sm'
                              : 'text-base-content/65 hover:bg-base-100 hover:text-base-content'
                          }`
                        }
                      >
                        <span className="shrink-0">{item.icon}</span>
                        <span className="truncate">{item.label}</span>
                      </NavLink>
                    ))}
                </div>
              </>
            )}
          </nav>

          <div className="border-t border-base-300 p-3">
            <div className="flex items-center gap-3 rounded-2xl border border-base-300 bg-base-100 p-3">
              <div className="avatar avatar-placeholder shrink-0">
                <div className="w-9 rounded-xl bg-primary text-primary-content">
                  <span className="text-xs font-semibold" aria-hidden="true">
                    {initials || 'A'}
                  </span>
                </div>
              </div>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{displayName}</p>
                <p className="truncate text-xs text-base-content/50">{email}</p>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/callback" element={<CallbackPage />} />
        <Route element={<RequireAuth />}>
          <Route element={<AdminLayout />}>
            <Route
              index
              element={
                <AnyScopeGate>
                  <Dashboard />
                </AnyScopeGate>
              }
            />
            <Route
              path="users"
              element={
                <ScopeGate scope="manage:system">
                  <UsersPage />
                </ScopeGate>
              }
            />
            <Route
              path="clients"
              element={
                <ScopeGate scope="manage:clients">
                  <ClientsPage />
                </ScopeGate>
              }
            />
            <Route
              path="admins"
              element={
                <ScopeGate scope="manage:system">
                  <AdminsPage />
                </ScopeGate>
              }
            />
            <Route
              path="sessions"
              element={
                <ScopeGate scope="manage:system">
                  <SessionsPage />
                </ScopeGate>
              }
            />
          </Route>
        </Route>
        <Route path="*" element={<NotFound />} />
      </Routes>
    </AuthProvider>
  )
}
