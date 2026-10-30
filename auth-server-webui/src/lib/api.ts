import { apiUrl } from './config'
import type {
  AdminEntry,
  ApiResponse,
  AuthClient,
  CreateClientPayload,
  CreateClientResult,
  MeResponse,
  User,
} from '../types'

export class ApiError extends Error {
  readonly status: number
  readonly code?: number

  constructor(message: string, status: number, code?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type UnauthorizedHandler = () => Promise<string | null>

class ApiClient {
  private getToken: () => string | null = () => null
  private onUnauthorized: UnauthorizedHandler | null = null

  setTokenReader(reader: () => string | null) {
    this.getToken = reader
  }

  setUnauthorizedHandler(handler: UnauthorizedHandler | null) {
    this.onUnauthorized = handler
  }

  private async request<T>(path: string, options: RequestInit = {}, retried = false): Promise<T> {
    const headers = new Headers(options.headers)
    headers.set('Accept', 'application/json')
    if (options.body && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json')
    }
    const token = this.getToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)

    const response = await fetch(apiUrl(path), { ...options, headers })

    if (response.status === 401 && !retried && this.onUnauthorized) {
      const nextToken = await this.onUnauthorized().catch(() => null)
      if (nextToken) {
        return this.request<T>(path, options, true)
      }
    }

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
      const message = extractErrorMessage(payload) || `HTTP ${response.status}`
      throw new ApiError(message, response.status, extractErrorCode(payload))
    }

    if (payload && typeof payload === 'object' && 'code' in payload) {
      const envelope = payload as ApiResponse<T>
      if (envelope.code !== 0) {
        throw new ApiError(envelope.msg || 'API error', response.status, envelope.code)
      }
      return envelope.data
    }

    return payload as T
  }

  getMe(accessToken?: string): Promise<MeResponse> {
    return this.request<MeResponse>('/api/admin/me', accessToken
      ? { headers: { Authorization: `Bearer ${accessToken}` } }
      : {})
  }

  // Users
  getUsers(): Promise<User[]> {
    return this.request<User[]>('/api/admin/users')
  }

  createUser(email: string, displayName: string, password: string): Promise<void> {
    return this.request<void>('/api/admin/users', {
      method: 'POST',
      body: JSON.stringify({ email, display_name: displayName, password }),
    })
  }

  deleteUser(id: number): Promise<void> {
    return this.request<void>(`/api/admin/users/${id}`, { method: 'DELETE' })
  }

  // Clients
  getClients(): Promise<AuthClient[]> {
    return this.request<AuthClient[]>('/api/admin/clients')
  }

  createClient(data: CreateClientPayload): Promise<CreateClientResult> {
    return this.request<CreateClientResult>('/api/admin/clients', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  deleteClient(id: number): Promise<void> {
    return this.request<void>(`/api/admin/clients/${id}`, { method: 'DELETE' })
  }

  // Admins
  getAdmins(): Promise<AdminEntry[]> {
    return this.request<AdminEntry[]>('/api/admin/admins')
  }

  addAdmin(userId: number): Promise<void> {
    return this.request<void>('/api/admin/admins', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId }),
    })
  }

  removeAdmin(userId: number): Promise<void> {
    return this.request<void>(`/api/admin/admins/${userId}`, { method: 'DELETE' })
  }

  // Sessions are user records until the Phase 4 token/session index exists.
  getSessions(): Promise<User[]> {
    return this.request<User[]>('/api/admin/sessions')
  }

  revokeUserSessions(userId: number): Promise<void> {
    return this.request<void>(`/api/admin/sessions/${userId}`, { method: 'DELETE' })
  }
}

function extractErrorMessage(payload: unknown): string | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }
  const record = payload as Record<string, unknown>
  if (typeof record.msg === 'string') {
    return record.msg
  }
  if (typeof record.error_description === 'string') {
    return record.error_description
  }
  if (typeof record.error === 'string') {
    return record.error
  }
  return null
}

function extractErrorCode(payload: unknown): number | undefined {
  if (!payload || typeof payload !== 'object') {
    return undefined
  }
  const code = (payload as Record<string, unknown>).code
  return typeof code === 'number' ? code : undefined
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message
  }
  return String(error)
}

export const api = new ApiClient()
