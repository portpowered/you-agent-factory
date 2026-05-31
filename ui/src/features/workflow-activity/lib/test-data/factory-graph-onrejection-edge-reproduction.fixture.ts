import type { CanonicalFactoryDefinition } from "../../../../api/factory-definition";
import factoryGraphOnrejectionEdgeReproductionFactory from "./factory-graph-onrejection-edge-reproduction.factory.json";

/** Committed copy of repo `factory/factory.json` for graph edit-mode regression tests. */
export function loadFactoryGraphOnrejectionEdgeReproductionFactory(): CanonicalFactoryDefinition {
  return factoryGraphOnrejectionEdgeReproductionFactory as CanonicalFactoryDefinition;
}
