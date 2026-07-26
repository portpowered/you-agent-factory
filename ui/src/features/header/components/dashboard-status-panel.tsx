import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import { Heading } from "@you-agent-factory/components/primitives";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { useAppLocale } from "../../../i18n";
import { DashboardBrandLockup } from "./dashboard-brand-lockup";

interface DashboardStatusPanelProps {
  actions?: ReactNode;
  detail?: string;
  locale?: string;
  title: string;
  tone?: "default" | "error";
}

export function DashboardStatusPanel({
  actions,
  detail,
  locale,
  title,
  tone = "default",
}: DashboardStatusPanelProps) {
  const { locale: resolvedLocale } = useAppLocale(locale);
  const detailClassName =
    tone === "error" ? "max-w-80 text-on-error-container" : "max-w-80";

  return (
    <DashboardPanelShell className="mb-4 p-4 md:p-5 md:px-6">
      <p className="mb-3 text-xs font-bold uppercase tracking-[0.16em] text-primary">
        <DashboardBrandLockup
          className="gap-2"
          locale={resolvedLocale}
          wordmarkClassName="truncate"
        />
      </p>
      <Heading className="m-0" level="page">
        {title}
      </Heading>
      {detail ? (
        <WidgetDetailCopy className={detailClassName}>
          {detail}
        </WidgetDetailCopy>
      ) : null}
      {actions ? <div className="mt-4">{actions}</div> : null}
    </DashboardPanelShell>
  );
}
