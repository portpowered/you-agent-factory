import {
  loadPublishedCliManifest,
  StaticCliCommandExplorer,
} from "./features/cli-command-explorer/public";
import { DashboardScreen } from "./features/dashboard/public";
import { CustomerFactoryEmulatorDemos } from "./features/factory-emulator/public";
import { AppNotificationToaster } from "./features/notifications/public";
import { AppLocaleProvider, useAppLocale } from "./i18n";
import { AppColorPaletteProvider } from "./theme";

export const CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH = "/factory-emulator-demos";
export const CLI_COMMAND_EXPLORER_PATH = "/cli";

export interface AppProps {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  initialLocale?: string | null;
  locationPathname?: string | null;
  locationSearch?: string | null;
}

export function App({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationPathname,
  locationSearch,
}: AppProps) {
  const pathname = locationPathname ?? window.location.pathname;

  return (
    <AppColorPaletteProvider>
      <AppLocaleProvider
        browserLanguage={browserLanguage}
        browserLanguages={browserLanguages}
        initialLocale={initialLocale}
        locationSearch={locationSearch}
      >
        {pathname === CLI_COMMAND_EXPLORER_PATH ? (
          <CliCommandExplorerPage />
        ) : pathname === CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH ? (
          <main className="min-h-screen overflow-x-hidden bg-surface p-1 md:p-2">
            <CustomerFactoryEmulatorDemos />
          </main>
        ) : (
          <DashboardScreen />
        )}
        <AppNotificationToaster />
      </AppLocaleProvider>
    </AppColorPaletteProvider>
  );
}

function CliCommandExplorerPage() {
  const { locale } = useAppLocale();
  return (
    <main className="min-h-screen overflow-x-hidden bg-surface p-3 md:p-6">
      <StaticCliCommandExplorer
        locale={locale}
        state={loadPublishedCliManifest()}
      />
    </main>
  );
}
