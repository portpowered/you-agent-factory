import type {
  FactoryGraphEditorTool,
  FactoryGraphEditorVisibilityPreset,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type {
  FactoryGraphDraft,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { FactoryGraphConnectionEndpoint } from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import type { FactoryGraphSelectionBatchRemovalPlan } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete";

/**
 * State that must survive a dashboard card's temporary unmount during undo.
 * Graph nodes/edges and layout remain canonical editor state; the remaining
 * fields retain the visible editor interaction that was in progress.
 */
export interface WorkflowActivityBentoCardState {
  activeTool: FactoryGraphEditorTool;
  addEntityDraft: FactoryGraphAddEntityDraft | null;
  addEntityErrors: FactoryGraphAddEntityFieldErrors;
  addMenuOpen: boolean;
  blockedRemovalReason: string | null;
  connectionNotice: string | null;
  editorMode: boolean;
  hideShowMenuOpen: boolean;
  hiddenNodeClasses: FactoryGraphNodeKind[];
  isConfirmingLeaveEditor: boolean;
  isConfirmingSave: boolean;
  layout: FactoryLayout;
  pendingBatchRemovalPlan: FactoryGraphSelectionBatchRemovalPlan | null;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingRemovalEdgeId: string | null;
  pendingRemovalNodeId: string | null;
  saveAttemptRevision: number;
  topologyDraft: FactoryGraphDraft;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
}
