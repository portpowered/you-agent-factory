import { act, renderHook } from "@testing-library/react";

import type { FactoryImportPreviewState } from "./use-factory-import-preview";
import { useFactoryImportSaveChoice } from "./use-factory-import-save-choice";

function createReadyPreview(): Extract<FactoryImportPreviewState, { status: "ready" }> {
  return {
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
}

describe("useFactoryImportSaveChoice", () => {
  it("defaults to REPLACE_CURRENT when no preview is ready", () => {
    const { result } = renderHook(() => useFactoryImportSaveChoice(null));

    expect(result.current[0]).toBe("REPLACE_CURRENT");
  });

  it("resets to REPLACE_CURRENT when a ready preview appears", () => {
    const readyPreview = createReadyPreview();
    const { result, rerender } = renderHook(
      ({ preview }: { preview: Extract<FactoryImportPreviewState, { status: "ready" }> | null }) =>
        useFactoryImportSaveChoice(preview),
      { initialProps: { preview: null as Extract<FactoryImportPreviewState, { status: "ready" }> | null } },
    );

    act(() => {
      result.current[1]("CREATE_NEW_NAMED");
    });
    expect(result.current[0]).toBe("CREATE_NEW_NAMED");

    rerender({ preview: readyPreview });

    expect(result.current[0]).toBe("REPLACE_CURRENT");
  });
});
