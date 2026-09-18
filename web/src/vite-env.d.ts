/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Overrides main.ts's default relay URL - set in a local .env.local
   *  (not committed) to point a dev server at a local relay. */
  readonly VITE_RELAY_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
