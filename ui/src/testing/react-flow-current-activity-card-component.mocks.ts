import { vi } from "vitest";

vi.mock(
  "../features/current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../features/current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../features/current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../features/factory-graph-editor/hooks/use-editable-factory-graph",
  async () => {
    const actual = await vi.importActual(
      "../features/factory-graph-editor/hooks/use-editable-factory-graph",
    );

    return {
      ...actual,
      useEditableFactoryGraph: vi.fn(),
    };
  },
);

vi.mock(
  "../features/factory-graph-editor/hooks/factory-graph-draft-hook",
  async () => {
    const actual = await vi.importActual(
      "../features/factory-graph-editor/hooks/factory-graph-draft-hook",
    );

    return {
      ...actual,
      useFactoryGraphDraftState: vi.fn(),
    };
  },
);

vi.mock(
  "../components/ui/dialog",
  () => import("./mock-dashboard-dialog"),
);
