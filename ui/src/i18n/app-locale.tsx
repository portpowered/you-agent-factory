import type { ReactNode } from "react";
import { createContext, useContext, useMemo, useState } from "react";

import { resolveSupportedLocale, type SupportedLocale } from "./locales";

interface AppLocaleContextValue {
  clearLocaleSelection: () => void;
  locale: SupportedLocale;
  setLocale: (locale: string | null | undefined) => void;
}

export interface AppLocaleProviderProps {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  children: ReactNode;
  initialLocale?: string | null;
  locationSearch?: string | null;
}

export interface ResolveAppLocaleInput {
  browserLanguage?: string | null;
  browserLanguages?: readonly string[] | null;
  locationSearch?: string | null;
  selectedLocale?: string | null;
}

const AppLocaleContext = createContext<AppLocaleContextValue | null>(null);

export function AppLocaleProvider({
  browserLanguage,
  browserLanguages,
  children,
  initialLocale,
  locationSearch,
}: AppLocaleProviderProps) {
  const [selectedLocale, setSelectedLocale] = useState<
    string | null | undefined
  >(initialLocale);
  const locale = useMemo(
    () =>
      resolveAppLocale({
        browserLanguage,
        browserLanguages,
        locationSearch,
        selectedLocale,
      }),
    [browserLanguage, browserLanguages, locationSearch, selectedLocale],
  );
  const value = useMemo<AppLocaleContextValue>(
    () => ({
      clearLocaleSelection: () => {
        setSelectedLocale(null);
      },
      locale,
      setLocale: (nextLocale) => {
        setSelectedLocale(nextLocale);
      },
    }),
    [locale],
  );

  return (
    <AppLocaleContext.Provider value={value}>
      {children}
    </AppLocaleContext.Provider>
  );
}

export function resolveAppLocale({
  browserLanguage,
  browserLanguages,
  locationSearch,
  selectedLocale,
}: ResolveAppLocaleInput): SupportedLocale {
  if (selectedLocale !== undefined && selectedLocale !== null) {
    return resolveSupportedLocale(selectedLocale);
  }

  const urlLocale = readLocaleSearchParam(locationSearch);
  if (urlLocale !== undefined) {
    return resolveSupportedLocale(urlLocale);
  }

  for (const locale of browserLanguages ?? []) {
    if (locale.trim().length === 0) {
      continue;
    }

    const resolvedLocale = resolveSupportedLocale(locale);
    if (resolvedLocale !== "en" || locale.toLowerCase().startsWith("en")) {
      return resolvedLocale;
    }
  }

  return resolveSupportedLocale(browserLanguage);
}

export function useAppLocale(
  localeOverride?: string | null,
): AppLocaleContextValue {
  const context = useContext(AppLocaleContext);

  if (localeOverride !== undefined && localeOverride !== null) {
    return {
      clearLocaleSelection: () => {},
      locale: resolveSupportedLocale(localeOverride),
      setLocale: () => {},
    };
  }

  if (context) {
    return context;
  }

  return {
    clearLocaleSelection: () => {},
    locale: resolveAppLocale({
      browserLanguage:
        typeof navigator === "undefined" ? undefined : navigator.language,
      browserLanguages:
        typeof navigator === "undefined" ? undefined : navigator.languages,
      locationSearch:
        typeof window === "undefined" ? undefined : window.location.search,
    }),
    setLocale: () => {},
  };
}

function readLocaleSearchParam(
  locationSearch: string | null | undefined,
): string | undefined {
  if (!locationSearch) {
    return undefined;
  }

  return new URLSearchParams(locationSearch).get("locale") ?? undefined;
}
