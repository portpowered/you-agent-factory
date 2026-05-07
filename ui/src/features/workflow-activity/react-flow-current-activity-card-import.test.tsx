import { fireEvent, render, screen } from "@testing-library/react";

import type { ReadFactoryImportPngError } from "../import";
import { getWorkflowActivityGraphImportMessages } from "./messages/graph-import";
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

  it("renders localized drag-active and reading overlay copy and hides idle state", () => {
    const japaneseMessages = getWorkflowActivityGraphImportMessages("ja");
    const { rerender } = render(<GraphDropOverlay dropState={{ status: "idle" }} />);

    expect(screen.queryByText(japaneseMessages.graphDropTitle)).toBeNull();

    rerender(<GraphDropOverlay dropState={{ status: "drag-active" }} locale="ja" />);

    expect(screen.getByText(japaneseMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(japaneseMessages.graphDropHint)).toBeTruthy();

    rerender(
      <GraphDropOverlay
        dropState={{ fileName: "factory.png", status: "reading" }}
        locale="ja"
      />,
    );

    expect(screen.getByText(japaneseMessages.graphImportLoadingTitle)).toBeTruthy();
    expect(screen.getByText(japaneseMessages.graphDropReadingMessage("factory.png"))).toBeTruthy();
  });

  it("falls back to default English overlay copy for unsupported locales", () => {
    const englishMessages = getWorkflowActivityGraphImportMessages("en");

    render(<GraphDropOverlay dropState={{ status: "drag-active" }} locale="fr-CA" />);

    expect(screen.getByText(englishMessages.graphDropTitle)).toBeTruthy();
    expect(screen.getByText(englishMessages.graphDropHint)).toBeTruthy();
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
      const englishMessages = getWorkflowActivityGraphImportMessages("en");
      render(
        <GraphImportErrorPanel
          error={{
            code,
            details,
            message: "Fallback error message.",
          } satisfies ReadFactoryImportPngError}
          fileName="factory.png"
          locale="en"
          onDismiss={vi.fn()}
        />,
      );

      expect(screen.getByRole("alert")).toBeTruthy();
      expect(screen.getByText(englishMessages.graphImportErrorTitle)).toBeTruthy();
      expect(screen.getByText("factory.png")).toBeTruthy();
      expect(screen.getByText(expectedMessage)).toBeTruthy();
    },
  );

  it("renders localized dismiss copy and falls back to the backend-provided error message", () => {
    const onDismiss = vi.fn();
    const japaneseMessages = getWorkflowActivityGraphImportMessages("ja");

    const { rerender } = render(
      <GraphImportErrorPanel
        error={{
          code: "NOT_PNG_FILE",
          message: "Custom browser validation failure.",
        }}
        fileName="factory.png"
        locale="ja"
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByText(japaneseMessages.importErrorNotPngFile)).toBeTruthy();

    rerender(
      <GraphImportErrorPanel
        error={{
          code: "UNKNOWN" as ReadFactoryImportPngError["code"],
          message: "Custom browser validation failure.",
        }}
        fileName="factory.png"
        locale="ja"
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByText("Custom browser validation failure.")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: japaneseMessages.dismissAction }),
    );

    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
