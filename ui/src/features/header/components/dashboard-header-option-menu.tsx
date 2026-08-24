import { forwardRef, type HTMLAttributes, type ReactNode } from "react";

import { cn } from "../../../lib/cn";

const DASHBOARD_HEADER_OPTION_MENU_ITEM_BASE_CLASS =
  "inline-flex min-h-0 w-full items-center justify-start rounded-xl border px-3 py-2 text-sm font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0 disabled:pointer-events-none disabled:border-outline disabled:bg-surface-container-low disabled:text-on-surface-disabled";
const DASHBOARD_HEADER_OPTION_MENU_ITEM_UNSELECTED_CLASS =
  "border-transparent bg-transparent text-on-surface-variant hover:bg-af-overlay hover:text-on-surface";
const DASHBOARD_HEADER_OPTION_MENU_ITEM_SELECTED_CLASS =
  "border-primary bg-primary-container text-on-primary factory-light:text-on-primary-container";

export interface DashboardHeaderOptionMenuSurfaceProps
  extends HTMLAttributes<HTMLDivElement> {
  minWidthClassName?: string;
}

export const DashboardHeaderOptionMenuSurface = forwardRef<
  HTMLDivElement,
  DashboardHeaderOptionMenuSurfaceProps
>(function DashboardHeaderOptionMenuSurface(
  { className, minWidthClassName = "min-w-44", ...props },
  ref,
) {
  return (
    <div
      className={cn(
        "absolute right-0 top-full z-10 mt-2 overflow-hidden rounded-2xl border border-outline bg-surface-container-high p-1 text-on-surface shadow-af-panel backdrop-blur-lg",
        minWidthClassName,
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});

export interface DashboardHeaderOptionMenuItemProps {
  children: ReactNode;
  isSelected: boolean;
  onClick: () => void;
}

export function DashboardHeaderOptionMenuItem({
  children,
  isSelected,
  onClick,
}: DashboardHeaderOptionMenuItemProps) {
  return (
    <button
      aria-checked={isSelected}
      className={cn(
        DASHBOARD_HEADER_OPTION_MENU_ITEM_BASE_CLASS,
        isSelected
          ? DASHBOARD_HEADER_OPTION_MENU_ITEM_SELECTED_CLASS
          : DASHBOARD_HEADER_OPTION_MENU_ITEM_UNSELECTED_CLASS,
      )}
      onClick={onClick}
      role="menuitemradio"
      type="button"
    >
      <span className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-2 text-left">
        {children}
        {isSelected ? <DashboardHeaderOptionMenuCheckIcon /> : null}
      </span>
    </button>
  );
}

function DashboardHeaderOptionMenuCheckIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="16"
    >
      <path d="m5 12 4 4L19 6" />
    </svg>
  );
}
