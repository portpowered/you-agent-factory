import type { Mock } from "vitest";
import { fetchRequestPath } from "./app-shell-session-stream-test-utils";

export type FetchMock = Mock<
  (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
>;

export type RenderAppFetchOverride = (
  path: string,
  method: string,
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response | undefined>;

export function fetchCallPaths(fetchMock: FetchMock) {
  return fetchMock.mock.calls.map(([input]) =>
    typeof input === "string"
      ? input
      : input instanceof URL
        ? `${input.pathname}${input.search}`
        : input.url,
  );
}

export function nonPromptTemplateFetchPaths(fetchMock: FetchMock) {
  return fetchCallPaths(fetchMock).filter(
    (path) =>
      !path.includes("/prompt-template-contract") &&
      !path.includes("/prompt-template-validation") &&
      path !== "/factory-sessions" &&
      !/^\/factory-sessions\/[^/]+$/.test(path) &&
      !/^\/factory-sessions\/[^/]+\/sync-preflight(?:\?.*)?$/.test(path) &&
      !path.endsWith("/factory"),
  );
}

export function chainRenderAppFetchMock(
  fetchMock: FetchMock,
  override: RenderAppFetchOverride,
): void {
  const defaultHandler = fetchMock.getMockImplementation();
  if (defaultHandler == null) {
    throw new Error("fetchMock has no default implementation");
  }

  fetchMock.mockImplementation(async (input, init) => {
    const path = fetchRequestPath(input);
    const method = (init?.method ?? "GET").toUpperCase();
    const overridden = await override(path, method, input, init);
    if (overridden !== undefined) {
      return overridden;
    }

    return defaultHandler(input, init);
  });
}

export function jsonResponse(
  body: unknown,
  status = 200,
  statusText?: string,
): Response {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status,
    statusText,
  });
}
