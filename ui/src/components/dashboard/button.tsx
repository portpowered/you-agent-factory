import type { ButtonHTMLAttributes, ReactNode } from "react";

import { cx } from "./classnames";

export interface DashboardButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  busy?: boolean;
  children: ReactNode;
  tone?: "primary" | "secondary";
}

const DASHBOARD_BUTTON_CLASS =
  "inline-flex min-h-11 items-center justify-center rounded-lg border px-4 py-[0.65rem] font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-offset-0 disabled:cursor-not-allowed";
const DASHBOARD_BUTTON_TONE_CLASS: Record<NonNullable<DashboardButtonProps["tone"]>, string> = {
  primary:
    "border-af-accent/45 bg-af-accent text-af-accent-ink hover:border-af-accent hover:bg-af-accent-bright focus-visible:ring-af-accent/25 disabled:border-af-overlay/12 disabled:bg-af-overlay/8 disabled:text-af-ink/48",
  secondary:
    "border-af-accent/35 bg-af-accent/10 text-af-accent hover:bg-af-accent/15 focus-visible:ring-af-accent/25 disabled:border-af-overlay/10 disabled:bg-af-overlay/4 disabled:text-af-ink/42",
};

export function DashboardButton({
  busy = false,
  children,
  className,
  tone = "primary",
  type = "button",
  ...buttonProps
}: DashboardButtonProps) {
  return (
    <button
      aria-busy={busy ? "true" : undefined}
      className={cx(DASHBOARD_BUTTON_CLASS, DASHBOARD_BUTTON_TONE_CLASS[tone], className)}
      type={type}
      {...buttonProps}
    >
      {children}
    </button>
  );
}
