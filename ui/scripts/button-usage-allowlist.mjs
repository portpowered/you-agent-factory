export const approvedButtonUsageAllowlist = [
  {
    buttonVariantsCount: 1,
    buttonVariantsReason:
      "Shared Button primitive owner may compose buttonVariants for the canonical ordinary-action lane.",
    relativeFilePath: "src/components/ui/button.tsx",
  },
  {
    buttonVariantsCount: 2,
    buttonVariantsReason:
      "Shared dialog owner may compose buttonVariants for its structural close and action controls.",
    rawButtonFingerprints: ['aria-label={resolvedCloseLabel}'],
    rawButtonReason:
      "Shared dialog owner keeps one raw semantic close trigger around the action rows.",
    relativeFilePath: "src/components/ui/dialog.tsx",
  },
  {
    buttonVariantsCount: 2,
    buttonVariantsReason:
      "Shared calendar owner may compose buttonVariants for calendar navigation controls.",
    relativeFilePath: "src/components/ui/calendar.tsx",
  },
  {
    rawButtonFingerprints: [
      "className={cn(GRAPH_NODE_BUTTON_BASE_CLASS, className)}",
    ],
    rawButtonReason:
      "Graph nodes stay behind the dedicated GraphNodeButton semantic wrapper owner.",
    relativeFilePath: "src/components/ui/graph-node-button.tsx",
  },
  {
    rawButtonFingerprints: [
      'aria-haspopup="dialog"',
      'aria-controls={controlsID}',
      'aria-label={sessionCloseLabel(session, messages)}',
    ],
    rawButtonReason:
      "Session tabs keep dedicated tab and tab-close button semantics instead of collapsing into the ordinary Button lane.",
    relativeFilePath: "src/features/header/components/dashboard-session-tabs.tsx",
  },
  {
    rawButtonFingerprints: ['SESSION_TARGET_BUTTON_CLASS'],
    rawButtonReason:
      "The open-session dialog trigger is a dedicated tab-strip affordance rather than an ordinary action button.",
    relativeFilePath: "src/features/header/components/dashboard-session-tabs-open-dialog.tsx",
  },
  {
    rawButtonFingerprints: [
      'aria-controls={panelId} aria-expanded={expanded}',
      'className={HISTORY_TOGGLE_CLASS} onClick={() => setExpanded((current) => !current)}',
      'aria-label={workstationMessages.selectProviderSessionLabel(',
    ],
    rawButtonReason:
      "Inference attempt controls are disclosure toggles and provider-session selection shells inside the current-selection detail surface.",
    relativeFilePath: "src/features/current-selection/components/inference-attempt.tsx",
  },
  {
    rawButtonFingerprints: [
      'aria-controls={historyID} aria-expanded={expanded}',
      'aria-label={messages.selectProviderSessionLabel(',
      'aria-label={messages.selectWorkItemLabel(',
      'aria-label={messages.selectWorkstationRequestLabel(',
    ],
    rawButtonReason:
      "Provider-session attempt rows use disclosure and selection semantics rather than ordinary action-button styling.",
    relativeFilePath: "src/features/current-selection/components/provider-session-attempts.tsx",
  },
  {
    rawButtonFingerprints: ['aria-controls={panelId} aria-expanded={expanded}'],
    rawButtonReason:
      "Selected-work dispatch attempt sections are disclosure shells and stay outside the ordinary action-button lane.",
    relativeFilePath: "src/features/current-selection/components/selected-work-dispatch-attempt-sections.tsx",
  },
  {
    rawButtonFingerprints: ['aria-label={selectWorkItemAccessibleLabel('],
    rawButtonReason:
      "Shared selected-work dispatch controls use work-selection chip semantics rather than ordinary actions.",
    relativeFilePath:
      "src/features/current-selection/components/selected-work-dispatch-history-card-shared.tsx",
  },
  {
    rawButtonFingerprints: ['aria-label={messages.selectWorkItemLabel(workLabel)}'],
    rawButtonReason:
      "State-node work rows are selection shells that need full-row button semantics.",
    relativeFilePath: "src/features/current-selection/components/state-node-detail.tsx",
  },
  {
    rawButtonFingerprints: ['aria-label={messages.relatedWorkSelectLabel(label)}'],
    rawButtonReason:
      "Work-item relationship nodes use selection-chip semantics to jump between related work items rather than performing ordinary button-lane actions.",
    relativeFilePath:
      "src/features/current-selection/components/work-item-relationship-graph.tsx",
  },
  {
    rawButtonFingerprints: ['aria-label={resolvedMessages.selectWorkItemLabel(workLabel)}'],
    rawButtonReason:
      "Consumed-work payload rows use work-selection chip semantics rather than ordinary action buttons.",
    relativeFilePath: "src/features/current-selection/components/work-item-payload-details.tsx",
  },
  {
    rawButtonFingerprints: [
      'aria-controls={historyID} aria-expanded={expanded} className={HISTORY_TOGGLE_CLASS}',
    ],
    rawButtonReason:
      "Workstation detail keeps one disclosure-toggle shell inside the current-selection detail surface while ordinary actions stay on shared button primitives.",
    relativeFilePath: "src/features/current-selection/components/workstation-detail-card.tsx",
  },
  {
    rawButtonFingerprints: ['messages.editableConfigurationCollapseActionLabel'],
    rawButtonReason:
      "Editable workstation configuration uses a disclosure toggle for an expandable semantic section.",
    relativeFilePath:
      "src/features/current-selection/components/workstation-editable-configuration-section.tsx",
  },
  {
    rawButtonFingerprints: ['aria-controls={panelID} aria-expanded={expanded}'],
    rawButtonReason:
      "Transcript code blocks use a disclosure toggle for expandable inline code content.",
    relativeFilePath:
      "src/features/provider-session-detail/components/transcript-code-block.tsx",
  },
  {
    rawButtonFingerprints: [
      'className="pointer-events-auto absolute inset-0"',
      'className={DIALOG_CLOSE_BUTTON_CLASS}',
    ],
    rawButtonReason:
      "The workflow mutation dialog keeps structural overlay-dismiss and close-icon button semantics in its shared shell.",
    relativeFilePath: "src/features/workflow-activity/components/mutation-dialog.tsx",
  },
  {
    rawButtonFingerprints: [
      'aria-expanded="true"',
      'aria-expanded="false"',
    ],
    rawButtonReason:
      "The dashboard flow-axis legend uses narrow disclosure-toggle semantics for its collapsible chrome.",
    relativeFilePath:
      "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx",
  },
];
