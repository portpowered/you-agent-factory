import type { FactoryGraphNodeFamily } from "./node-family.js";
export type FactoryGraphVisualLifecycleRole = "unknown" | "initial" | "processing" | "terminal" | "failed";
export type FactoryGraphVisualStatusRole = "quiet" | "waiting" | "active" | "success" | "danger";
/** Semantic role used by nested accents owned by a graph node. */
export type FactoryGraphVisualNestedAccentRole = "neutral" | "info" | "warning" | "success" | "danger";
/**
 * Derive the semantic role for a nested accent from its parent's resolved
 * visual status. Callers can use the result for surfaces, borders, readable
 * foregrounds, glows, rings, badges, and icons without copying this policy.
 */
export declare function factoryGraphVisualNestedAccentRole(parentStatus: FactoryGraphVisualStatusRole): FactoryGraphVisualNestedAccentRole;
export type FactoryGraphVisualBorderRole = FactoryGraphVisualStatusRole | "selection" | "validation";
/**
 * Whether a node paints its tone as a translucent container or as a solid
 * block. Only a node that currently holds Work earns the solid treatment.
 */
export type FactoryGraphVisualFillRole = "soft" | "solid";
export type FactoryGraphVisualGlowRole = "none" | "active" | "danger" | "selection" | "validation";
export type FactoryGraphVisualStatusTreatment = "none" | "waiting" | "processing" | "completed" | "failed";
export type FactoryGraphVisualEmphasis = "quiet" | "standard" | "strong" | "attention" | "selected";
export type FactoryGraphVisualFocusRole = "none" | "keyboard" | "selection" | "selection-and-keyboard";
export type FactoryGraphValidationState = "none" | "warning" | "error";
export interface FactoryGraphVisualStateInput {
    /**
     * Whether the node currently holds active Work. A work state reports its
     * held Work; a workstation reports its running executions. This is separate
     * from `lifecycle`, which is an authored category and says nothing about
     * whether anything is in the node right now.
     */
    activeWork?: boolean;
    /** Protocol-neutral Factory graph family, not a transport node type. */
    family: FactoryGraphNodeFamily;
    /** Work-state lifecycle or an equivalent host runtime status. */
    lifecycle?: string | null;
    runtimeStatus?: string | null;
    selected?: boolean;
    focused?: boolean;
    validation?: FactoryGraphValidationState | boolean | null;
    activeFlow?: boolean;
    muted?: boolean;
}
export interface FactoryGraphVisualState {
    family: FactoryGraphNodeFamily;
    lifecycle: FactoryGraphVisualLifecycleRole;
    status: FactoryGraphVisualStatusRole;
    surface: FactoryGraphVisualStatusRole;
    fill: FactoryGraphVisualFillRole;
    border: FactoryGraphVisualBorderRole;
    glow: FactoryGraphVisualGlowRole;
    icon: FactoryGraphVisualStatusRole;
    statusTreatment: FactoryGraphVisualStatusTreatment;
    emphasis: FactoryGraphVisualEmphasis;
    focus: FactoryGraphVisualFocusRole;
    selection: boolean;
    validation: FactoryGraphValidationState;
    activeFlow: boolean;
    muted: boolean;
}
/**
 * Resolve one composable visual grammar for a semantic Factory node.
 *
 * Precedence is explicit: lifecycle status is always retained in `status`;
 * active flow may elevate a quiet node's `surface`, `icon`, and
 * `statusTreatment`; validation overlays selection and active-flow emphasis;
 * selection/focus overlays active-flow emphasis; and muted is returned as an
 * independent flag so it cannot erase any status.
 *
 * Tone and occupancy are separate axes. `surface` carries the authored tone
 * even when the node is empty; only held Work promotes `fill` to `solid` and
 * lights the active glow, so an authored `PROCESSING` work state stays a
 * translucent container until Work actually enters it.
 */
export declare function resolveFactoryGraphVisualState(input: FactoryGraphVisualStateInput): FactoryGraphVisualState;
