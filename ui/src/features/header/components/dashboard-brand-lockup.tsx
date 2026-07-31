import { cn } from "../../../lib/cn";

interface DashboardBrandLockupProps {
  className?: string;
  locale?: string;
  wordmarkClassName?: string;
}

export function DashboardBrandLockup({
  className = "",
  locale: _locale,
  wordmarkClassName: _wordmarkClassName = "",
}: DashboardBrandLockupProps) {
  return (
    <span
      className={cn(
        "inline-flex min-w-0 items-center pl-2 align-middle leading-none",
        className,
      )}
    >
      <span className="inline-flex h-12 items-center justify-center gap-1 rounded-sm border border-primary bg-primary-container px-3 text-sm font-black uppercase leading-none tracking-[0.18em] text-primary">
        <span className="text-[1rem] leading-none">U</span>
      </span>
    </span>
  );
}
