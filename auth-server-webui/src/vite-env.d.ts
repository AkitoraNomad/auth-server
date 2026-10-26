/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_ADMIN_CLIENT_ID?: string
  readonly VITE_AUTH_BASE_URL?: string
  readonly VITE_API_BASE_URL?: string
  readonly VITE_ADMIN_REDIRECT_URI?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
