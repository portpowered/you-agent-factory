import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardLocaleMenu } from "./dashboard-header-locale-menu";
import { DashboardPaletteMenu } from "./dashboard-palette-menu";

interface DashboardHeaderColorPaletteControlsProps {
  locale: string;
  onChangeLocale: (locale: string) => void;
}

export function DashboardHeaderColorPaletteControls({
  locale,
  onChangeLocale,
}: DashboardHeaderColorPaletteControlsProps) {
  const headerMessages = getHeaderControlsMessages(locale);

  return (
    <fieldset
      aria-label={headerMessages.globalHeaderActionsLabel}
      className="flex shrink-0 items-center gap-1.5 self-center"
    >
      <DashboardPaletteMenu locale={locale} />
      <DashboardLocaleMenu locale={locale} onChangeLocale={onChangeLocale} />
    </fieldset>
  );
}
