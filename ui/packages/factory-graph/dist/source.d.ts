import type { FactoryDefinition } from "@you-agent-factory/client";
import type { FactoryActivityProjection, FactoryLoadProjection, FactoryTopologyProjection } from "@you-agent-factory/factory-replay";
/** Runtime information consumed by the semantic Factory graph at one selected tick. */
export interface FactoryGraphRuntimeProjection {
    activity: FactoryActivityProjection;
    load: FactoryLoadProjection;
    topology: FactoryTopologyProjection;
}
/** Host-owned selection behavior for a read-only graph. */
export interface FactoryGraphObserveControls {
    onSelectNode?: (nodeId: string) => void;
    selectedNodeId?: string;
}
/**
 * Lossless input to a Factory graph renderer.
 *
 * Hosts retain ownership of event streaming, timeline selection, persistence,
 * and editing. The graph receives the complete Factory alongside the selected
 * runtime projection, so semantic node renderers never need a reduced
 * topology-only substitute.
 */
export interface FactoryGraphSource {
    factory: Readonly<FactoryDefinition>;
    runtime: FactoryGraphRuntimeProjection;
    selectedTick: number;
}
export declare function isFactoryGraphSource(value: unknown): value is FactoryGraphSource;
/** Validate the stable graph boundary without cloning or reducing its inputs. */
export declare function createFactoryGraphSource(source: FactoryGraphSource): FactoryGraphSource;
