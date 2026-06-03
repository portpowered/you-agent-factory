import { cn } from "../../../lib/cn";
import {
  WORK_STATE_PHASE_LEGEND_ORDER,
  workStatePhaseSwatchClassName,
} from "../lib/factory-graph-work-state-phase-styling";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export function FactoryGraphEditorWorkStatePhaseLegend({
  locale,
  visible,
}: {
  locale?: string;
  visible: boolean;
}) {
  if (!visible) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <section
      aria-label={messages.workStatePhaseLegendAriaLabel}
      className="pointer-events-auto absolute right-4 top-4 z-20 flex flex-wrap items-center gap-2 rounded-full border border-outline bg-surface-container-high px-3 py-2 shadow-af-panel backdrop-blur-[16px] max-md:left-4 max-md:right-4 max-md:top-4"
      data-factory-graph-work-state-phase-legend=""
    >
      <ul className="m-0 flex list-none flex-wrap items-center gap-2 p-0">
        {WORK_STATE_PHASE_LEGEND_ORDER.map((phase) => (
          <li
            className="flex items-center gap-1.5 text-xs leading-5 text-on-surface-variant"
            key={phase}
          >
            <span
              aria-hidden="true"
              className={cn(
                "h-3 w-3 shrink-0 rounded-sm border",
                workStatePhaseSwatchClassName(phase),
              )}
            />
            <span>{messages.workStatePhaseLegendLabel(phase)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
