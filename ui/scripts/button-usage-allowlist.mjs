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
    rawButtonFingerprints: ["aria-label={resolvedCloseLabel}"],
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
      "aria-controls={controlsID}",
      "aria-label={sessionCloseLabel(session, messages)}",
    ],
    rawButtonReason:
      "Session tabs keep dedicated tab and tab-close button semantics instead of collapsing into the ordinary Button lane.",
    relativeFilePath:
      "src/features/header/components/dashboard-session-tab.tsx",
  },
  {
    rawButtonFingerprints: ['className="pointer-events-auto absolute inset-0"'],
    rawButtonReason:
      "The workflow mutation dialog keeps structural overlay-dismiss and close-icon button semantics in its shared shell.",
    relativeFilePath:
      "src/features/workflow-activity/components/mutation-dialog.tsx",
  },
];
