import type { FactoryGraphSemanticPlaceRef } from "./semantic-place-nodes.js";
import type { FactoryGraphVisualState } from "./visual-state.js";
export declare function FactoryGraphPlaceSemanticIcon({ locale, place, visualState, }: {
    locale?: string;
    place: FactoryGraphSemanticPlaceRef;
    visualState: FactoryGraphVisualState;
}): import("react/jsx-runtime").JSX.Element;
export declare function FactoryGraphPlaceLabelText({ dataPrefix, place, }: {
    dataPrefix: "place" | "state";
    place: FactoryGraphSemanticPlaceRef;
}): import("react/jsx-runtime").JSX.Element;
export declare function factoryGraphPlaceLabel(place: FactoryGraphSemanticPlaceRef): string;
export declare function factoryGraphPlaceKindLabel(place: FactoryGraphSemanticPlaceRef, locale?: string): string;
