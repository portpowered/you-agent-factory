import type { ComponentPropsWithoutRef, ReactNode } from "react";

import { cn } from "../../../lib/cn";

interface CurrentActivityWorkProgressMarkerBaseProps
  extends Omit<ComponentPropsWithoutRef<"span">, "children"> {
  ariaLabel: string;
  children?: ReactNode;
}

interface CurrentActivityWorkProgressNumericMarkerProps
  extends CurrentActivityWorkProgressMarkerBaseProps {
  count: number;
  kind: "numeric";
}

interface CurrentActivityWorkProgressDotsMarkerProps
  extends CurrentActivityWorkProgressMarkerBaseProps {
  dotClassName?: string;
  dotCount: number;
  dotDataAttribute: string;
  kind: "dots";
  suffix?: ReactNode;
}

export type CurrentActivityWorkProgressMarkerProps =
  | CurrentActivityWorkProgressNumericMarkerProps
  | CurrentActivityWorkProgressDotsMarkerProps;

const WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME =
  "items-center justify-center border border-af-success-border bg-success-container font-mono font-bold leading-none text-success";
const WORK_PROGRESS_DOTS_MARKER_CLASS_NAME =
  "items-center justify-center border border-af-success-border bg-success-container";

export function CurrentActivityWorkProgressMarker(
  props: CurrentActivityWorkProgressMarkerProps,
) {
  if (props.kind === "numeric") {
    const { ariaLabel, className, count, kind: _kind, ...rest } = props;

    return (
      <span
        aria-label={ariaLabel}
        className={cn(WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME, className)}
        role="status"
        {...rest}
      >
        {count}
      </span>
    );
  }

  const {
    ariaLabel,
    className,
    dotClassName = "h-2 w-2",
    dotCount,
    dotDataAttribute,
    kind: _kind,
    suffix,
    ...rest
  } = props;

  return (
    <span
      aria-label={ariaLabel}
      className={cn(WORK_PROGRESS_DOTS_MARKER_CLASS_NAME, className)}
      role="status"
      {...rest}
    >
      {Array.from({ length: dotCount }, (_, dotNumber) => dotNumber + 1).map(
        (dotNumber) => (
          <span
            key={`${dotCount}-${dotNumber}`}
            aria-hidden="true"
            className={cn("rounded-full bg-success", dotClassName)}
            data-current-activity-work-progress-dot={String(dotNumber - 1)}
            {...{ [dotDataAttribute]: String(dotNumber - 1) }}
          />
        ),
      )}
      {suffix}
    </span>
  );
}
