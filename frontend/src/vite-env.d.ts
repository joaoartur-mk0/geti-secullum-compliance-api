/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Endereço base da API do backend. Resolvido em build time. */
  readonly VITE_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
