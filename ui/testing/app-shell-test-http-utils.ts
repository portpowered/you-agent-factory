import { vi, type Mock } from "bun:test";
import type { FactoryPngImportValue } from "../src/features/import/lib/factory-png-import";

type FetchMock = Mock<
  (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
>;

function fetchCallPaths(fetchMock: FetchMock) {
  return fetchMock.mock.calls.map(([input]: [RequestInfo | URL]) =>
    typeof input === "string"
      ? input
      : input instanceof URL
        ? `${input.pathname}${input.search}`
        : input.url,
  );
}

export function nonPromptTemplateFetchPaths(fetchMock: FetchMock) {
  return fetchCallPaths(fetchMock).filter(
    (path: string) =>
      !path.includes("/prompt-template-contract") &&
      path !== "/factory-sessions",
  );
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

export function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: vi.fn(),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

export function createFileDropTransfer(files: File[]): {
  dataTransfer: {
    dropEffect: string;
    files: File[];
    types: string[];
  };
} {
  return {
    dataTransfer: {
      dropEffect: "none",
      files,
      types: ["Files"],
    },
  };
}

export type { FetchMock };
