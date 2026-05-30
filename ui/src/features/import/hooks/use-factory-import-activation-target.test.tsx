import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  discoverSessionNamedFactoryNames,
  getCurrentFactory,
} from "../../../api/named-factory";
import { useFactoryImportActivationTarget } from "./use-factory-import-activation-target";

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

describe("useFactoryImportActivationTarget", () => {
  beforeEach(() => {
    mockedGetCurrentFactory.mockReset();
    mockedDiscoverSessionNamedFactoryNames.mockReset();
  });

  it("resolves create-target metadata from session queries", async () => {
    mockedGetCurrentFactory.mockResolvedValue({
      name: "Session Current",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    mockedDiscoverSessionNamedFactoryNames.mockResolvedValue([
      "Session Current",
      "Dropped Factory",
    ]);

    const { result } = renderHook(
      () =>
        useFactoryImportActivationTarget({
          preferredFactoryName: "Dropped Factory",
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.currentFactoryName).toBe("Session Current");
    expect(result.current.createTargetFactoryName).toBe("Dropped Factory-2");
    expect(result.current.replacesExistingCreateTarget).toBe(false);
    expect(result.current.existingNamedFactoryNames).toEqual([
      "Session Current",
      "Dropped Factory",
    ]);
  });

  it("omits create-target metadata when the preferred name is blank", async () => {
    mockedGetCurrentFactory.mockResolvedValue({
      name: "Session Current",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    mockedDiscoverSessionNamedFactoryNames.mockResolvedValue([]);

    const { result } = renderHook(
      () =>
        useFactoryImportActivationTarget({
          preferredFactoryName: "   ",
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.createTargetFactoryName).toBeNull();
    expect(result.current.replacesExistingCreateTarget).toBe(false);
  });
});
