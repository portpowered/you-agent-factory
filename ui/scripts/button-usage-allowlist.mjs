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
    rawButtonCount: 1,
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
    rawButtonCount: 1,
    rawButtonReason:
      "Graph nodes stay behind the dedicated GraphNodeButton semantic wrapper owner.",
    relativeFilePath: "src/components/ui/graph-node-button.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "The inline add-widget card is a structural popover trigger shell rather than an ordinary action button.",
    relativeFilePath: "src/features/bento/components/inline-add-widget-card.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Inline widget picker options are selectable option rows inside a popover rather than ordinary action buttons.",
    relativeFilePath: "src/features/bento/components/inline-widget-picker.tsx",
  },
  {
    rawButtonCount: 3,
    rawButtonReason:
      "Session tabs keep dedicated tab and tab-close button semantics instead of collapsing into the ordinary Button lane.",
    relativeFilePath: "src/features/header/components/dashboard-session-tabs.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "The open-session dialog trigger is a dedicated tab-strip affordance rather than an ordinary action button.",
    relativeFilePath: "src/features/header/components/dashboard-session-tabs-open-dialog.tsx",
  },
  {
    rawButtonCount: 3,
    rawButtonReason:
      "Inference attempt controls are disclosure toggles and provider-session selection shells inside the current-selection detail surface.",
    relativeFilePath: "src/features/current-selection/components/inference-attempt.tsx",
  },
  {
    rawButtonCount: 4,
    rawButtonReason:
      "Provider-session attempt rows use disclosure and selection semantics rather than ordinary action-button styling.",
    relativeFilePath: "src/features/current-selection/components/provider-session-attempts.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Selected-work dispatch attempt sections are disclosure shells and stay outside the ordinary action-button lane.",
    relativeFilePath: "src/features/current-selection/components/selected-work-dispatch-attempt-sections.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Shared selected-work dispatch controls use work-selection chip semantics rather than ordinary actions.",
    relativeFilePath:
      "src/features/current-selection/components/selected-work-dispatch-history-card-shared.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "State-node work rows are selection shells that need full-row button semantics.",
    relativeFilePath: "src/features/current-selection/components/state-node-detail.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Work-item detail action chips select related work items rather than performing ordinary button-lane actions.",
    relativeFilePath: "src/features/current-selection/components/work-item-card.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Consumed-work payload rows use work-selection chip semantics rather than ordinary action buttons.",
    relativeFilePath: "src/features/current-selection/components/work-item-payload-details.tsx",
  },
  {
    rawButtonCount: 4,
    rawButtonReason:
      "Workstation detail controls are disclosure and selection shells inside the current-selection detail surface.",
    relativeFilePath: "src/features/current-selection/components/workstation-detail-card.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Editable workstation configuration uses a disclosure toggle for an expandable semantic section.",
    relativeFilePath:
      "src/features/current-selection/components/workstation-editable-configuration-section.tsx",
  },
  {
    rawButtonCount: 1,
    rawButtonReason:
      "Transcript code blocks use a disclosure toggle for expandable inline code content.",
    relativeFilePath:
      "src/features/provider-session-detail/components/transcript-code-block.tsx",
  },
  {
    rawButtonCount: 2,
    rawButtonReason:
      "The workflow mutation dialog keeps structural overlay-dismiss and close-icon button semantics in its shared shell.",
    relativeFilePath: "src/features/workflow-activity/components/mutation-dialog.tsx",
  },
  {
    rawButtonCount: 2,
    rawButtonReason:
      "The dashboard flow-axis legend uses narrow disclosure-toggle semantics for its collapsible chrome.",
    relativeFilePath:
      "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx",
  },
];
