import { cn } from "../../../lib/cn";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import { useAppLocale } from "../../../i18n";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";

const STATUS_PANEL_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "mb-4 p-4 md:p-5 md:px-6",
);
const EYEBROW_CLASS =
  "mb-3 text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const DASHBOARD_TITLE_CLASS = cn("m-0", DASHBOARD_PAGE_HEADING_CLASS);
const DETAIL_COPY_CLASS = cn("m-0 max-w-80", DASHBOARD_BODY_TEXT_CLASS);

interface DashboardStatusPanelProps {
  detail?: string;
  locale?: string;
  title: string;
  tone?: "default" | "error";
}

export function DashboardStatusPanel({
  detail,
  locale,
  title,
  tone = "default",
}: DashboardStatusPanelProps) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const detailClassName =
    tone === "error"
      ? cn(DETAIL_COPY_CLASS, "text-af-danger-ink")
      : DETAIL_COPY_CLASS;

  return (
    <section className={STATUS_PANEL_CLASS}>
      <p className={EYEBROW_CLASS}>
        <DashboardBrandLockup
          className="gap-2"
          locale={resolvedLocale}
          wordmarkClassName="truncate"
        />
      </p>
      <h1 className={DASHBOARD_TITLE_CLASS}>{title}</h1>
      {detail ? <p className={detailClassName}>{detail}</p> : null}
    </section>
  );
}
