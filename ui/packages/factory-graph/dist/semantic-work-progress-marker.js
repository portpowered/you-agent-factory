import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
const WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME = "inline-flex min-h-6 items-center justify-center font-mono text-base font-bold leading-none text-on-surface";
const WORK_PROGRESS_DOTS_MARKER_CLASS_NAME = "items-center justify-center";
const WORK_PROGRESS_DOT_CLASS_NAME = "h-2 w-2";
const WORK_PROGRESS_ACTIVE_DOT_CLASS_NAME = "bg-on-surface";
const WORK_PROGRESS_IDLE_DOT_CLASS_NAME = "border border-outline-variant bg-surface";
/** Shared progress marker used by every Factory graph host. */
export function FactoryGraphWorkProgressMarker(props) {
    if (props.kind === "numeric") {
        const { ariaLabel, className, count, kind: _kind, ...rest } = props;
        return (_jsx("span", { "aria-label": ariaLabel, className: classNames(WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME, className), role: "status", ...rest, children: count }));
    }
    const { active = true, ariaLabel, className, dotClassName = WORK_PROGRESS_DOT_CLASS_NAME, dotCount, dotDataAttribute, kind: _kind, suffix, ...rest } = props;
    const dotState = active ? "active" : "idle";
    const dotStateClassName = active
        ? WORK_PROGRESS_ACTIVE_DOT_CLASS_NAME
        : WORK_PROGRESS_IDLE_DOT_CLASS_NAME;
    return (_jsxs("span", { "aria-label": ariaLabel, className: classNames(WORK_PROGRESS_DOTS_MARKER_CLASS_NAME, className), "data-work-progress-state": dotState, role: "status", ...rest, children: [Array.from({ length: dotCount }, (_, dotNumber) => dotNumber + 1).map((dotNumber) => (_jsx("span", { "aria-hidden": "true", className: classNames("rounded-full", dotStateClassName, dotClassName), "data-current-activity-work-progress-dot": String(dotNumber - 1), "data-work-progress-dot-state": dotState, [dotDataAttribute]: String(dotNumber - 1) }, `${dotCount}-${dotNumber}`))), suffix] }));
}
function classNames(...values) {
    return values.filter(Boolean).join(" ");
}
