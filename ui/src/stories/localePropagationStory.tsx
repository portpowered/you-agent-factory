import { DashboardScreen } from "../features/dashboard";
import { AppLocaleProvider, useAppLocale } from "../i18n";

export function LocalePropagationStory() {
  return (
    <AppLocaleProvider initialLocale="en">
      <LocalePropagationControls />
      <div style={{ maxWidth: "100%", width: "1280px" }}>
        <DashboardScreen />
      </div>
    </AppLocaleProvider>
  );
}

function LocalePropagationControls() {
  const { locale, setLocale } = useAppLocale();

  return (
    <fieldset style={{ display: "flex", gap: "0.75rem", marginBottom: "1rem" }}>
      {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
      <legend>Locale verification controls</legend>
      {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
      <span>Current locale: {locale}</span>
      <button onClick={() => setLocale("en")} type="button">
        {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
        Switch to English
      </button>
      <button onClick={() => setLocale("zh-CN")} type="button">
        {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
        Switch to zh-CN
      </button>
    </fieldset>
  );
}
