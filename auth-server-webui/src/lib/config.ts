const rawBase = import.meta.env.BASE_URL || '/'

// Vite's BASE_URL keeps a trailing slash; React Router's basename does not.
export const APP_BASE = rawBase.endsWith('/') ? rawBase.slice(0, -1) : rawBase
export const APP_BASE_WITH_SLASH = rawBase.endsWith('/') ? rawBase : rawBase + '/'

function normalizeBase(value: string | undefined): string {
  return (value || '').trim().replace(/\/+$/, '')
}

export const AUTH_BASE_URL = normalizeBase(import.meta.env.VITE_AUTH_BASE_URL) || window.location.origin
export const API_BASE_URL = normalizeBase(import.meta.env.VITE_API_BASE_URL) || window.location.origin

export async function getAdminClientID(): Promise<string> {
  const response = await fetch(authUrl('/api/public/admin-config'), { cache: 'no-store' })
  if (!response.ok) throw new Error('Unable to load management client configuration.')
  const config = await response.json() as { client_id?: unknown }
  if (typeof config.client_id !== 'string' || !/^[1-9]\d*$/.test(config.client_id)) {
    throw new Error('Invalid management client configuration.')
  }
  return config.client_id
}
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
