import { DashboardScreen } from "./features/dashboard/public";
import { CustomerFactoryEmulatorDemos } from "./features/factory-emulator/public";
import { AppNotificationToaster } from "./features/notifications/public";
import {
  type PackagedFactoryCopyText,
  PackagedFactoryInventory,
  type PackagedFactoryPublicDataSource,
  packagedFactoryPublicDataSource,
} from "./features/packaged-factories/public";
import { AppLocaleProvider } from "./i18n";
import { AppColorPaletteProvider } from "./theme";

export const CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH = "/factory-emulator-demos";
export const PACKAGED_FACTORIES_PATH = "/packaged-factories";

export interface AppProps {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  initialLocale?: string | null;
  locationPathname?: string | null;
  locationSearch?: string | null;
  packagedFactoryCopyText?: PackagedFactoryCopyText;
  packagedFactorySource?: PackagedFactoryPublicDataSource;
}

export function App({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationPathname,
  locationSearch,
  packagedFactoryCopyText,
  packagedFactorySource = packagedFactoryPublicDataSource,
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
        {pathname === CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH ? (
          <main className="min-h-screen overflow-x-hidden bg-surface p-1 md:p-2">
            <CustomerFactoryEmulatorDemos />
          </main>
        ) : pathname === PACKAGED_FACTORIES_PATH ? (
          <main className="min-h-screen overflow-x-hidden bg-surface p-4 md:p-6">
            <div className="mx-auto min-w-0 max-w-7xl">
              <PackagedFactoryInventory
                copyText={packagedFactoryCopyText}
                source={packagedFactorySource}
              />
            </div>
          </main>
        ) : (
          <DashboardScreen />
        )}
        <AppNotificationToaster />
      </AppLocaleProvider>
    </AppColorPaletteProvider>
  );
}
