import { useId, useState, type ButtonHTMLAttributes, type ReactNode } from "react";

const INLINE_TOOLTIP_CLASS =
  "pointer-events-none absolute left-1/2 top-full z-30 mt-2 -translate-x-1/2 rounded-xl border border-af-border-strong bg-af-surface-raised px-3 py-2 text-center text-xs font-medium text-af-text shadow-af-panel";

export function FactoryGraphEditorTooltipButton({
  children,
  className,
  tooltip,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
  className: string;
  tooltip: string;
}) {
  const tooltipID = useId();
  const [tooltipVisible, setTooltipVisible] = useState(false);

  return (
    <div className="relative inline-flex">
      <button
        {...props}
        aria-describedby={tooltipVisible ? tooltipID : undefined}
        className={className}
        onBlur={(event) => {
          props.onBlur?.(event);
          setTooltipVisible(false);
        }}
        onFocus={(event) => {
          props.onFocus?.(event);
          setTooltipVisible(true);
        }}
        onMouseEnter={(event) => {
          props.onMouseEnter?.(event);
          setTooltipVisible(true);
        }}
        onMouseLeave={(event) => {
          props.onMouseLeave?.(event);
          setTooltipVisible(false);
        }}
      >
        {children}
      </button>
      {tooltipVisible ? (
        <span className={INLINE_TOOLTIP_CLASS} id={tooltipID} role="tooltip">
          {tooltip}
        </span>
      ) : null}
    </div>
  );
}
