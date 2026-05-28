import { DashboardScreen } from "./features/dashboard/public";
import { AppNotificationToaster } from "./features/notifications/public";
import { AppLocaleProvider } from "./i18n";

export interface AppProps {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  initialLocale?: string | null;
  locationSearch?: string | null;
}

export function App({
  browserLanguage,
  browserLanguages,
  initialLocale,
  locationSearch,
}: AppProps) {
  return (
    <AppLocaleProvider
      browserLanguage={browserLanguage}
      browserLanguages={browserLanguages}
      initialLocale={initialLocale}
      locationSearch={locationSearch}
    >
      <DashboardScreen />
      <AppNotificationToaster />
    </AppLocaleProvider>
  );
}
