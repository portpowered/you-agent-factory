import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render } from "@testing-library/react";
import type { ReactNode } from "react";

export function renderWithQueryClient(
  children: ReactNode,
  options: { supplementalReadDefaults?: boolean } = {},
): RenderResult {
  if (options.supplementalReadDefaults !== false) {
    installCanonicalSupplementalReadDefaults();
  }
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
  );
}

export function installCanonicalSupplementalReadDefaults(): void {
  const fetchMock = globalThis.fetch;
  if (
    typeof fetchMock !== "function" ||
    !("getMockImplementation" in fetchMock) ||
    !("mockImplementation" in fetchMock)
  ) {
    return;
  }

  const mock = fetchMock as typeof fetchMock & {
    getMockImplementation: () => typeof fetchMock | undefined;
    mockImplementation: (implementation: typeof fetchMock) => void;
  };
  const configuredFetch = mock.getMockImplementation();
  if (!configuredFetch) {
    return;
  }
  mock.mockImplementation(async (input, init) => {
    const response = await configuredFetch(input, init);
    const url = String(input);
    const dispatchMatch = url.match(
      /\/factory-sessions\/([^/?]+)\/dispatches$/,
    );
    if (
      dispatchMatch &&
      (response.status === 404 || !(await hasDispatches(response)))
    ) {
      return jsonResponse({
        dispatches: [],
        sessionId: decodeURIComponent(dispatchMatch[1]),
      });
    }

    const resultMatch = url.match(
      /\/factory-sessions\/([^/?]+)\/results\?mode=(?:final|partial)$/,
    );
    if (resultMatch && response.status === 404) {
      return jsonResponse({ sessionId: decodeURIComponent(resultMatch[1]) });
    }

    const liveResultMatch = url.match(
      /\/factory-sessions\/([^/?]+)\/(?:result|partial-result)$/,
    );
    if (
      liveResultMatch &&
      (response.status === 404 || (await isSessionReadResponse(response)))
    ) {
      return jsonResponse({
        sessionId: decodeURIComponent(liveResultMatch[1]),
      });
    }

    // Some panel fixtures intentionally reuse one Response for every request.
    // Clone it so supplemental reads do not consume the primary read's body.
    return response.clone();
  });
}

async function isSessionReadResponse(response: Response): Promise<boolean> {
  if (!response.ok) {
    return false;
  }
  try {
    const body = (await response.clone().json()) as {
      orchestratorKind?: unknown;
      runtime?: unknown;
    };
    return body.orchestratorKind !== undefined || body.runtime !== undefined;
  } catch {
    return false;
  }
}

async function hasDispatches(response: Response): Promise<boolean> {
  if (!response.ok) {
    return true;
  }
  try {
    const body = (await response.clone().json()) as { dispatches?: unknown };
    return Array.isArray(body.dispatches);
  } catch {
    return false;
  }
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}

export function createDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return {
    promise,
    reject,
    resolve,
  };
}
