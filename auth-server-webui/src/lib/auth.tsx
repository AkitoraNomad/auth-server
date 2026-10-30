import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, ApiError, errorMessage } from './api'
import { ADMIN_CLIENT_ID, ADMIN_SCOPES, REDIRECT_URI, authUrl, safeReturnPath } from './config'
import {
  AuthContext,
  type AuthContextValue,
  type AuthStatus,
  type SessionState,
} from './auth-context'
import type { MeResponse, TokenResponse } from '../types'

const PENDING_KEY = 'auth_admin_pkce_v1'
const PENDING_TTL_MS = 10 * 60 * 1000

// Remove credentials written by the pre-Phase-1 implementation. Tokens are
// memory-only now, and the old localStorage/sessionStorage entries must not be
// silently reused.
try {
  localStorage.removeItem('admin_tokens')
  sessionStorage.removeItem('pkce_verifier')
  sessionStorage.removeItem('pkce_state')
} catch {
  // Storage can be unavailable in hardened/privacy modes; memory-only auth still works.
}

interface PendingAuthorization {
  verifier: string
  state: string
  createdAt: number
  returnTo: string
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function randomValue(length: number): string {
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  return base64UrlEncode(bytes)
}

async function codeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))
  return base64UrlEncode(new Uint8Array(digest))
}

function readPendingAuthorization(): PendingAuthorization | null {
  try {
    const raw = sessionStorage.getItem(PENDING_KEY)
    if (!raw) {
      return null
    }
    const value = JSON.parse(raw) as Partial<PendingAuthorization>
    if (
      typeof value.verifier !== 'string' ||
      typeof value.state !== 'string' ||
      typeof value.createdAt !== 'number'
    ) {
      return null
    }
    return {
      verifier: value.verifier,
      state: value.state,
      createdAt: value.createdAt,
      returnTo: safeReturnPath(value.returnTo),
    }
  } catch {
    return null
  }
}

function writePendingAuthorization(pending: PendingAuthorization) {
  sessionStorage.setItem(PENDING_KEY, JSON.stringify(pending))
}

function clearPendingAuthorization() {
  sessionStorage.removeItem(PENDING_KEY)
}

function scopesFromToken(value: string | undefined): string[] {
  return (value || '').split(/\s+/).filter(Boolean)
}

function sessionFromToken(token: TokenResponse): SessionState {
  if (!token.access_token) {
    throw new Error('The token endpoint did not return an access token.')
  }
  return {
    accessToken: token.access_token,
    refreshToken: token.refresh_token || null,
    expiresAt: Date.now() + Math.max(1, token.expires_in || 3600) * 1000,
    scopes: scopesFromToken(token.scope),
    user: null,
  }
}

