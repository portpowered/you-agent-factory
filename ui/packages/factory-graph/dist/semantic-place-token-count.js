import { jsx as _jsx } from "react/jsx-runtime";
import { FactoryGraphNodeBadge } from "./semantic-support-nodes.js";
export function FactoryGraphPlaceTokenCount({ ariaLabel, count, }) {
    return (_jsx(FactoryGraphNodeBadge, { "aria-label": ariaLabel, className: "w-fit", "data-place-token-count": true, role: "status", children: count }));
}
