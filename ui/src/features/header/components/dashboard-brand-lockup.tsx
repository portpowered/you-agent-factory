import youWordmark from "../../../assets/brand/you-wordmark.png";
import { cn } from "../../../lib/cn";
import { getHeaderControlsMessages } from "../messages/header-controls";

interface DashboardBrandLockupProps {
  className?: string;
  locale?: string;
  wordmarkClassName?: string;
}

export function DashboardBrandLockup({
  className = "",
  locale,
  wordmarkClassName: _wordmarkClassName = "",
}: DashboardBrandLockupProps) {
  const messages = getHeaderControlsMessages(locale);

  return (
    <span
      className={cn(
        "inline-flex min-w-0 items-center align-middle leading-none",
        className,
      )}
    >
      <img
        alt={messages.brandWordmark}
        className="h-12 w-12 shrink-0 rounded-sm object-cover"
        height={48}
        src={youWordmark}
        width={48}
      />
    </span>
  );
}
