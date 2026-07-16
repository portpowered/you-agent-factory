import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import { type ReactNode, useEffect } from "react";

import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

vi.mock(
  "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-layer",
  () => ({
    FactoryGraphVisualGroupLayer: ({
      groups,
      selectedGroupId,
    }: {
      groups: readonly { id: string }[];
      selectedGroupId: string | null;
    }) => (
      <div
        data-selected-group-id={selectedGroupId ?? ""}
        data-testid="factory-visual-group-layer"
      >
        {groups.map((group) => group.id).join(",")}
      </div>
    ),
  }),
);

vi.mock(
  "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-controls",
  () => ({
    FactoryGraphVisualGroupControls: () => (
      <section data-testid="factory-visual-group-controls">controls</section>
    ),
  }),
);

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      onInit,
    }: {
      children: ReactNode;
      onInit?: (instance: {
        fitView: () => Promise<boolean>;
        getViewport: () => { x: number; y: number; zoom: number };
      }) => void;
    }) => {
      useEffect(() => {
        onInit?.({
          fitView: vi.fn().mockResolvedValue(true),
          getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
        });
      }, [onInit]);

      return <div data-testid="mock-react-flow">{children}</div>;
    },
  };
});

const importController: CurrentActivityImportController = {
  activateImport: vi.fn().mockResolvedValue(undefined),
  activationState: { status: "idle" },
  clearActivationError: vi.fn(),
  clearError: vi.fn(),
  closeImportPreview: vi.fn(),
  dropState: { status: "idle" },
  importPreviewState: { status: "idle" },
  onDragEnter: vi.fn(),
  onDragLeave: vi.fn(),
  onDragOver: vi.fn(),
  onDrop: vi.fn(),
};

const sampleGroups = [
  {
    bounds: { height: 120, width: 200, x: 40, y: 60 },
    id: "group-1",
    label: "Review",
    nodeIds: [],
  },
] as const;

const visualGroupControls = {
  canvasNodeOptions: [],
  colorLabel: "Group color",
  colorOptionLabel: (token: string) => `Use ${token} group color`,
  deleteGroupLabel: "Delete group",
  boundsError: null,
  emptyLabelError: "Enter a group label.",
  group: sampleGroups[0],
  isNodeMember: () => false,
  labelFieldLabel: "Group label",
  membershipEmptyLabel: "No canvas nodes are available to assign.",
  membershipLabel: "Group members",
  membershipNodeLabel: (label: string) => `Include ${label} in this group`,
  membershipStaleNodeLabel: (nodeId: string) =>
    `Saved member ${nodeId} is no longer on the canvas.`,
  onDeleteGroup: vi.fn(),
  onRenameGroup: vi.fn(),
  onSetGroupColor: vi.fn(),
  onToggleNodeMembership: vi.fn(),
  selectedGroupLabel: "Selected visual group",
  staleMemberNodeIds: [],
};

const DEFAULT_GRAPH_RECT = {
  bottom: 720,
  height: 720,
  left: 0,
  right: 1280,
  top: 0,
  width: 1280,
  x: 0,
  y: 0,
  toJSON: () => ({}),
} as DOMRect;

