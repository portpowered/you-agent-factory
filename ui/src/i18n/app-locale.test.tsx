import { fireEvent, render, screen } from "@testing-library/react";

import {
  AppLocaleProvider,
  resolveAppLocale,
  useAppLocale,
} from "./app-locale";

function LocaleProbe() {
  const { clearLocaleSelection, locale, setLocale } = useAppLocale();

  return (
    <>
      <p>{locale}</p>
      <button onClick={() => setLocale("zh-CN")} type="button">
        select zh-CN
      </button>
      <button onClick={() => clearLocaleSelection()} type="button">
        clear locale
      </button>
    </>
  );
}

describe("app locale provider", () => {
  it("resolves locale from browser language when no session or URL locale is present", () => {
    render(
      <AppLocaleProvider browserLanguage="zh-CN">
        <LocaleProbe />
      </AppLocaleProvider>,
    );

    expect(screen.getByText("zh-CN")).toBeTruthy();
  });

  it("resolves a locale alias from the URL search when no session locale is selected", () => {
    render(
      <AppLocaleProvider
        browserLanguage="en-US"
        locationSearch="?locale=zh-Hans"
      >
        <LocaleProbe />
      </AppLocaleProvider>,
    );

    expect(screen.getByText("zh-CN")).toBeTruthy();
  });

  it("resolves case-insensitive locale aliases from the URL search", () => {
    render(
      <AppLocaleProvider
        browserLanguage="en-US"
        locationSearch="?locale=ZH-HANS"
      >
        <LocaleProbe />
      </AppLocaleProvider>,
    );

    expect(screen.getByText("zh-CN")).toBeTruthy();
  });

  it("keeps the in-app session locale ahead of URL and browser language until cleared", () => {
    render(
      <AppLocaleProvider browserLanguage="en-US" locationSearch="?locale=en">
        <LocaleProbe />
      </AppLocaleProvider>,
    );

    expect(screen.getByText("en")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "select zh-CN" }));
    expect(screen.getByText("zh-CN")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "clear locale" }));
    expect(screen.getByText("en")).toBeTruthy();
  });

  it("lets tests and stories seed the initial locale directly", () => {
    render(
      <AppLocaleProvider initialLocale="zh-CN" locationSearch="?locale=en">
        <LocaleProbe />
      </AppLocaleProvider>,
    );

    expect(screen.getByText("zh-CN")).toBeTruthy();
  });
});

describe("resolveAppLocale", () => {
  it("falls back to English for an unsupported selected locale", () => {
    expect(
      resolveAppLocale({
        browserLanguage: "zh-CN",
        selectedLocale: "fr-FR",
      }),
    ).toBe("en");
  });

  it("walks the ordered browser language list until it finds a supported locale", () => {
    expect(
      resolveAppLocale({
        browserLanguage: "fr-FR",
        browserLanguages: ["fr-FR", "zh-Hans", "en-US"],
      }),
    ).toBe("zh-CN");
  });

  it("falls back to the standalone browser language when the preference list has no supported locale", () => {
    expect(
      resolveAppLocale({
        browserLanguage: "zh-CN",
        browserLanguages: ["fr-FR", "de-DE"],
      }),
    ).toBe("zh-CN");
  });

  it("resolves expanded Mandarin browser-language aliases predictably", () => {
    expect(
      resolveAppLocale({
        browserLanguage: "en-US",
        browserLanguages: ["zh-Hans-CN"],
      }),
    ).toBe("zh-CN");
  });
});
