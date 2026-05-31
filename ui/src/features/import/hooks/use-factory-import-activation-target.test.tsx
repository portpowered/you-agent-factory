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
    vi.clearAllMocks();
  });

  it("resolves create target names for non-default sessions", async () => {
    vi.mocked(getCurrentFactory).mockResolvedValue({
      name: "Review Session Import Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    vi.mocked(discoverSessionNamedFactoryNames).mockResolvedValue([
      "Review Session Import Factory",
    ]);

    const { result } = renderHook(
      () =>
        useFactoryImportActivationTarget({
          enabled: true,
          preferredFactoryName: "Review Session Import Factory",
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.createTargetFactoryName).toBe(
        "Review Session Import Factory-2",
      );
    });
    expect(result.current.currentFactoryName).toBe("Review Session Import Factory");
    expect(discoverSessionNamedFactoryNames).toHaveBeenCalledWith({
      sessionID: "session-review",
    });
  });

  it("returns null create target names when the preferred name is blank", async () => {
    vi.mocked(getCurrentFactory).mockResolvedValue({
      name: "alpha",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    vi.mocked(discoverSessionNamedFactoryNames).mockResolvedValue(["alpha"]);

    const { result } = renderHook(
      () =>
        useFactoryImportActivationTarget({
          enabled: true,
          preferredFactoryName: "   ",
          sessionID: "~default",
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.createTargetFactoryName).toBeNull();
    expect(result.current.replacesExistingCreateTarget).toBe(false);
  });

  it("skips discovery queries when disabled", () => {
    const { result } = renderHook(
      () =>
        useFactoryImportActivationTarget({
          enabled: false,
          preferredFactoryName: "Dropped Factory",
          sessionID: "session-review",
        }),
      { wrapper: createWrapper() },
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.createTargetFactoryName).toBe("Dropped Factory");
    expect(result.current.currentFactoryName).toBeNull();
    expect(getCurrentFactory).not.toHaveBeenCalled();
    expect(discoverSessionNamedFactoryNames).not.toHaveBeenCalled();
  });
});
