import "../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";

import * as useDocDetailStateModule from "../hooks/use-doc-detail-state";
import type { DocDetailCardProps } from "../lib/detail-card-types";
import { DocDetailCard } from "./doc-detail-card";

vi.mock("../hooks/use-doc-detail-state");

const mockUseDocDetailState = vi.mocked(
  useDocDetailStateModule.useDocDetailState,
);

function buildReadyEditableState(
  overrides: Partial<
    Extract<
      NonNullable<DocDetailCardProps["editableConfigurationState"]>,
      { status: "ready" }
    >
  > = {},
) {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: true,
    draft: {
      fileName: "overview.md",
      inlineContent: "# Overview\n",
      originalExtension: ".md",
    },
    hasValidationErrors: false,
    initialValues: {
      fileName: "overview.md",
      inlineContent: "# Overview\n",
      targetPath: "factory/docs/overview.md",
    },
    isDirty: true,
    markChangesSaved: vi.fn(),
    onFileNameChange: vi.fn(),
    onInlineContentChange: vi.fn(),
    onResetToLatest: vi.fn(),
    originalTargetPath: "factory/docs/overview.md",
    overwriteFieldNames: [],
    pendingFactoryDefinition: null,
    pendingTargetPath: "factory/docs/overview.md",
    savedFactoryDefinition: {
      name: "Current Factory",
      workTypes: [],
    },
    status: "ready" as const,
    validationErrors: {},
    ...overrides,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: doc detail card state matrix stays in one readable suite.
describe("DocDetailCard editable configuration", () => {
  beforeEach(() => {
    mockUseDocDetailState.mockReturnValue({
      displayLabel: "overview.md",
      inlineContent: "# Overview\n",
      status: "ready",
      targetPath: "factory/docs/overview.md",
    });
  });

  it("renders rename and Monaco-backed content editing controls", () => {
    const onFileNameChange = vi.fn();
    const onInlineContentChange = vi.fn();

    render(
      <DocDetailCard
        editableConfigurationState={buildReadyEditableState({
          onFileNameChange,
          onInlineContentChange,
        })}
        saveState={{ status: "idle" }}
        targetPath="factory/docs/overview.md"
      />,
    );

    fireEvent.change(screen.getByDisplayValue("overview.md"), {
      target: { value: "guide.md" },
    });
    fireEvent.change(
      document.querySelector(
        '[data-monaco-editor="factory-doc-text"] textarea',
      ) as HTMLTextAreaElement,
      {
        target: { value: "# Guide\n" },
      },
    );

    expect(onFileNameChange).toHaveBeenCalledWith("guide.md");
    expect(onInlineContentChange).toHaveBeenCalledWith("# Guide\n");
  });

  it("keeps focus in the inline content editor while local draft text changes", () => {
    function FocusRetentionHarness() {
      const [draft, setDraft] = useState("# Overview\n");

      return (
        <DocDetailCard
          editableConfigurationState={buildReadyEditableState({
            draft: {
              fileName: "overview.md",
              inlineContent: draft,
              originalExtension: ".md",
            },
            onInlineContentChange: setDraft,
          })}
          saveState={{ status: "idle" }}
          targetPath="factory/docs/overview.md"
        />
      );
    }

    render(<FocusRetentionHarness />);

    const editorTextarea = document.querySelector(
      '[data-monaco-editor="factory-doc-text"] textarea',
    ) as HTMLTextAreaElement;

    editorTextarea.focus();
    fireEvent.change(editorTextarea, {
      target: { value: "# Overview updated\n" },
    });

    expect(document.activeElement).toBe(editorTextarea);
    expect(
      document.querySelector(
        '[data-monaco-editor="factory-doc-text"] textarea',
      ),
    ).toBe(editorTextarea);
  });

  it("renders loading, error, and empty detail states before editable configuration is ready", () => {
    mockUseDocDetailState.mockReturnValue({
      status: "loading",
      targetPath: "factory/docs/overview.md",
    });
    const { rerender } = render(
      <DocDetailCard targetPath="factory/docs/overview.md" />,
    );
    expect(screen.getByText("Loading doc details…")).toBeTruthy();

    mockUseDocDetailState.mockReturnValue({
      errorMessage: "Network dropped",
      status: "error",
      targetPath: "factory/docs/overview.md",
    });
    rerender(<DocDetailCard targetPath="factory/docs/overview.md" />);
    expect(screen.getByText(/Unable to load the selected doc\./)).toBeTruthy();
    expect(screen.getByText(/Network dropped/)).toBeTruthy();

    mockUseDocDetailState.mockReturnValue({
      status: "empty",
      targetPath: "factory/docs/overview.md",
    });
    rerender(<DocDetailCard targetPath="factory/docs/overview.md" />);
    expect(
      screen.getByText(
        "This doc is no longer attached to the current factory.",
      ),
    ).toBeTruthy();
  });

  it("renders editable configuration loading, error, and empty states", () => {
    const { rerender } = render(
      <DocDetailCard
        editableConfigurationState={{ status: "loading" }}
        targetPath="factory/docs/overview.md"
      />,
    );
    expect(
      screen.getByText("Loading editable doc configuration."),
    ).toBeTruthy();

    rerender(
      <DocDetailCard
        editableConfigurationState={{
          errorMessage: "Contract mismatch",
          status: "error",
        }}
        targetPath="factory/docs/overview.md"
      />,
    );
    expect(screen.getByText(/Doc configuration unavailable\./)).toBeTruthy();
    expect(screen.getByText(/Contract mismatch/)).toBeTruthy();

    rerender(
      <DocDetailCard
        editableConfigurationState={{
          message: "Doc removed from factory",
          status: "empty",
        }}
        targetPath="factory/docs/overview.md"
      />,
    );
    expect(screen.getByText("Doc removed from factory")).toBeTruthy();
  });

  it("surfaces validation errors and save feedback in the editor", () => {
    render(
      <DocDetailCard
        editableConfigurationState={buildReadyEditableState({
          hasValidationErrors: true,
          validationErrors: {
            fileName: "Enter a valid file name under factory/docs/.",
            inlineContent: "Enter doc content before saving.",
          },
        })}
        saveState={{
          errorMessage: "Factory definition is invalid.",
          status: "error",
        }}
        targetPath="factory/docs/overview.md"
      />,
    );

    expect(
      screen.getByText("Enter a valid file name under factory/docs/."),
    ).toBeTruthy();
    expect(screen.getByText("Enter doc content before saving.")).toBeTruthy();
    expect(
      screen.getByText(
        /Resolve the highlighted fields before saving this doc\./,
      ),
    ).toBeTruthy();
    expect(screen.getByText(/Saving failed\./)).toBeTruthy();
    expect(screen.getByText(/Factory definition is invalid\./)).toBeTruthy();
  });

  it("shows stale-version warning feedback without field errors", () => {
    render(
      <DocDetailCard
        editableConfigurationState={buildReadyEditableState()}
        saveState={{
          message: "The factory definition changed on the server.",
          status: "warning",
        }}
        targetPath="factory/docs/overview.md"
      />,
    );

    expect(
      screen.getByText("The factory definition changed on the server."),
    ).toBeTruthy();
    expect(
      screen.getByText(
        /Reload the latest running-factory values or keep this draft/,
      ),
    ).toBeTruthy();
  });

  it("falls back to the doc kind label when detail is ready without editable configuration", () => {
    render(<DocDetailCard targetPath="factory/docs/overview.md" />);

    expect(screen.getByText(/Factory doc:/)).toBeTruthy();
    expect(screen.getByText(/factory\/docs\/overview\.md/)).toBeTruthy();
  });
});
