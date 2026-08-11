export const approvedButtonUsageAllowlist = [
  {
    buttonVariantsCount: 2,
    buttonVariantsReason:
      "Shared calendar owner may compose buttonVariants for calendar navigation controls.",
    relativeFilePath: "src/components/ui/calendar.tsx",
  },
  {
    rawButtonFingerprints: [
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
  {
    rawButtonFingerprints: ['role="menuitemradio"'],
    rawButtonReason:
      "Header option menu items need a tone-free button shell so selected primary-container emphasis classes win over DashboardActionButton ghost utilities at runtime.",
    relativeFilePath:
      "src/features/header/components/dashboard-header-option-menu.tsx",
  },
];
