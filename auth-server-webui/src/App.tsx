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

function Spinner({ label }: { label: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-screen gap-2">
      <span className="loading loading-spinner loading-lg" />
      <p className="text-base-content/60">{label}</p>
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

function LoginPage() {
  const { status, error, login } = useAuth()

  if (status === 'authenticated') {
    return <Navigate to="/" replace />
  }

  return (
    <div className="flex flex-col items-center justify-center h-screen gap-4 px-4">
      <h1 className="text-3xl font-bold">Auth Server Admin</h1>
      <p className="text-base-content/60 text-center">
        Sign in with the authorization server to manage users, clients and administrators.
      </p>
      {error && <div className="alert alert-error max-w-xl">{error}</div>}
      <button className="btn btn-primary btn-lg" onClick={() => login('/')}>
        Sign in with Auth Server
      </button>
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
    <div className="hero min-h-[60vh]">
      <div className="hero-content text-center">
        <div className="max-w-lg space-y-4">
          <h1 className="text-3xl font-bold">403 — Forbidden</h1>
          <p className="text-base-content/70">
            Your account is signed in but does not have the required management permission.
          </p>
          {required && (
            <p>
              Required scope: <code className="badge badge-ghost">{required}</code>
            </p>
          )}
          <div className="flex justify-center gap-2">
            <Link to="/" className="btn btn-primary">
              Back to dashboard
            </Link>
            <button
              className="btn"
              onClick={() => {
                logout()
                navigate('/login', { replace: true })
              }}
            >
              Sign out
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function NotFound() {
  return (
    <div className="hero min-h-screen">
      <div className="hero-content text-center">
        <div className="max-w-lg space-y-4">
          <h1 className="text-3xl font-bold">404 — Not Found</h1>
          <p className="text-base-content/70">This page does not exist.</p>
          <Link to="/" className="btn btn-primary">
            Go to dashboard
          </Link>
        </div>
      </div>
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

function AdminLayout() {
  const { session, logout, hasScope } = useAuth()
  const navigate = useNavigate()

  if (!session) {
    return null
  }

  const navItems = [
    { to: '/', label: 'Dashboard', end: true, visible: true },
    { to: '/users', label: 'Users', end: false, visible: hasScope('manage:system') },
    { to: '/clients', label: 'Clients', end: false, visible: hasScope('manage:clients') },
    { to: '/admins', label: 'Admins', end: false, visible: hasScope('manage:system') },
    { to: '/sessions', label: 'Sessions', end: false, visible: hasScope('manage:system') },
  ]

  return (
    <div className="drawer lg:drawer-open">
      <input id="main-drawer" type="checkbox" className="drawer-toggle" />
      <div className="drawer-content flex flex-col min-h-screen">
        <div className="navbar bg-base-300">
          <div className="flex-none lg:hidden">
            <label htmlFor="main-drawer" className="btn btn-square btn-ghost drawer-button">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                className="inline-block h-5 w-5 stroke-current"
              >
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </label>
          </div>
          <div className="flex-1">
            <Link to="/" className="btn btn-ghost text-xl">
              Auth Server Admin
            </Link>
          </div>
          <div className="flex-none flex items-center gap-3">
            <span className="hidden sm:inline text-sm text-base-content/70">{session.user?.email}</span>
            <button
              className="btn btn-ghost"
              onClick={() => {
                logout()
                navigate('/login', { replace: true })
              }}
            >
              Logout
            </button>
          </div>
        </div>
        <main className="p-4 flex-1">
          <Outlet />
        </main>
      </div>
      <div className="drawer-side">
        <label htmlFor="main-drawer" aria-label="close sidebar" className="drawer-overlay" />
        <ul className="menu bg-base-200 text-base-content min-h-full w-60 p-4">
          {navItems
            .filter((item) => item.visible)
            .map((item) => (
              <li key={item.to}>
                <NavLink
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) => (isActive ? 'active' : '')}
                >
                  {item.label}
                </NavLink>
              </li>
            ))}
        </ul>
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