async function postToken(body: URLSearchParams): Promise<TokenResponse> {
  const response = await fetch(authUrl('/oauth2/token'), {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: body.toString(),
  })
  const text = await response.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = null
    }
  }
  if (!response.ok) {
    const record = (payload && typeof payload === 'object' ? payload : {}) as Record<string, unknown>
    const message =
      (typeof record.error_description === 'string' && record.error_description) ||
      (typeof record.error === 'string' && record.error) ||
      `Token request failed (HTTP ${response.status})`
    throw new Error(message)
  }
  return payload as TokenResponse
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<SessionState | null>(null)
  const [logoutState, setLogoutState] = useState<'idle' | 'pending' | 'complete'>('idle')
  const signingOut = logoutState === 'pending'
  const [error, setError] = useState<string | null>(null)
  const sessionRef = useRef<SessionState | null>(null)
  const authGenerationRef = useRef(0)
  const signingOutRef = useRef(false)
  const refreshPromiseRef = useRef<Promise<SessionState | null> | null>(null)
  const exchangeRef = useRef<Map<string, Promise<string | null>>>(new Map())

  const setSession = useCallback((next: SessionState | null) => {
    sessionRef.current = next
    setSessionState(next)
  }, [])

  const refreshAccessToken = useCallback(async (): Promise<SessionState | null> => {
    const current = sessionRef.current
    if (signingOutRef.current || !current?.refreshToken) {
      return null
    }
    if (refreshPromiseRef.current) {
      return refreshPromiseRef.current
    }

    const generation = authGenerationRef.current
    const promise = (async () => {
      try {
        const token = await postToken(
          new URLSearchParams({
            grant_type: 'refresh_token',
            refresh_token: current.refreshToken as string,
            client_id: ADMIN_CLIENT_ID,
          }),
        )
        const refreshed = sessionFromToken(token)
        const next: SessionState = {
          ...refreshed,
          refreshToken: token.refresh_token || current.refreshToken,
          scopes: refreshed.scopes.length > 0 ? refreshed.scopes : current.scopes,
          user: current.user,
        }
        if (generation !== authGenerationRef.current) return null
        setSession(next)
        setError(null)
        return next
      } catch (err) {
        if (generation !== authGenerationRef.current) return null
        setSession(null)
        setError(errorMessage(err) || 'Your session expired. Please sign in again.')
        return null
      } finally {
        refreshPromiseRef.current = null
      }
    })()

    refreshPromiseRef.current = promise
    return promise
  }, [setSession])

  const login = useCallback((returnTo?: string) => {
    setLogoutState('idle')
    const run = async () => {
      const verifier = randomValue(32)
      const state = randomValue(24)
      const challenge = await codeChallenge(verifier)
      writePendingAuthorization({
        verifier,
        state,
        createdAt: Date.now(),
        returnTo: safeReturnPath(returnTo),
      })

      const params = new URLSearchParams({
        client_id: ADMIN_CLIENT_ID,
        response_type: 'code',
        scope: ADMIN_SCOPES.join(' '),
        redirect_uri: REDIRECT_URI,
        code_challenge: challenge,
        code_challenge_method: 'S256',
        state,
      })
      window.location.assign(authUrl('/oauth2/authorize') + '?' + params.toString())
    }

    void run().catch((err: unknown) => {
      setError(errorMessage(err) || 'Unable to start sign in.')
    })
  }, [])

  const handleCallback = useCallback(
    async (
      code: string,
      state: string,
      oauthError?: string | null,
      errorDescription?: string | null,
    ): Promise<string | null> => {
      if (oauthError) {
        clearPendingAuthorization()
        setError(errorDescription || oauthError)
        return null
      }
      if (!code || !state) {
        clearPendingAuthorization()
        setError('The authorization response is missing code or state.')
        return null
      }

      const existingExchange = exchangeRef.current.get(code)
      if (existingExchange) {
        return existingExchange
      }

      const pending = readPendingAuthorization()
      if (!pending) {
        setError('This sign-in request is no longer valid. Please start again.')
        return null
      }
      if (Date.now() - pending.createdAt > PENDING_TTL_MS) {
        clearPendingAuthorization()
        setError('The sign-in request expired. Please try again.')
        return null
      }
      if (state !== pending.state) {
        clearPendingAuthorization()
        setError('The state check failed. Please start sign-in again.')
        return null
      }

      const promise = (async (): Promise<string | null> => {
        try {
          const token = await postToken(
            new URLSearchParams({
              grant_type: 'authorization_code',
              code,
              redirect_uri: REDIRECT_URI,
              client_id: ADMIN_CLIENT_ID,
              code_verifier: pending.verifier,
            }),
          )
          const initial = sessionFromToken(token)

          let me: MeResponse
          try {
            me = await api.getMe(initial.accessToken)
          } catch (err) {
            if (err instanceof ApiError && err.status === 403) {
              throw new Error('This account does not have administrator access.', { cause: err })
            }
            throw err
          }

          const next: SessionState = {
            ...initial,
            scopes: me.scopes.length > 0 ? me.scopes : initial.scopes,
            user: me.user,
          }
          setSession(next)
          setError(null)
          clearPendingAuthorization()
          return safeReturnPath(pending.returnTo)
        } catch (err) {
          setSession(null)
          setError(errorMessage(err) || 'Sign in failed. Please try again.')
          clearPendingAuthorization()
          return null
        } finally {
          exchangeRef.current.delete(code)
        }
      })()

      exchangeRef.current.set(code, promise)
      return promise
    },
    [setSession],
  )

  const logout = useCallback(async (): Promise<boolean> => {
    if (signingOutRef.current) return false
    signingOutRef.current = true
    setLogoutState('pending')
    setError(null)
    try {
      // Finish a refresh already in flight so logout uses its successor.
      await refreshPromiseRef.current
      const current = sessionRef.current
      const token = current?.accessToken
      authGenerationRef.current += 1
      // A successful response means the server has invalidated the SSO
      // session and every existing grant, including concurrent refreshes.
      let headers: Record<string, string>
      if (token) {
        headers = { Authorization: `Bearer ${token}` }
      } else {
        const csrfResponse = await fetch(authUrl('/logout'), { credentials: 'include' })
        if (!csrfResponse.ok) throw new Error('Unable to end the server session.')
        const payload = await csrfResponse.json() as { data: { csrf_token: string } }
        headers = { 'X-CSRF-Token': payload.data.csrf_token }
      }
      let response = await fetch(authUrl('/logout'), {
        method: 'POST', credentials: 'include', headers,
      })
      // When the SSO cookie has expired, a live refresh token still
      // represents a grant that must be revoked before reporting success.
      if (response.status === 401 && current?.refreshToken) {
        const renewed = await postToken(new URLSearchParams({
          grant_type: 'refresh_token', client_id: ADMIN_CLIENT_ID,
          refresh_token: current.refreshToken,
        }))
        setSession({ ...sessionFromToken(renewed), user: current.user })
        response = await fetch(authUrl('/logout'), {
          method: 'POST', credentials: 'include',
          headers: { Authorization: `Bearer ${renewed.access_token}` },
        })
      }
      if (!response.ok) throw new Error('Server sign out failed. Please try again.')
      clearPendingAuthorization()
      setLogoutState('complete')
      setSession(null)
      setError(null)
      return true
    } catch (err) {
      setError(errorMessage(err))
      return false
    } finally {
      signingOutRef.current = false
      setLogoutState((current) => current === 'complete' ? current : 'idle')
    }
  }, [setSession])

  const hasScope = useCallback(
    (scope: string) => Boolean(session?.scopes.includes(scope)),
    [session],
  )

  useEffect(() => {
    api.setTokenReader(() => sessionRef.current?.accessToken ?? null)
    api.setUnauthorizedHandler(async () => {
      const next = await refreshAccessToken()
      return next?.accessToken ?? null
    })
    return () => {
      api.setTokenReader(() => null)
      api.setUnauthorizedHandler(null)
    }
  }, [refreshAccessToken])

  useEffect(() => {
    if (!session) {
      return
    }
    const refreshAt = session.expiresAt - Date.now() - 60_000
    const delay = Math.max(5_000, Math.min(refreshAt, 2_147_000_000))
    const timer = window.setTimeout(() => {
      void refreshAccessToken()
    }, delay)
    return () => window.clearTimeout(timer)
  }, [session, refreshAccessToken])

  useEffect(() => {
    const onVisibilityChange = () => {
      const current = sessionRef.current
      if (
        document.visibilityState === 'visible' &&
        current &&
        current.expiresAt - Date.now() < 60_000
      ) {
        void refreshAccessToken()
      }
    }
    document.addEventListener('visibilitychange', onVisibilityChange)
    return () => document.removeEventListener('visibilitychange', onVisibilityChange)
  }, [refreshAccessToken])

  const status: AuthStatus = session ? 'authenticated'
    : logoutState === 'complete' ? 'signed-out'
    : error ? 'error' : 'anonymous'
  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      session,
      error,
      login,
      logout,
      signingOut,
      handleCallback,
      hasScope,
    }),
    [status, session, error, login, logout, signingOut, handleCallback, hasScope],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
