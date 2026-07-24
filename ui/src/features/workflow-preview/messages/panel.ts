export const workflowPreviewPanelMessages = {
  empty:
    "Enter a workflow name or inline source to preview validation and policy.",
  loading: "Loading factory preview…",
  success: "Factory preview passed.",
  error: "Factory preview failed.",
  sourceResolution: "Source resolution",
  deniedCapabilities: "Denied capabilities",
  resultConstraints: "Result constraints",
  sourceRefLabel: "Source ref",
  sourceHashLabel: "Source hash",
  policyHashLabel: "Policy hash",
  resultConstraintsSummary: (
    artifactScheme: string,
    maxEmbeddedBytes: number,
  ) =>
    `Structured JSON required; artifact scheme ${artifactScheme}; max embedded bytes ${maxEmbeddedBytes}`,
} as const;
