import { cx } from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
} from "../../components/ui/dashboard-typography";

const PANEL_CLASS =
  "rounded-3xl border border-af-overlay/10 bg-af-surface/72 shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const STATUS_PANEL_CLASS = cx(PANEL_CLASS, "mb-4 p-5 px-6");
const EYEBROW_CLASS =
  "mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const DASHBOARD_TITLE_CLASS = cx("m-0", DASHBOARD_PAGE_HEADING_CLASS);
const DETAIL_COPY_CLASS = cx("m-0 max-w-80", DASHBOARD_BODY_TEXT_CLASS);

interface DashboardStatusPanelProps {
  detail?: string;
  title: string;
  tone?: "default" | "error";
}

export function DashboardStatusPanel({
  detail,
  title,
  tone = "default",
}: DashboardStatusPanelProps) {
  const panelClassName =
    tone === "error" ? cx(STATUS_PANEL_CLASS, "border-af-danger/45") : STATUS_PANEL_CLASS;

  return (
    <section className={panelClassName}>
      <p className={EYEBROW_CLASS}>Agent Factory</p>
      <h1 className={DASHBOARD_TITLE_CLASS}>{title}</h1>
      {detail ? <p className={DETAIL_COPY_CLASS}>{detail}</p> : null}
    </section>
  );
}
