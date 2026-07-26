import { Heading, Text } from "@you-agent-factory/components/primitives";
import type {
  FactoryWorkProgressCategory,
  FactoryWorkProgressProjection,
} from "@you-agent-factory/factory-replay";
import type { HTMLAttributes } from "react";

const CATEGORY_PRESENTATION = {
  queued: { cue: "○" },
  active: { cue: "▶" },
  completed: { cue: "✓" },
  failed: { cue: "!" },
  unclassified: { cue: "?" },
} as const satisfies Record<FactoryWorkProgressCategory, { cue: string }>;

const CATEGORY_ORDER = [
  "queued",
  "active",
  "completed",
  "failed",
  "unclassified",
] as const satisfies readonly FactoryWorkProgressCategory[];

export interface WorkProgressCategoryMessage {
  plural: (formattedCount: string) => string;
  singular: (formattedCount: string) => string;
}

export interface WorkProgressVisualizerMessages {
  categories: Record<FactoryWorkProgressCategory, WorkProgressCategoryMessage>;
  empty: string;
  regionLabel: string;
  title: string;
  total: (formattedTotal: string) => string;
}

export interface WorkProgressVisualizerProps
  extends Omit<HTMLAttributes<HTMLElement>, "children"> {
  formatNumber: (value: number) => string;
  messages: WorkProgressVisualizerMessages;
  projection: FactoryWorkProgressProjection;
}

function categoryMessage(
  count: number,
  formattedCount: string,
  message: WorkProgressCategoryMessage,
): string {
  return count === 1
    ? message.singular(formattedCount)
    : message.plural(formattedCount);
}

export function WorkProgressVisualizer({
  className,
  formatNumber,
  messages,
  projection,
  ...sectionProps
}: WorkProgressVisualizerProps) {
  const classNames = ["factory-work-progress", className]
    .filter(Boolean)
    .join(" ");

  return (
    <section
      aria-label={messages.regionLabel}
      className={classNames}
      data-work-progress-total={projection.total}
      {...sectionProps}
    >
      <header className="factory-work-progress__header">
        <Heading as="h2">{messages.title}</Heading>
        <Text as="p" className="factory-work-progress__total">
          {messages.total(formatNumber(projection.total))}
        </Text>
      </header>

      {projection.total === 0 ? (
        <Text as="p" className="factory-work-progress__empty" role="status">
          {messages.empty}
        </Text>
      ) : (
        <ul className="factory-work-progress__categories">
          {CATEGORY_ORDER.map((category) => {
            const count = projection.counts[category];
            const formattedCount = formatNumber(count);

            return (
              <li
                className="factory-work-progress__category"
                data-work-progress-category={category}
                key={category}
              >
                <span aria-hidden="true" className="factory-work-progress__cue">
                  {CATEGORY_PRESENTATION[category].cue}
                </span>
                <span className="factory-work-progress__message">
                  {categoryMessage(
                    count,
                    formattedCount,
                    messages.categories[category],
                  )}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
