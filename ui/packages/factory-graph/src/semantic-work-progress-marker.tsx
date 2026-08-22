import type { ComponentPropsWithoutRef, ReactNode } from "react";

interface FactoryGraphWorkProgressMarkerBaseProps
  extends Omit<ComponentPropsWithoutRef<"span">, "children"> {
  ariaLabel: string;
  children?: ReactNode;
}

interface FactoryGraphWorkProgressNumericMarkerProps
  extends FactoryGraphWorkProgressMarkerBaseProps {
  count: number;
  kind: "numeric";
}

interface FactoryGraphWorkProgressDotsMarkerProps
  extends FactoryGraphWorkProgressMarkerBaseProps {
  active?: boolean;
  dotClassName?: string;
  dotCount: number;
  dotDataAttribute: string;
  kind: "dots";
  suffix?: ReactNode;
}

export type FactoryGraphWorkProgressMarkerProps =
  | FactoryGraphWorkProgressNumericMarkerProps
  | FactoryGraphWorkProgressDotsMarkerProps;

const WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME =
  "inline-flex min-h-6 items-center justify-center font-mono text-base font-bold leading-none text-on-surface";
const WORK_PROGRESS_DOTS_MARKER_CLASS_NAME = "items-center justify-center";
const WORK_PROGRESS_DOT_CLASS_NAME = "h-2 w-2";
const WORK_PROGRESS_ACTIVE_DOT_CLASS_NAME = "bg-on-surface";
const WORK_PROGRESS_IDLE_DOT_CLASS_NAME =
  "border border-outline-variant bg-surface";

/** Shared progress marker used by every Factory graph host. */
export function FactoryGraphWorkProgressMarker(
  props: FactoryGraphWorkProgressMarkerProps,
) {
  if (props.kind === "numeric") {
    const { ariaLabel, className, count, kind: _kind, ...rest } = props;

    return (
      <span
        aria-label={ariaLabel}
        className={classNames(
          WORK_PROGRESS_NUMERIC_MARKER_CLASS_NAME,
          className,
        )}
        role="status"
        {...rest}
      >
        {count}
      </span>
    );
  }

  const {
    active = true,
    ariaLabel,
    className,
    dotClassName = WORK_PROGRESS_DOT_CLASS_NAME,
    dotCount,
    dotDataAttribute,
    kind: _kind,
    suffix,
    ...rest
  } = props;
  const dotState = active ? "active" : "idle";
  const dotStateClassName = active
    ? WORK_PROGRESS_ACTIVE_DOT_CLASS_NAME
    : WORK_PROGRESS_IDLE_DOT_CLASS_NAME;

  return (
    <span
      aria-label={ariaLabel}
      className={classNames(WORK_PROGRESS_DOTS_MARKER_CLASS_NAME, className)}
      data-work-progress-state={dotState}
      role="status"
      {...rest}
    >
      {Array.from({ length: dotCount }, (_, dotNumber) => dotNumber + 1).map(
        (dotNumber) => (
          <span
            key={`${dotCount}-${dotNumber}`}
            aria-hidden="true"
            className={classNames(
              "rounded-full",
              dotStateClassName,
              dotClassName,
            )}
            data-current-activity-work-progress-dot={String(dotNumber - 1)}
            data-work-progress-dot-state={dotState}
            {...{ [dotDataAttribute]: String(dotNumber - 1) }}
          />
        ),
      )}
      {suffix}
    </span>
  );
}

function classNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}
