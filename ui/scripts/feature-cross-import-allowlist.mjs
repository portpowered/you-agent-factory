// Temporary cross-feature internal-import debt inventory.
// Remove each entry in the same change that routes reuse through public/ boundaries.
export const allowlistedCrossFeatureBoundaryViolations = [
  {
    relativeFilePath: "src/features/bento/components/dashboard-bento-cards.tsx",
    importSpecifiers: [
      "../../current-selection/hooks/core/useCurrentSelection",
      "../../current-selection/hooks/core/useCurrentSelectionDetails",
      "../../current-selection/work-selection/hooks/useSelectedProviderSessionState",
      "../../dashboard-add-card/components/inline-add-widget-card",
      "../../provider-session-detail/messages/provider-session-widget",
      "../../submit-work/messages/submit-work",
      "../../terminal-work/messages/terminal-work",
      "../../trace-drilldown/hooks/useTraceDrilldown",
      "../../trace-drilldown/messages/trace-drilldown",
      "../../work-outcome/hooks/useWorkOutcomeChart",
      "../../work-outcome/messages/work-outcome",
      "../../work-totals/messages/work-totals",
      "../../workflow-activity/hooks/current-activity-import-controller",
      "../../workflow-activity/messages/activity-shell",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/bento/components/dashboard-bento-story-shared.tsx",
    importSpecifiers: [
      "../../current-selection/hooks/core/useCurrentSelection",
      "../../current-selection/hooks/core/useCurrentSelectionDetails",
      "../../current-selection/work-selection/hooks/useSelectedProviderSessionState",
      "../../dashboard-add-card/components/inline-add-widget-card",
      "../../submit-work/components/submit-work-card",
      "../../trace-drilldown/hooks/useTraceDrilldown",
      "../../work-outcome/lib/trends",
      "../../workflow-activity/hooks/current-activity-import-controller",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/bento/components/dashboard-bento.tsx",
    importSpecifiers: [
      "../../current-selection/hooks/core/useCurrentSelectionDetails",
      "../../current-selection/work-selection/hooks/useSelectedProviderSessionState",
      "../../dashboard/session/dashboard-session-provider",
      "../../import/lib/factory-import-save-choice",
      "../../trace-drilldown/hooks/useTraceDrilldown",
      "../../work-outcome/hooks/useWorkOutcomeChart",
      "../../workflow-activity/hooks/current-activity-import-controller",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/bento/hooks/use-dashboard-bento-snapshot.ts",
    importSpecifiers: [
      "../../current-selection/hooks/core/useCurrentSelection",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-factory-definition/hooks/useFactoryDocumentSave.ts",
    importSpecifiers: ["../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-factory-definition/lib/resource-editable-values.ts",
    importSpecifiers: [
      "../../current-selection/resource-selection/lib/resource-detail-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-factory-definition/lib/runner-selection.ts",
    importSpecifiers: [
      "../../current-selection/workstation-selection/messages/runner-openapi-enums",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-factory-definition/lib/workstation-editable-values.ts",
    importSpecifiers: [
      "../../current-selection/workstation-selection/messages/runner-openapi-enums",
    ],
    reason:
      "Canonical OpenAPI runner identifier type lives in workstation-selection runner-openapi-enums; cross-feature type import is allowlisted instead of re-exporting through public/.",
  },
  {
    relativeFilePath: "src/features/current-factory-definition/public/index.ts",
    importSpecifiers: [
      "../../current-selection/base/hooks/factory-document-save-types",
      "../../current-selection/base/hooks/useScopedFactoryDocumentSave",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/components/detail-card/detail-card-save-test-helpers.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/hooks/useFactoryDocumentSave",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/components/detail-card/detail-card-types.ts",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/hooks/useScopedFactoryDocumentSave.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/operations/factory-graph-topology-impact",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/lib/build-current-selection-save-toast-messages.ts",
    importSpecifiers: [
      "../../../notifications/lib/save-notification-delivery-policy",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/lib/current-selection-save-notifications.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/document-save/graph-document-save-notifications",
      "../../../notifications/lib/save-notification-delivery-policy",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/messages/shell/current-selection-save-toast.ts",
    importSpecifiers: [
      "../../../../notifications/lib/save-notification-delivery-policy",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/state/factoryGraphNodeSelection.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/state/selection-types.ts",
    importSpecifiers: ["../../../terminal-work/lib/types"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/components/widget/current-selection-widget-save-notifications.tsx",
    importSpecifiers: [
      "../../../workflow-activity/state/factory-graph-topology-editor-bridge",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/components/widget/current-selection-widget.tsx",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
      "../../../provider-session-detail/lib/provider-session-ref",
      "../../../workflow-activity/state/factory-graph-editor-selection-bridge",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/graph-selection/components/graph-bulk-selection-detail-card.tsx",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/graph-selection/messages/graph-bulk-selection-detail.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/graph-selection/lib/resolve-active-graph-bulk-selection-summary.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
      "../../../workflow-activity/state/factory-graph-editor-selection-bridge",
    ],
    reason:
      "Graph bulk summary panel scoping reads editor-local bridge state and dashboard selection together.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/dispatch-selection/components/dispatch-history/selected-work-dispatch-attempt-sections.tsx",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/dispatch-selection/components/dispatch-history/selected-work-dispatch-history-card.tsx",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/dispatch-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/hooks/core/useCurrentSelection.actions.ts",
    importSpecifiers: ["../../../terminal-work/lib/types"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/lib/detail-card-types.ts",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Legacy cross-feature bundled-doc reuse while workflow-activity/public boundaries are still being established.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/hooks/core/useCurrentSelection.derived.ts",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Legacy cross-feature bundled-doc reuse while workflow-activity/public boundaries are still being established.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/hooks/core/useCurrentSelection.ts",
    importSpecifiers: [
      "../../../workflow-activity/lib/factory-bundled-docs",
      "../../../terminal-work/lib/types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-view.tsx",
    importSpecifiers: [
      "../../import/hooks/use-factory-png-drop",
      "../../import/lib/factory-import-save-choice",
      "../../import/lib/factory-png-import",
    ],
    reason:
      "Current workflow-activity import flow still depends on import feature internals pending a public boundary.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/current-activity-graph-editor-status-state.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state",
    ],
    reason:
      "Workflow-activity graph editor state currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/current-activity-graph-state-value.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook",
      "../../factory-graph-editor/hooks/validation/use-factory-validation",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-additions",
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state",
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary",
      "../../factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Workflow-activity graph editor state currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-add-controller.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/doc-editable-values",
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-additions",
    ],
    reason:
      "Workflow-activity graph editor state currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-editable-graph.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
    ],
    reason:
      "Workflow-activity graph editor state currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-view-model.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../factory-graph-editor/lib/projection/factory-graph-react-flow-edge-waypoint-projection",
      "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection",
    ],
    reason:
      "Workflow-activity graph view-model currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/current-activity-graph-selection-bridge-publisher.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-selection",
    ],
    reason:
      "Graph selection bridge publishing reads editor-local selection helpers pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-selection-gestures.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection",
    ],
    reason:
      "Workflow-activity graph selection gestures reuse editor-local selection helpers pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/state/factory-graph-editor-selection-bridge.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-selection",
    ],
    reason:
      "Graph selection bridge state stores editor-local selection snapshots pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-flow-projection.ts",
    importSpecifiers: ["../../flowchart/lib/layout"],
    reason:
      "Workflow-activity graph projection currently reuses flowchart layout internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-card-view-model.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/layout/factory-graph-visual-group-editor-hook",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-toolbar-selection",
    ],
    reason:
      "Workflow-activity graph view-model currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-render-state.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/validation/use-factory-validation",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Workflow-activity graph render state currently reuses factory-graph-editor and flowchart internals pending public boundaries.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-state.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/layout/factory-graph-edge-waypoint-editor-hook",
    ],
    reason:
      "Workflow-activity graph state currently reuses factory-graph-editor internals pending a public boundary split.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/hooks/helpers/useCurrentSelection.selection-helpers.ts",
    importSpecifiers: ["../../../terminal-work/lib/types"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/resource-selection/components/resource-editable-configuration-section.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/resource-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/resource-selection/editing/editable-resource-overwrite-fields.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/resource-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/resource-selection/hooks/use-editable-resource-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/resource-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/resource-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/resource-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/resource-selection/lib/resource-editable-validation.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/resource-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-selection/components/inference-attempt/inference-attempt-provider-session.tsx",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-selection/components/inference-attempt/inference-attempt-summary-header.tsx",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-selection/hooks/useSelectedProviderSessionState.ts",
    importSpecifiers: [
      "../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-state-selection/hooks/use-editable-work-state-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-state-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-state-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-state-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-state-selection/lib/work-state-editable-validation.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-state-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-state-selection/messages/work-state-detail-enums.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-state-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-state-selection/messages/work-state-detail-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-state-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/components/work-type-ready-section.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-editable-validation",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/components/work-type-save-controls.tsx",
    importSpecifiers: ["../../../workflow-activity/components/mutation-dialog"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/components/work-type-states-list.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/hooks/use-editable-work-type-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-editable-validation",
      "../../../current-factory-definition/lib/work-type-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-editable-validation",
      "../../../current-factory-definition/lib/work-type-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/work-type-selection/messages/work-type-detail-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-editable-validation",
      "../../../current-factory-definition/lib/work-type-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/components/editable/fields/worker-editable-configuration-shared-fields.tsx",
    importSpecifiers: [
      "../../../../../current-factory-definition/lib/worker-timeout-duration",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/components/editable/fields/worker-editable-configuration-model-fields.tsx",
    importSpecifiers: [
      "../../../../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/components/editable/fields/worker-editable-configuration-hosted-fields.tsx",
    importSpecifiers: [
      "../../../../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/editing/editable-worker-overwrite-fields.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/hooks/use-editable-worker-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/worker-selection/lib/worker-editable-validation.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/worker-editable-values",
      "../../../current-factory-definition/lib/worker-timeout-duration",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/workstation-model-invoke-fields.tsx",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/factory-graph-add-model-operation-draft",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/editable/workstation-editable-configuration-section.tsx",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-guards",
      "../../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../../current-factory-definition/lib/workstation/workstation-model-invoke",
      "../../../../current-factory-definition/lib/workstation/workstation-type",
      "../../../../flowchart/components/graph-semantic-icon",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/provider-session-attempts.tsx",
    importSpecifiers: [
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/workstation-guards-field.tsx",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-guards",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/workstation-input-guards-field.tsx",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation-guards",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/workstation-runner-field.tsx",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/runner-selection",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/workstation-summary-field-values.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../../flowchart/lib/workstation-graph-presentation",
      "../../../../flowchart/lib/workstation-icon-metadata",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/editing/editable-workstation-configuration-mutators.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/editing/editable-workstation-cron-draft-mutators.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/editing/editable-workstation-overwrite-fields.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation-guards",
      "../../../current-factory-definition/lib/workstation/workstation-model-invoke",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/editing/model-invoke/editable-workstation-model-invoke-mutators.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation/workstation-model-invoke",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/editing/type/editable-workstation-type-mutators.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation/workstation-model-invoke",
      "../../../../current-factory-definition/lib/workstation/workstation-type",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/hooks/editable-workstation-ready-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation-guards",
      "../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../current-factory-definition/lib/workstation/workstation-model-invoke",
      "../../../current-factory-definition/lib/workstation/workstation-type",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/hooks/use-editable-workstation-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation-guards",
      "../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../current-factory-definition/lib/workstation/workstation-model-invoke",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/hooks/useCurrentWorkstationPromptTemplateContract.ts",
    importSpecifiers: ["../../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/hooks/useCurrentWorkstationPromptTemplateValidation.ts",
    importSpecifiers: ["../../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/lib/editable-workstation-model-invoke-options.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation/workstation-model-invoke",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/lib/keys/detail-card-types.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-behavior",
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation/workstation-model-invoke",
      "../../../../provider-session-detail/lib/provider-session-ref",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/lib/keys/workstation-guard-row-keys.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-guards",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/lib/validation/editable-workstation-configuration-validation.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/editable-workstation-cron-validation",
      "../../../../current-factory-definition/lib/workstation-behavior",
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../../current-factory-definition/lib/workstation/workstation-model-invoke",
      "../../../../factory-graph-editor/lib/factory-graph-add-model-operation-draft",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/lib/validation/workstation-editable-validation.ts",
    importSpecifiers: [
      "../../../../current-factory-definition/lib/workstation-editable-values",
      "../../../../current-factory-definition/lib/workstation-guards",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/workstation-selection/messages/workstation-detail-enums.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-guards",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard-add-card/components/inline-add-widget-card.tsx",
    importSpecifiers: [
      "../../bento/lib/dashboard-widget-picker",
      "../../bento/messages/inline-add-widget",
      "../../bento/messages/inline-widget-picker",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard-add-card/components/inline-add-widget-selector.tsx",
    importSpecifiers: ["../../bento/lib/dashboard-widget-picker"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/dashboard/components/dashboard-screen.tsx",
    importSpecifiers: [
      "../../bento/state/dashboardBentoStore",
      "../../header/messages/header-controls",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard/hooks/event-stream/useFactoryEventStream.fixtures.ts",
    importSpecifiers: ["../../../timeline/state/factoryTimelineStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard/hooks/event-stream/useFactoryEventStream.ts",
    importSpecifiers: ["../../../timeline/state/factoryTimelineDebug"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard/hooks/useDashboardSessionLifecycle.ts",
    importSpecifiers: ["../../timeline/state/factoryTimelineStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard/hooks/useDashboardTimelineMemoryDebug.ts",
    importSpecifiers: [
      "../../timeline/state/factoryTimelineDebug",
      "../../timeline/state/factoryTimelineStore",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/dashboard/hooks/useDashboardWorldView.ts",
    importSpecifiers: ["../../timeline/state/factoryTimelineStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/dashboard/lib/dashboard-event-stream.ts",
    importSpecifiers: [
      "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/dashboard/lib/dashboard-session-lifecycle.ts",
    importSpecifiers: [
      "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/export/hooks/use-current-factory-export.ts",
    importSpecifiers: [
      "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
      "../../dashboard/session/dashboard-session-provider",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/components/add-dialog/factory-graph-editor-add-dialog.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/worker-editable-values",
      "../../../current-selection/worker-selection/messages/worker-detail",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/components/add-dialog/factory-graph-editor-add-workstation-fields.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation/workstation-type",
      "../../../current-factory-definition/lib/workstation-worker-assignment",
      "../../../current-selection/workstation-selection/messages/workstation-detail",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/components/flow/factory-graph-editor-flow.tsx",
    importSpecifiers: [
      "../../../flowchart/components/current-activity-node-chrome",
      "../../../flowchart/components/current-activity-node-shell",
      "../../../flowchart/components/graph-semantic-icon",
      "../../../flowchart/lib/current-activity-graph-hover",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/hooks/use-editable-factory-graph-save-controller.ts",
    importSpecifiers: [
      "../../current-selection/base/hooks/factory-document-save-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/draft/factory-graph-draft-validation.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-worker-assignment",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/editor/factory-graph-editor-additions.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/editable-workstation-cron-validation",
      "../../../current-factory-definition/lib/workstation-behavior",
      "../../../current-factory-definition/lib/workstation-editable-values",
      "../../../current-factory-definition/lib/workstation/workstation-type",
      "../../../current-factory-definition/lib/workstation-worker-assignment",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/factory-graph-editor-additions.worker.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/worker-editable-values",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/editor/factory-graph-editor-connections.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../../current-factory-definition/lib/workstation-worker-assignment",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/editor/factory-graph-editor-layout.ts",
    importSpecifiers: ["../../../flowchart/lib/layered-layout"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/operations/factory-graph-customer-display.ts",
    importSpecifiers: ["../../../timeline/state/timeline/systemTime"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/projection/factory-graph-react-flow-projection.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/work-type-default-handling",
      "../../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../../flowchart/components/current-activity-node-shell",
      "../../../flowchart/lib/current-activity-graph-hover",
      "../../../flowchart/lib/workstation-graph-presentation",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/projection/factory-graph-workstation-connection-context.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-progress-outcome-routes",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/projection/factory-validation-graph-projection.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/workstation-progress-outcome-routes",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling.ts",
    importSpecifiers: [
      "../../../flowchart/components/current-activity-node-chrome",
      "../../../flowchart/components/graph-semantic-icon",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/components/current-activity-place-node.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling",
      "../../workflow-activity/messages/activity-shell",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/components/current-activity-resource-node.tsx",
    importSpecifiers: ["../../workflow-activity/messages/activity-shell"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/components/current-activity-work-type-node.tsx",
    importSpecifiers: ["../../workflow-activity/messages/activity-shell"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/components/current-activity-worker-node.tsx",
    importSpecifiers: ["../../workflow-activity/messages/activity-shell"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/lib/workstation-graph-presentation.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/workstation-behavior",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/graphs/components/work-relation-node.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../flowchart/components/current-activity-node-chrome",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/graphs/components/workstation-node-view.tsx",
    importSpecifiers: [
      "../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../flowchart/components/current-activity-node-chrome",
      "../../flowchart/components/current-activity-work-progress-marker",
      "../../flowchart/components/graph-semantic-icon",
      "../../flowchart/lib/current-activity-graph-hover",
      "../../flowchart/lib/workstation-graph-presentation",
      "../../flowchart/messages/activity-graph",
      "../../workflow-activity/messages/activity-shell",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/header/components/dashboard-export-dialog.tsx",
    importSpecifiers: [
      "../../export/hooks/use-current-factory-export",
      "../../export/state/exportDialogStore",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/header/components/dashboard-header-session-controls.tsx",
    importSpecifiers: ["../../export/messages/export-dialog"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/header/components/dashboard-header.tsx",
    importSpecifiers: ["../../export/state/exportDialogStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/header/components/dashboard-session-tabs.tsx",
    importSpecifiers: ["../../dashboard/state/dashboardStreamStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/header/components/tick-slider-control.tsx",
    importSpecifiers: ["../../timeline/state/factoryTimelineStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/header/hooks/use-dashboard-session-tabs-state.ts",
    importSpecifiers: ["../../dashboard/state/dashboardSessionStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/import/hooks/use-factory-import-activation.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/sync-current-factory-document-cache",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/submit-work/components/submit-work-widget.tsx",
    importSpecifiers: ["../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/terminal-work/components/terminal-work-card.tsx",
    importSpecifiers: [
      "../../current-selection/base/components/presentation/current-selection-pill",
      "../../current-selection/base/components/presentation/current-selection-supporting-text",
      "../../current-selection/workstation-selection/components/detail-card/workstation-dispatch-row",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/hooks/use-trace-dispatch-factory-graph-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../workflow-activity/hooks/workflow-topology-async-cache",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/hooks/use-trace-relation-factory-graph-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../workflow-activity/hooks/workflow-topology-async-cache",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath: "src/features/trace-drilldown/hooks/useTrace.ts",
    importSpecifiers: ["../../timeline/state/factoryTimelineStore"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/lib/trace-dispatch-factory-graph-flow.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection",
      "../../flowchart/lib/workstation-icon-metadata",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/lib/trace-dispatch-factory-graph.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/lib/trace-factory-graph-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/lib/trace-relation-factory-graph-flow.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/trace-drilldown/lib/trace-relation-factory-graph.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling",
      "../../factory-graph-editor/lib/work-state/factory-graph-work-state-type",
      "../../factory-graph-editor/messages/editor",
      "../../flowchart/lib/workstation-icon-metadata",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-editor-chrome.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-editor-dialogs.tsx",
    importSpecifiers: [
      "../../current-selection/base/hooks/factory-document-save-types",
      "../../factory-graph-editor/components/add-dialog/factory-graph-editor-add-dialog",
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/components/dialogs/factory-graph-editor-leave-dialog",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-import.tsx",
    importSpecifiers: [
      "../../import/hooks/use-factory-png-drop",
      "../../import/lib/factory-png-import",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-save-notifications.tsx",
    importSpecifiers: [
      "../../factory-graph-editor/lib/document-save/graph-document-save-notifications",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-surface.tsx",
    importSpecifiers: ["../../factory-graph-editor/messages/editor"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card-viewport.tsx",
    importSpecifiers: [
      "../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-connections",
      "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-controls",
      "../../factory-graph-editor/components/flow/factory-graph-edge-waypoint-layer",
      "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-controls",
      "../../factory-graph-editor/components/flow/visual-groups/factory-graph-visual-group-layer",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups",
      "../../factory-graph-editor/lib/layout/history/factory-graph-layout-keyboard-shortcuts",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-react-flow-interaction",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/react-flow-current-activity-card.tsx",
    importSpecifiers: ["../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/components/workflow-activity-bento-card.tsx",
    importSpecifiers: ["../../dashboard/session/dashboard-session-provider"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/current-activity-import-controller.ts",
    importSpecifiers: [
      "../../import/hooks/use-factory-import-activation",
      "../../import/hooks/use-factory-import-preview",
      "../../import/hooks/use-factory-png-drop",
      "../../import/lib/factory-import-save-choice",
      "../../import/lib/factory-png-import",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/factory-graph-editor-availability.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-connections.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-connections",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-draft-validation.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/hooks/validation/use-factory-validation",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-apply",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-validation",
      "../../factory-graph-editor/lib/projection/factory-validation-graph-projection",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-editable-graph.ts",
    importSpecifiers: [
      "../../factory-graph-editor/hooks/use-editable-factory-graph",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-removals.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-removals",
      "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/operations/factory-graph-topology-impact",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-current-activity-graph-flow-projection.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../factory-graph-editor/lib/layout/factory-graph-layout-validation",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-view-model.ts",
    importSpecifiers: ["../../flowchart/lib/layout"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-graph-editor-controllers.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-graph-editor-save-flow.ts",
    importSpecifiers: [
      "../../current-selection/base/hooks/factory-document-save-types",
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/hooks/use-editable-factory-graph-types",
      "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-save-summary",
      "../../factory-graph-editor/messages/editor",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-graph-editor-session.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-additions",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-hidden-factory-graph-node-classes.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/preferences/factory-graph-editor-preferences",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/use-topology-stable-factory-for-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/operations/factory-graph-topology-impact",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/current-activity-factory-graph-layout.ts",
    importSpecifiers: [
      "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-graph",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-layout",
      "../../factory-graph-editor/lib/operations/factory-graph-customer-display",
      "../../factory-graph-editor/lib/preferences/factory-graph-visibility-preset-projection",
      "../../factory-graph-editor/lib/work-state/factory-graph-node-class-visibility",
      "../../factory-graph-editor/messages/editor",
      "../../flowchart/lib/layered-layout",
      "../../flowchart/lib/layout",
      "../../flowchart/lib/workstation-icon-metadata",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/layout/factory-graph-canonical-layout-positions.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
      "../../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/layout/use-canonical-layout-viewport-sync.ts",
    importSpecifiers: [
      "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/current-activity-factory-graph-node-ids.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/graph-editor-add-node-placement.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-additions",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/graph-editor-node-placement.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-draft-edges.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-edges.ts",
    importSpecifiers: [
      "../../flowchart/lib/current-activity-graph-hover",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-editor-handles.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-connections",
      "../../factory-graph-editor/lib/projection/factory-graph-progress-outcome-handle-visibility",
      "../../factory-graph-editor/lib/projection/factory-validation-graph-projection",
      "../../factory-graph-editor/messages/editor",
      "../../flowchart/components/current-activity-node-shell",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-graph.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/work-type-default-handling",
      "../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../factory-graph-editor/lib/editor/factory-graph-editor-connections",
      "../../factory-graph-editor/lib/projection/factory-validation-graph-projection",
      "../../flowchart/lib/layout",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-keys.ts",
    importSpecifiers: ["../../flowchart/lib/layout"],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/react-flow-current-activity-card-validation.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/workstation-progress-outcome-routes",
      "../../factory-graph-editor/lib/projection/factory-validation-graph-projection",
    ],
    reason:
      "Legacy cross-feature internal import. Route reuse through the target feature public/ boundary or relocate shared behavior.",
  },
  {
    relativeFilePath:
      "src/features/current-factory-definition/lib/doc-editable-values.ts",
    importSpecifiers: ["../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/base/state/dashboardSelection.ts",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/components/widget/current-selection-widget-save-notifications.tsx",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/components/doc-detail-card.tsx",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/components/doc-editable-configuration-section.tsx",
    importSpecifiers: [
      "../../../current-factory-definition/lib/doc-editable-values",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/editing/editable-doc-overwrite-fields.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/doc-editable-values",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/hooks/use-doc-detail-state.ts",
    importSpecifiers: [
      "../../../workflow-activity/lib/factory-bundled-docs",
      "../../../workflow-activity/state/graph-editor-pending-factory-bridge",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/hooks/use-editable-doc-configuration-state.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/doc-editable-values",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/lib/detail-card-types.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/doc-editable-values",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/doc-selection/lib/doc-editable-validation.ts",
    importSpecifiers: [
      "../../../current-factory-definition/lib/doc-editable-values",
      "../../../workflow-activity/lib/factory-bundled-docs",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/current-selection/hooks/core/useCurrentSelection.ts",
    importSpecifiers: [
      "../../../workflow-activity/state/graph-editor-pending-factory-bridge",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/draft/factory-graph-draft-apply.ts",
    importSpecifiers: ["../../../workflow-activity/lib/factory-bundled-docs"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/factory-graph-doc-editor.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/doc-editable-values",
      "../../workflow-activity/lib/factory-bundled-docs",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/factory-graph-editor/lib/factory-graph-editor-doc-additions.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/doc-editable-values",
      "../../workflow-activity/lib/factory-bundled-docs",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/flowchart/components/current-activity-doc-node.tsx",
    importSpecifiers: ["../../workflow-activity/messages/activity-shell"],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/hooks/react-flow-current-activity-card-editor-removals.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/factory-graph-doc-editor",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/lib/graph-editor-add-node-placement.ts",
    importSpecifiers: [
      "../../current-factory-definition/lib/doc-editable-values",
      "../../factory-graph-editor/lib/factory-graph-doc-editor",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
  {
    relativeFilePath:
      "src/features/workflow-activity/state/graph-editor-pending-factory-bridge.ts",
    importSpecifiers: [
      "../../factory-graph-editor/lib/draft/factory-graph-draft-types",
    ],
    reason:
      "Doc-nodes cross-feature internal import debt from factory/docs dashboard graph and current-selection flows. Route reuse through public/ boundaries in a follow-up structural cleanup.",
  },
];
