import { cn } from "../../../lib/cn";
import { getHeaderControlsMessages } from "../messages/header-controls";

interface DashboardBrandLockupProps {
  className?: string;
  locale?: string;
  wordmarkClassName?: string;
}

const BRAND_MARK_CLASS =
  "inline-flex h-12 items-center justify-center gap-1 rounded-full border border-af-accent-border bg-af-accent-surface px-3 text-sm font-black uppercase leading-none tracking-[0.18em] text-af-accent";

export function DashboardBrandLockup({
  className = "",
  locale,
  wordmarkClassName = "",
}: DashboardBrandLockupProps) {
  const messages = getHeaderControlsMessages(locale);

  return (
    <span
      className={cn(
        "inline-flex min-w-0 items-center gap-3 align-middle leading-none",
        className,
      )}
    >
      <span aria-hidden="true" className={BRAND_MARK_CLASS}>
        <span className="text-[1.45rem] leading-none">∞</span>
        <span className="text-[1rem] leading-none">U</span>
      </span>
      <span className={cn("sr-only", wordmarkClassName)}>
        {messages.brandWordmark}
      </span>
    </span>
  );
}
