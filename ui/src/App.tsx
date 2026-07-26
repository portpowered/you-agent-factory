import {
  CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
  PACKAGED_FACTORIES_HOSTED_PATH,
  PACKAGED_FACTORIES_PATH,
  resolveAppSurface,
} from "./features/app-routing/public";
import { DashboardScreen } from "./features/dashboard/public/screen";
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

export {
  CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
  PACKAGED_FACTORIES_HOSTED_PATH,
  PACKAGED_FACTORIES_PATH,
};

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
  const surface = resolveAppSurface(pathname);

  return (
    <AppColorPaletteProvider>
      <AppLocaleProvider
        browserLanguage={browserLanguage}
        browserLanguages={browserLanguages}
        initialLocale={initialLocale}
        locationSearch={locationSearch}
      >
        {surface === "customer-factory-emulator-demos" ? (
          <main className="min-h-screen overflow-x-hidden bg-surface p-1 md:p-2">
            <CustomerFactoryEmulatorDemos />
          </main>
        ) : surface === "packaged-factories" ? (
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
