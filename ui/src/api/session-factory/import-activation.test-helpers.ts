export const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;

export const defaultSessionFactoryActivation = {
  activationId: "test-activation",
  loadedSourceDigest: `sha256:${"0".repeat(64)}`,
  state: "ACTIVE",
} as const;
