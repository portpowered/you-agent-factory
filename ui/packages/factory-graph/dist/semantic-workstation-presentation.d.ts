import type { GraphSemanticIconKind } from "./semantic-icon.js";
export interface FactoryGraphWorkstationRef {
    node_id: string;
    transition_id: string;
    workstation_name: string;
    worker_type?: string;
    workstation_kind?: string;
}
export interface FactoryGraphWorkItemRef {
    display_name?: string;
    work_id: string;
    work_type_id?: string;
}
export type FactoryGraphWorkstationSemanticKind = "CRON" | "POLLER" | "REPEATER" | "STANDARD" | "exhaustion";
export interface FactoryGraphWorkstationPresentation {
    borderClassName?: string;
    className: string;
    iconKind: GraphSemanticIconKind;
    label: string;
    semanticKind: FactoryGraphWorkstationSemanticKind;
}
export declare function factoryGraphWorkstationPresentation(workstation: FactoryGraphWorkstationRef, locale?: string): FactoryGraphWorkstationPresentation;
export declare function factoryGraphWorkItemLabel(item: FactoryGraphWorkItemRef): string;
export declare function factoryGraphGraphDuration(startedAt: string, now: number, locale?: string): string;
export declare function factoryGraphDurationText(startedAt: string, now: number, locale?: string): string;
export declare function factoryGraphActiveItemsLabel(count: number, locale?: string): string;
export declare function factoryGraphSelectWorkstationLabel(title: string, locale?: string): string;
export declare function factoryGraphSelectExhaustionLabel(title: string, locale?: string): string;
export declare function factoryGraphWorkstationTitleClassName(label: string): string;
export declare function factoryGraphWorkItemLabelClassName(label: string): string;
export declare function factoryGraphClassNames(...values: Array<string | false | null | undefined>): string;
