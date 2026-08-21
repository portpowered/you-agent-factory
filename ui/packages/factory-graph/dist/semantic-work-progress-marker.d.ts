import type { ComponentPropsWithoutRef, ReactNode } from "react";
interface FactoryGraphWorkProgressMarkerBaseProps extends Omit<ComponentPropsWithoutRef<"span">, "children"> {
    ariaLabel: string;
    children?: ReactNode;
}
interface FactoryGraphWorkProgressNumericMarkerProps extends FactoryGraphWorkProgressMarkerBaseProps {
    count: number;
    kind: "numeric";
}
interface FactoryGraphWorkProgressDotsMarkerProps extends FactoryGraphWorkProgressMarkerBaseProps {
    active?: boolean;
    dotClassName?: string;
    dotCount: number;
    dotDataAttribute: string;
    kind: "dots";
    suffix?: ReactNode;
}
export type FactoryGraphWorkProgressMarkerProps = FactoryGraphWorkProgressNumericMarkerProps | FactoryGraphWorkProgressDotsMarkerProps;
/** Shared progress marker used by every Factory graph host. */
export declare function FactoryGraphWorkProgressMarker(props: FactoryGraphWorkProgressMarkerProps): import("react/jsx-runtime").JSX.Element;
export {};
