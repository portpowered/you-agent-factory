import { fireEvent, render, screen } from "@testing-library/react";

import { DocDetailCard } from "./doc-detail-card";

vi.mock("../hooks/use-doc-detail-state", () => ({
  useDocDetailState: () => ({
    displayLabel: "overview.md",
    inlineContent: "# Overview\n",
    status: "ready",
    targetPath: "factory/docs/overview.md",
  }),
}));

describe("DocDetailCard editable configuration", () => {
  it("renders rename and Monaco-backed content editing controls", () => {
    const onFileNameChange = vi.fn();
    const onInlineContentChange = vi.fn();

    render(
      <DocDetailCard
        editableConfigurationState={{
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
          onFileNameChange,
          onInlineContentChange,
          onResetToLatest: vi.fn(),
          originalTargetPath: "factory/docs/overview.md",
          overwriteFieldNames: [],
          pendingFactoryDefinition: null,
          pendingTargetPath: "factory/docs/overview.md",
          savedFactoryDefinition: {
            name: "Current Factory",
            workTypes: [],
          },
          status: "ready",
          validationErrors: {},
        }}
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
});
