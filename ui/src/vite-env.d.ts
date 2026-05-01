/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_AGENT_FACTORY_API_ORIGIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
