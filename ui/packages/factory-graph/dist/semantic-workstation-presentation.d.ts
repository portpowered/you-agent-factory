import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { type FactoryGraphWorkstationGuardedControl, type FactoryGraphWorkstationSemantics } from "./workstation-semantics.js";
export interface FactoryGraphWorkstationRef {
    node_id: string;
    transition_id: string;
    workstation_name: string;
    /** @deprecated Runtime topology metadata is not semantic authority. */
    worker_type?: string;
    /** @deprecated Runtime topology metadata is not semantic authority. */
    workstation_kind?: string;
}
export interface FactoryGraphWorkItemRef {
    display_name?: string;
    work_id: string;
    work_type_id?: string;
}
export interface FactoryGraphWorkstationPresentation extends FactoryGraphWorkstationSemantics {
    borderClassName?: string;
    className: string;
    iconKind: GraphSemanticIconKind;
    label: string;
    schedulingLabel: string | undefined;
}
/** Render-independent workstation presentation metadata for a graph node. */
export declare function factoryGraphWorkstationPresentation(semantics?: FactoryGraphWorkstationSemantics, locale?: string): FactoryGraphWorkstationPresentation;
export declare function factoryGraphWorkstationControlRoleLabel(controlRole: FactoryGraphWorkstationSemantics["controlRole"], locale?: string): string;
export declare function factoryGraphWorkstationGuardLimitValue(control: FactoryGraphWorkstationGuardedControl): string;
export declare function factoryGraphWorkstationGuardTargetLabel(locale?: string): string;
export declare function factoryGraphWorkstationGuardLimitLabel(locale?: string): string;
export declare function factoryGraphWorkItemLabel(item: FactoryGraphWorkItemRef): string;
export declare function factoryGraphGraphDuration(startedAt: string, now: number, locale?: string): string;
export declare function factoryGraphDurationText(startedAt: string, now: number, locale?: string): string;
export declare function factoryGraphActiveItemsLabel(count: number, locale?: string): string;
export declare function factoryGraphSelectWorkstationLabel(title: string, locale?: string): string;
export declare function factoryGraphWorkstationTitleClassName(label: string): string;
export declare function factoryGraphWorkItemLabelClassName(label: string): string;
export declare function factoryGraphClassNames(...values: Array<string | false | null | undefined>): string;
