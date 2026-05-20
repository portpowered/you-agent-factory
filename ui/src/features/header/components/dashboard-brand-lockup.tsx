import { cn } from "../../../lib/cn";
import { getHeaderControlsMessages } from "../messages/header-controls";

interface DashboardBrandLockupProps {
  className?: string;
  locale?: string;
  wordmarkClassName?: string;
}

const BRAND_MARK_CLASS =
  "inline-flex h-14 items-center justify-center gap-1.5 rounded-full border border-af-accent/28 bg-af-accent/12 px-4 text-[1rem] font-black uppercase leading-none tracking-[0.24em] text-af-accent";

export function DashboardBrandLockup({
  className = "",
  locale,
  wordmarkClassName = "",
}: DashboardBrandLockupProps) {
  const messages = getHeaderControlsMessages(locale);

  return (
    <span
      className={cn(
        "inline-flex min-w-0 items-center gap-4 align-middle leading-none",
        className,
      )}
    >
      <span aria-hidden="true" className={BRAND_MARK_CLASS}>
        <span className="text-[1.65rem] leading-none">∞</span>
        <span className="text-[1.12rem] leading-none">U</span>
      </span>
      <span className={cn("sr-only", wordmarkClassName)}>
        {messages.brandWordmark}
      </span>
    </span>
  );
}
