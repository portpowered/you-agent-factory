import type { FactoryActivationProvenance } from "../api/session-factory";

export const defaultFactoryActivation: FactoryActivationProvenance = {
  activationId: "fixture-activation",
  loadedSourceDigest: `sha256:${"0".repeat(64)}`,
  state: "ACTIVE",
};
