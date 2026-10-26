import { createContext, useContext } from 'react'
import type { User } from '../types'

export interface SessionState {
  accessToken: string
  refreshToken: string | null
  expiresAt: number
  scopes: string[]
  user: User | null
}

export type AuthStatus = 'anonymous' | 'authenticated' | 'error'

export interface AuthContextValue {
  status: AuthStatus
  session: SessionState | null
  error: string | null
  login: (returnTo?: string) => void
  logout: () => void
  handleCallback: (
    code: string,
    state: string,
    oauthError?: string | null,
    errorDescription?: string | null,
  ) => Promise<string | null>
  hasScope: (scope: string) => boolean
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return context
}
