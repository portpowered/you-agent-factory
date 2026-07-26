import {
  CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
  PACKAGED_FACTORIES_HOSTED_PATH,
  PACKAGED_FACTORIES_PATH,
  resolveAppSurface,
} from "./features/app-routing/lib/resolve-app-surface";
import { DashboardScreen } from "./features/dashboard/public/screen";
import { CustomerFactoryEmulatorDemos } from "./features/factory-emulator/components/customer-factory-emulator-demos";
import { AppNotificationToaster } from "./features/notifications/components/app-notification-toaster";
import type { PackagedFactoryCopyText } from "./features/packaged-factories/components/packaged-factory-detail";
import { PackagedFactoryInventory } from "./features/packaged-factories/components/packaged-factory-inventory";
import { packagedFactoryPublicDataSource } from "./features/packaged-factories/lib/generated/public-package-data";
import type { PackagedFactoryPublicDataSource } from "./features/packaged-factories/lib/public-contract";
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
