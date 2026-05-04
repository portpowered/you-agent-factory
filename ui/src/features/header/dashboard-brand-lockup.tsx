import { cx } from "../../components/ui";

interface DashboardBrandLockupProps {
  className?: string;
  wordmarkClassName?: string;
}

const BRAND_MARK_CLASS =
  "inline-flex h-8 items-center gap-1 rounded-full border border-af-accent/28 bg-af-accent/12 px-2.5 text-[0.72rem] font-black uppercase tracking-[0.2em] text-af-accent";

export function DashboardBrandLockup({
  className = "",
  wordmarkClassName = "",
}: DashboardBrandLockupProps) {
  return (
    <span className={cx("inline-flex min-w-0 items-center gap-3", className)}>
      <span aria-hidden="true" className={BRAND_MARK_CLASS}>
        <span className="text-[0.96rem] leading-none">∞</span>
        <span className="leading-none">U</span>
      </span>
      <span className={cx("min-w-0 truncate", wordmarkClassName)}>
        Infinite You
      </span>
    </span>
  );
}
