// Temporary shared-ui boundary debt inventory.
// Remove each entry in the same change that relocates feature-owned behavior out of ui/src/components/ui.
export const allowlistedSharedUiBoundaryViolations = [
  {
    importSpecifiers: ["../../features/bento/components/agent-bento"],
    reason:
      "Legacy feature composition in DashboardWidgetFrame. Move AgentBentoCard wiring into a feature-owned wrapper and keep ui/src/components/ui limited to shared presentation shells.",
    relativeFilePath: "src/components/ui/widget-frame.tsx",
  },
];
