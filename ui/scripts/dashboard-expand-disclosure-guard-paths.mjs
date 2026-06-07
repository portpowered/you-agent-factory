/**
 * Production files that migrated to shared dashboard inline expand controls.
 * The guard keeps disclosure ownership on ExpandablePanelTrigger, the
 * StandardExpandableSection / CurrentSelectionExpandableSection wrappers, or
 * the ExpandablePanelIcon shell exception for workflow-activity legend chrome.
 */
export const dashboardExpandDisclosureGuardPaths = [
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/standard-card-components/components/standard-expandable-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/editable/workstation-editable-configuration-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/worker-selection/components/worker-editable-configuration-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/resource-selection/components/resource-editable-configuration-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/work-state-selection/components/work-state-editable-configuration-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/work-type-selection/components/work-type-ready-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/detail-card/workstation-active-work-list.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/detail-card/workstation-request-history-section.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/fields/provider-session-attempts.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/dispatch-selection/components/dispatch-history/selected-work-dispatch-attempt-sections.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/work-selection/components/inference-attempt/inference-attempt/inference-attempt-summary-header.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/work-selection/components/inference-attempt/inference-attempt/inference-attempt-body-sections.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/current-selection/workstation-selection/components/editable/workstation-prompt-field.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/terminal-work/components/terminal-work-card.tsx",
  },
  {
    owner: "expandable-panel-trigger",
    relativeFilePath:
      "src/features/trace-drilldown/components/trace-grid-card.tsx",
  },
  {
    owner: "expandable-panel-icon-shell",
    relativeFilePath:
      "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx",
  },
];
