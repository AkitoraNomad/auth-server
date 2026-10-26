export interface User {
  id: number
  email: string
  display_name: string
  last_login_at?: string
}

export interface AuthClient {
  id: number
  display_name: string
  domain: string
  scopes: string[]
  token_type: number | null
  is_public: boolean
  redirect_uris: string[]
  grant_types: string[]
  created_at?: string
  updated_at?: string
}

export interface AdminEntry {
  id: number
  user_id: number
  email: string
}

export interface MeResponse {
  user: User
  scopes: string[]
  client_id: string
}

export interface TokenResponse {
  access_token: string
  refresh_token?: string
  expires_in: number
  token_type: string
  scope?: string
}

export interface CreateClientPayload {
  display_name: string
  domain: string
  secret: string
  scopes: string[]
  token_type: number
  is_public: boolean
  redirect_uris: string[]
  grant_types: string[]
}

export interface CreateClientResult {
  id: number
  secret: string
}

export interface ApiResponse<T> {
  code: number
  data: T
  msg?: string
}
