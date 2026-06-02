const API_ORIGIN_ENV = "VITE_AGENT_FACTORY_API_ORIGIN";

function configuredAPIOrigin(): string {
  return import.meta.env[API_ORIGIN_ENV]?.replace(/\/+$/, "") ?? "";
}

export function factoryAPIURL(path: string): string {
  const origin = configuredAPIOrigin();
  if (origin === "") {
    return path;
  }
  return `${origin}${path.startsWith("/") ? path : `/${path}`}`;
}
