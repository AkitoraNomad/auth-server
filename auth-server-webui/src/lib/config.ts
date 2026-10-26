const rawBase = import.meta.env.BASE_URL || '/'

// Vite's BASE_URL keeps a trailing slash; React Router's basename does not.
export const APP_BASE = rawBase.endsWith('/') ? rawBase.slice(0, -1) : rawBase
export const APP_BASE_WITH_SLASH = rawBase.endsWith('/') ? rawBase : rawBase + '/'

function normalizeBase(value: string | undefined): string {
  return (value || '').trim().replace(/\/+$/, '')
}

export const AUTH_BASE_URL = normalizeBase(import.meta.env.VITE_AUTH_BASE_URL) || window.location.origin
export const API_BASE_URL = normalizeBase(import.meta.env.VITE_API_BASE_URL) || window.location.origin

export const ADMIN_CLIENT_ID = (import.meta.env.VITE_ADMIN_CLIENT_ID || '2').trim()
export const ADMIN_SCOPES = ['manage:system', 'manage:clients'] as const
export type AdminScope = (typeof ADMIN_SCOPES)[number]

export const REDIRECT_URI = (
  import.meta.env.VITE_ADMIN_REDIRECT_URI ||
  `${window.location.origin}${APP_BASE_WITH_SLASH}callback`
).trim()

export function authUrl(path: string): string {
  return AUTH_BASE_URL + (path.startsWith('/') ? path : `/${path}`)
}

export function apiUrl(path: string): string {
  return API_BASE_URL + (path.startsWith('/') ? path : `/${path}`)
}

// Return paths are always React Router paths (basename already stripped).
export function safeReturnPath(input: string | undefined): string {
  if (!input || !input.startsWith('/') || input.startsWith('//') || input.includes('://')) {
    return '/'
  }
  if (input === '/login' || input.startsWith('/login?') || input.startsWith('/callback')) {
    return '/'
  }
  return input
}
