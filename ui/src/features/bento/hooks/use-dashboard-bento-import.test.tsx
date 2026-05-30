import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  discoverSessionNamedFactoryNames,
  getCurrentFactory,
} from "../../../api/named-factory";
import type { FactoryImportPreviewState } from "../../import/hooks/use-factory-import-preview";
import { useDashboardBentoImport } from "./use-dashboard-bento-import";

vi.mock("../../../api/named-factory", async () => {
  const actual = await vi.importActual<typeof import("../../../api/named-factory")>(
    "../../../api/named-factory",
  );
  return {
    ...actual,
    discoverSessionNamedFactoryNames: vi.fn(actual.discoverSessionNamedFactoryNames),
    getCurrentFactory: vi.fn(actual.getCurrentFactory),
  };
});

const mockedGetCurrentFactory = vi.mocked(getCurrentFactory);
const mockedDiscoverSessionNamedFactoryNames = vi.mocked(discoverSessionNamedFactoryNames);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function createReadyImportController() {
  const readyPreview: Extract<FactoryImportPreviewState, { status: "ready" }> = {
    file: new File(["png"], "factory-import.png", { type: "image/png" }),
    status: "ready",
    value: {
      factory: {
        name: "Dropped Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      },
      previewImageSrc: "blob:factory-preview",
      revokePreviewImageSrc: vi.fn(),
      schemaVersion: "portos.agent-factory.png.v1",
    },
  };

  return {
    importPreviewState: readyPreview,
    activateImport: vi.fn(),
    cancelImport: vi.fn(),
    clearImportError: vi.fn(),
    importError: null,
    importSubmitting: false,
    openImportPreview: vi.fn(),
  };
}

describe("useDashboardBentoImport", () => {
  beforeEach(() => {
    mockedGetCurrentFactory.mockReset();
    mockedDiscoverSessionNamedFactoryNames.mockReset();
  });

  it("wires save-choice state and activation target queries when preview is ready", async () => {
    mockedGetCurrentFactory.mockResolvedValue({
      name: "Session Current",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    mockedDiscoverSessionNamedFactoryNames.mockResolvedValue(["Session Current"]);

    const { result } = renderHook(
      () =>
        useDashboardBentoImport({
          importController: createReadyImportController(),
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.importActivationTarget.isLoading).toBe(false);
    });

    expect(result.current.importSaveChoice).toBe("REPLACE_CURRENT");
    expect(result.current.importActivationTarget.createTargetFactoryName).toBe("Dropped Factory");
    expect(result.current.importActivationTarget.currentFactoryName).toBe("Session Current");
    expect(typeof result.current.setImportSaveChoice).toBe("function");
  });

  it("does not load activation targets while import preview is not ready", () => {
    const { result } = renderHook(
      () =>
        useDashboardBentoImport({
          importController: {
            ...createReadyImportController(),
            importPreviewState: { status: "idle" },
          },
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    expect(result.current.importActivationTarget.isLoading).toBe(false);
    expect(result.current.importActivationTarget.createTargetFactoryName).toBeNull();
    expect(mockedGetCurrentFactory).not.toHaveBeenCalled();
    expect(mockedDiscoverSessionNamedFactoryNames).not.toHaveBeenCalled();
  });
});
