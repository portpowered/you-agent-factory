import { vi } from "vitest";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../../../factory-graph-editor/hooks/use-editable-factory-graph",
  async () => {
    const actual = await vi.importActual(
      "../../../factory-graph-editor/hooks/use-editable-factory-graph",
    );

    return {
      ...actual,
      useEditableFactoryGraph: vi.fn(),
    };
  },
);

vi.mock(
  "../../../factory-graph-editor/hooks/factory-graph-draft-hook",
  async () => {
    const actual = await vi.importActual(
      "../../../factory-graph-editor/hooks/factory-graph-draft-hook",
    );

    return {
      ...actual,
      useFactoryGraphDraftState: vi.fn(),
    };
  },
);

vi.mock(
  "../../../../components/ui/dialog",
  () => import("../../../../testing/mock-dashboard-dialog"),
);
