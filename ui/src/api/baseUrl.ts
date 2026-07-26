const API_ORIGIN_ENV = "VITE_AGENT_FACTORY_API_ORIGIN";
const runtimeAPIOriginKey = "__agentFactoryBrowserTestAPIOrigin";

function configuredAPIOrigin(): string {
  const runtimeConfig = globalThis as typeof globalThis &
    Record<typeof runtimeAPIOriginKey, string | undefined>;
  const runtimeAPIOrigin = runtimeConfig[runtimeAPIOriginKey];
  if (runtimeAPIOrigin) {
    return runtimeAPIOrigin.replace(/\/+$/, "");
  }
  return import.meta.env[API_ORIGIN_ENV]?.replace(/\/+$/, "") ?? "";
}

export function factoryAPIURL(path: string): string {
  const origin = configuredAPIOrigin();
  if (origin === "") {
    return path;
  }
  return `${origin}${path.startsWith("/") ? path : `/${path}`}`;
}
