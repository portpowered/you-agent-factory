export type RenderAppFetchOverride = (
  path: string,
  method: string,
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response | undefined>;
