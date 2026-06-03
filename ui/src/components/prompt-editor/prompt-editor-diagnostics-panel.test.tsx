import { render, screen } from "@testing-library/react";

import { PromptEditorDiagnosticsPanel } from "./prompt-editor-diagnostics-panel";

const labels = {
  diagnosticsHeading: "Prompt diagnostics",
  diagnosticsSummary:
    "Resolve the highlighted prompt diagnostics before saving this workstation.",
  validationErrorPrefix: "Prompt validation unavailable.",
  validationLoading: "Validating prompt variables for the current draft.",
  variableDiagnosticLabel: "Variable access",
};

describe("PromptEditorDiagnosticsPanel", () => {
  it("keeps validation loading quiet and renders error states", () => {
    const { rerender } = render(
      <PromptEditorDiagnosticsPanel
        diagnostics={[]}
        id="prompt-diagnostics"
        labels={labels}
        validationState={{ status: "loading" }}
      />,
    );

    const loadingPanel = document.getElementById("prompt-diagnostics");
    expect(
      screen.queryByText("Validating prompt variables for the current draft."),
    ).toBeNull();
    expect(loadingPanel?.getAttribute("aria-hidden")).toBe("true");
    expect(loadingPanel?.className).toContain("min-h-24");

    rerender(
      <PromptEditorDiagnosticsPanel
        diagnostics={[]}
        id="prompt-diagnostics"
        labels={labels}
        validationState={{
          errorMessage: "Prompt validation API unavailable.",
          status: "error",
        }}
      />,
    );

    expect(
      screen.getByText(
        "Prompt validation unavailable. Prompt validation API unavailable.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("alert").textContent).toContain(
      "Prompt validation API unavailable.",
    );
  });

  it("renders line-based syntax diagnostics without duplicate summary detail", () => {
    render(
      <PromptEditorDiagnosticsPanel
        diagnostics={[
          {
            endOffset: 18,
            kind: "SYNTAX_ERROR",
            message: "line 2: unexpected EOF in if block",
            sourceText: "{{ if .WorkID }}",
            startOffset: 5,
          },
        ]}
        id="prompt-diagnostics"
        labels={labels}
        validationState={{ status: "ready" }}
      />,
    );

    const panel = document.getElementById("prompt-diagnostics");
    expect(panel).toBeTruthy();
    expect(panel?.getAttribute("role")).toBe("alert");
    expect(screen.getByText("Prompt diagnostics")).toBeTruthy();
    expect(screen.getByText("line 2: unexpected EOF in if block")).toBeTruthy();
    expect(
      screen.queryByText("Template syntax: unexpected EOF in if block"),
    ).toBeNull();
    expect(
      screen.queryByText("Fix each issue below before saving."),
    ).toBeNull();
    expect(screen.getByText("{{ if .WorkID }}")).toBeTruthy();
  });

  it("keeps an inert reserved region when validation is idle and there are no diagnostics", () => {
    render(
      <PromptEditorDiagnosticsPanel
        diagnostics={[]}
        id="prompt-diagnostics"
        labels={labels}
        validationState={{ status: "idle" }}
      />,
    );

    const panel = document.getElementById("prompt-diagnostics");
    expect(panel).toBeTruthy();
    expect(panel?.getAttribute("aria-hidden")).toBe("true");
    expect(panel?.getAttribute("role")).toBeNull();
    expect(panel?.className).toContain("min-h-24");
    expect(panel?.className).toContain("border-transparent");
  });
});
