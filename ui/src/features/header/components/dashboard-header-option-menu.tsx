import { forwardRef, type HTMLAttributes, type ReactNode } from "react";

import { DashboardActionButton } from "../../../components/ui";
import { cn } from "../../../lib/cn";

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
    <DashboardActionButton
      aria-checked={isSelected}
      className={cn(
        "min-h-0 w-full justify-start rounded-xl border-transparent px-3 py-2 text-sm",
        "[&>span]:grid [&>span]:w-full [&>span]:grid-cols-[minmax(0,1fr)_auto] [&>span]:items-center [&>span]:gap-2 [&>span]:text-left",
        isSelected
          ? "border-primary bg-primary-container text-on-surface"
          : "text-on-surface-variant",
      )}
      onClick={onClick}
      role="menuitemradio"
      tone={isSelected ? "secondary" : "ghost"}
      type="button"
    >
      {children}
      {isSelected ? <DashboardHeaderOptionMenuCheckIcon /> : null}
    </DashboardActionButton>
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
