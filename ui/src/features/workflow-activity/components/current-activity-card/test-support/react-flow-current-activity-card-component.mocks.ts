import { vi } from "vitest";

vi.mock(
  "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../../../../factory-graph-editor/hooks/use-editable-factory-graph",
  async () => {
    const actual = await vi.importActual(
      "../../../../factory-graph-editor/hooks/use-editable-factory-graph",
    );

    return {
      ...actual,
      useEditableFactoryGraph: vi.fn(),
    };
  },
);

vi.mock(
  "../../../../factory-graph-editor/hooks/factory-graph-draft-hook",
  async () => {
    const actual = await vi.importActual(
      "../../../../factory-graph-editor/hooks/factory-graph-draft-hook",
    );

    return {
      ...actual,
      useFactoryGraphDraftState: vi.fn(),
    };
  },
);

vi.mock("@you-agent-factory/components/overlays", async (importOriginal) => {
  const actual =
    await importOriginal<
      typeof import("@you-agent-factory/components/overlays")
    >();
  const mockDialog = await import(
    "../../../../../testing/mock-dashboard-dialog"
  );

  return {
    ...actual,
    Dialog: mockDialog.Dialog,
    DialogContent: mockDialog.DialogContent,
    DialogDescription: mockDialog.DialogDescription,
    DialogFooter: mockDialog.DialogFooter,
    DialogHeader: mockDialog.DialogHeader,
    DialogOverlay: mockDialog.DialogOverlay,
    DialogPortal: mockDialog.DialogPortal,
    DialogTitle: mockDialog.DialogTitle,
  };
});
