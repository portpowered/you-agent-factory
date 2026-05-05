import { fireEvent, render, screen } from "@testing-library/react";

import type { ReadFactoryImportPngError } from "../import";
import {
  GraphDropOverlay,
  GraphImportErrorPanel,
  graphDropStateAttribute,
} from "./react-flow-current-activity-card-import";

describe("react-flow-current-activity-card-import", () => {
  it("exposes the drop state status for graph viewport attributes", () => {
    expect(graphDropStateAttribute({ status: "idle" })).toBe("idle");
    expect(graphDropStateAttribute({ status: "drag-active" })).toBe("drag-active");
    expect(graphDropStateAttribute({ fileName: "factory.png", status: "reading" })).toBe("reading");
    expect(
      graphDropStateAttribute({
        error: {
          code: "PNG_INVALID",
          message: "The selected PNG image is invalid or truncated.",
        },
        fileName: "factory.png",
        status: "error",
      }),
    ).toBe("error");
  });

  it("renders drag-active and reading overlay copy and hides idle state", () => {
    const { rerender } = render(<GraphDropOverlay dropState={{ status: "idle" }} />);

    expect(screen.queryByText("Import factory PNG")).toBeNull();

    rerender(<GraphDropOverlay dropState={{ status: "drag-active" }} />);

    expect(screen.getByText("Import factory PNG")).toBeTruthy();
    expect(screen.getByText("Drop an Infinite You PNG onto this graph to start import.")).toBeTruthy();

    rerender(
      <GraphDropOverlay dropState={{ fileName: "factory.png", status: "reading" }} />,
    );

    expect(screen.getByText("Validating factory PNG")).toBeTruthy();
    expect(
      screen.getByText(
        "factory.png is being parsed and validated locally before import continues.",
      ),
    ).toBeTruthy();
  });

  it.each([
    [
      "NOT_PNG_FILE",
      undefined,
      "Drop a PNG image exported by Infinite You.",
    ],
    [
      "PNG_METADATA_MISSING",
      undefined,
      "This PNG does not include the Infinite You factory metadata needed for import.",
    ],
    [
      "UNSUPPORTED_SCHEMA_VERSION",
      { schemaVersion: "portos.agent-factory.png.v9" },
      "This PNG uses unsupported Infinite You factory metadata version portos.agent-factory.png.v9.",
    ],
    [
      "UNSUPPORTED_SCHEMA_VERSION",
      undefined,
      "This PNG uses an unsupported Infinite You factory metadata version.",
    ],
    [
      "PNG_METADATA_INVALID",
      undefined,
      "The embedded Infinite You factory metadata is invalid, so the current factory was left unchanged.",
    ],
    [
      "FACTORY_PAYLOAD_INVALID",
      undefined,
      "The embedded Infinite You factory metadata is invalid, so the current factory was left unchanged.",
    ],
    [
      "IMAGE_DECODE_FAILED",
      undefined,
      "The browser could not validate this PNG for import preview, so the current factory was left unchanged.",
    ],
    [
      "PREVIEW_UNAVAILABLE",
      undefined,
      "The browser could not validate this PNG for import preview, so the current factory was left unchanged.",
    ],
    [
      "FILE_READ_FAILED",
      undefined,
      "The browser could not read the dropped file. Try dropping the PNG again.",
    ],
    [
      "PNG_INVALID",
      undefined,
      "This PNG appears truncated or malformed, so import stopped before any activation request.",
    ],
  ] as const)(
    "renders import error copy for %s",
    (code, details, expectedMessage) => {
      render(
        <GraphImportErrorPanel
          error={{
            code,
            details,
            message: "Fallback error message.",
          } satisfies ReadFactoryImportPngError}
          fileName="factory.png"
          onDismiss={vi.fn()}
        />,
      );

      expect(screen.getByRole("alert")).toBeTruthy();
      expect(screen.getByText("Factory import failed")).toBeTruthy();
      expect(screen.getByText("factory.png")).toBeTruthy();
      expect(screen.getByText(expectedMessage)).toBeTruthy();
    },
  );

  it("falls back to the backend-provided error message and dismisses the panel", () => {
    const onDismiss = vi.fn();

    const { rerender } = render(
      <GraphImportErrorPanel
        error={{
          code: "NOT_PNG_FILE",
          message: "Custom browser validation failure.",
        }}
        fileName="factory.png"
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByText("Drop a PNG image exported by Infinite You.")).toBeTruthy();

    rerender(
      <GraphImportErrorPanel
        error={{
          code: "UNKNOWN" as ReadFactoryImportPngError["code"],
          message: "Custom browser validation failure.",
        }}
        fileName="factory.png"
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByText("Custom browser validation failure.")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