describe("CurrentActivityGraphViewport visual groups", () => {
  const originalBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    HTMLElement.prototype.getBoundingClientRect = () => DEFAULT_GRAPH_RECT;
    globalThis.ResizeObserver = class {
      public constructor(private readonly callback: ResizeObserverCallback) {}

      public disconnect(): void {}

      public observe(target: Element): void {
        this.callback(
          [
            {
              contentRect: DEFAULT_GRAPH_RECT,
              target,
            } as ResizeObserverEntry,
          ],
          this as unknown as ResizeObserver,
        );
      }

      public unobserve(): void {}
    } as unknown as typeof ResizeObserver;
  });

  afterEach(() => {
    HTMLElement.prototype.getBoundingClientRect = originalBoundingClientRect;
    globalThis.ResizeObserver = originalResizeObserver;
  });

  it("renders visual group overlays while editing and hides them in observer mode", async () => {
    const { rerender } = renderVisualGroupViewport({
      editorMode: true,
      onCreateVisualGroup: vi.fn(),
      onSelectVisualGroup: vi.fn(),
      selectedVisualGroupId: "group-1",
      visualGroupControls,
      visualGroups: sampleGroups,
    });

    expect(
      await screen.findByTestId("factory-visual-group-layer"),
    ).toHaveTextContent("group-1");
    expect(
      screen.getByTestId("factory-visual-group-controls"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create group" })).toBeEnabled();

    rerender(
      <CurrentActivityGraphViewport
        {...buildVisualGroupViewportProps({
          editorMode: false,
          onCreateVisualGroup: vi.fn(),
          onSelectVisualGroup: vi.fn(),
          selectedVisualGroupId: "group-1",
          visualGroupControls,
          visualGroups: sampleGroups,
        })}
      />,
    );

    expect(
      screen.queryByTestId("factory-visual-group-layer"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("factory-visual-group-controls"),
    ).not.toBeInTheDocument();
  });

  it("invokes create visual group from the editor toolbar", async () => {
    const onCreateVisualGroup = vi.fn();

    renderVisualGroupViewport({
      editorMode: true,
      onCreateVisualGroup,
      onSelectVisualGroup: vi.fn(),
      visualGroups: sampleGroups,
    });

    fireEvent.click(
      await screen.findByRole("button", { name: "Create group" }),
    );

    expect(onCreateVisualGroup).toHaveBeenCalledTimes(1);
  });
});

function buildVisualGroupViewportProps({
  editorMode = false,
  onCreateVisualGroup,
  onSelectVisualGroup,
  selectedVisualGroupId = null,
  visualGroupControls = null,
  visualGroups = [],
}: {
  editorMode?: boolean;
  onCreateVisualGroup?: () => void;
  onSelectVisualGroup?: (groupId: string) => void;
  selectedVisualGroupId?: string | null;
  visualGroupControls?: typeof visualGroupControls;
  visualGroups?: readonly (typeof sampleGroups)[number][];
} = {}) {
  const flowContainerRef = { current: null as HTMLElement | null };

  return {
    addControls: {},
    editorControls: {
      activeTool: null,
      canInteract: true,
      discardPendingChanges: vi.fn(),
      isEditing: editorMode,
      selectTool: vi.fn(),
      toggleMode: vi.fn(),
      unavailableClassifierWorkstationName: undefined,
    },
    edges: [] as Edge[],
    flowContainerRef,
    handleNodesChange: vi.fn(),
    hasPendingChanges: false,
    layoutControls: {
      canMoveLayout: false,
      canRedo: false,
      canUndo: false,
      initialFitViewKey: "full-graph",
      initialFitViewOptions: { padding: 0.18 },
      moveNode: vi.fn(),
      moveNodesByDelta: vi.fn(),
      redo: vi.fn(),
      reset: vi.fn(),
      undo: vi.fn(),
      updateViewport: vi.fn(),
    },
    headingID: "test-heading",
    imports: importController,
    nodeTypes: {},
    nodes: [] as Node[],
    onCreateVisualGroup,
    onSelectVisualGroup,
    saveControls: { canSave: false, requestConfirmation: vi.fn() },
    selectedVisualGroupId,
    visualGroupAriaLabel: (group: { id: string; label?: string }) =>
      group.label ?? group.id,
    visualGroupCanEdit: true,
    visualGroupControls,
    visualGroups,
    visibilityControls: {
      hiddenNodeClasses: new Set(),
      isDirty: false,
      isMenuOpen: false,
      preset: "all" as const,
      resetPreferences: vi.fn(),
      setMenuOpen: vi.fn(),
      setPreset: vi.fn(),
      toggleHiddenNodeClass: vi.fn(),
    },
  };
}

function renderVisualGroupViewport(
  props: Parameters<typeof buildVisualGroupViewportProps>[0] = {},
) {
  return render(
    <CurrentActivityGraphViewport {...buildVisualGroupViewportProps(props)} />,
  );
}
