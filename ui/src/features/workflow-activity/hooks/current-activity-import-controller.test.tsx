import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { activateImportedFactoryForSession } from "../../../api/named-factory";
import { PORT_OS_FACTORY_PNG_SCHEMA_VERSION } from "../../import/lib/factory-png-import";
import { useCurrentActivityImportController } from "./current-activity-import-controller";

vi.mock("../../../api/named-factory", async () => {
  const actual = await vi.importActual<typeof import("../../../api/named-factory")>(
    "../../../api/named-factory",
  );
  return {
    ...actual,
    activateImportedFactoryForSession: vi.fn(actual.activateImportedFactoryForSession),
  };
});

const mockedActivateImportedFactoryForSession = vi.mocked(activateImportedFactoryForSession);

const canonicalFactory = {
  id: "agent-factory",
  name: "Imported Factory",
  workTypes: [],
  workers: [],
  workstations: [],
};

describe("useCurrentActivityImportController", () => {
  beforeEach(() => {
    mockedActivateImportedFactoryForSession.mockClear();
    mockedActivateImportedFactoryForSession.mockResolvedValue(canonicalFactory);
  });

  it("forwards sessionID into session-scoped import activation", async () => {
    const { result } = renderHook(
      () => useCurrentActivityImportController({ sessionID: "session-2" }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport({
        factory: canonicalFactory,
        previewImageSrc: "blob:preview",
        revokePreviewImageSrc: vi.fn(),
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      });
    });

    await waitFor(() => {
      expect(mockedActivateImportedFactoryForSession).toHaveBeenCalledWith(
        canonicalFactory,
        { sessionID: "session-2" },
      );
    });
  });
});

function createQueryClientWrapper(): ({ children }: { children: ReactNode }) => ReactNode {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }): ReactNode {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}
