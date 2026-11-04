import { apiUrl } from './config'
import type {
  AdminEntry,
  ApiResponse,
  AuthClient,
  CreateClientPayload,
  CreateClientResult,
  MeResponse,
  User,
  Page,
  ListQuery,
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

type ClientResponse = Omit<AuthClient, 'scopes' | 'redirect_uris' | 'grant_types'> & {
  scopes: string[] | null
  redirect_uris: string[] | null
  grant_types: string[] | null
}

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

  private listUrl(path: string, query: ListQuery): string {
    return `${path}?${new URLSearchParams({ page: String(query.page), page_size: String(query.page_size), q: query.q, status: query.status })}`
  }

  getMe(accessToken?: string): Promise<MeResponse> {
    return this.request<MeResponse>('/api/admin/me', accessToken
      ? { headers: { Authorization: `Bearer ${accessToken}` } }
      : {})
  }

  // Users
  getUsers(query: ListQuery): Promise<Page<User>> {
    return this.request<Page<User>>(this.listUrl('/api/admin/users', query))
  }

  updateUser(id: number, data: Partial<Pick<User, 'email' | 'display_name' | 'is_disabled'>>): Promise<void> {
    return this.request(`/api/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify(data) })
  }

  resetUserPassword(id: number, password: string): Promise<void> {
    return this.request(`/api/admin/users/${id}/password`, { method: 'POST', body: JSON.stringify({ password }) })
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
  async getClients(query: ListQuery): Promise<Page<AuthClient>> {
    const page = await this.request<Page<ClientResponse>>(this.listUrl('/api/admin/clients', query))
    return {
      ...page,
      items: page.items.map((client) => ({
        ...client,
        scopes: client.scopes ?? [],
        redirect_uris: client.redirect_uris ?? [],
        grant_types: client.grant_types ?? [],
      })),
    }
  }

  updateClient(id: number, data: Omit<CreateClientPayload, 'secret'>): Promise<void> {
    return this.request(`/api/admin/clients/${id}`, { method: 'PUT', body: JSON.stringify(data) })
  }

  setClientDisabled(id: number, is_disabled: boolean): Promise<void> {
    return this.request(`/api/admin/clients/${id}/state`, { method: 'PATCH', body: JSON.stringify({ is_disabled }) })
  }

  resetClientSecret(id: number): Promise<CreateClientResult> {
    return this.request(`/api/admin/clients/${id}/secret`, { method: 'POST' })
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
  getAdmins(query: ListQuery): Promise<Page<AdminEntry>> {
    return this.request<Page<AdminEntry>>(this.listUrl('/api/admin/admins', query))
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
  getSessions(query: ListQuery): Promise<Page<User>> {
    return this.request<Page<User>>(this.listUrl('/api/admin/sessions', query))
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
