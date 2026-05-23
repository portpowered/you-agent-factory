import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getProviderSessionDetailMessages,
  type ProviderSessionDetailMessages,
} from "./provider-session-detail";
import {
  getProviderSessionWidgetMessages,
  type ProviderSessionWidgetMessages,
} from "./provider-session-widget";

const assertResolvedValue = (value: unknown) => {
  expect(typeof value).toBe("string");
  expect((value as string).length).toBeGreaterThan(0);
};

const assertCatalogValuesResolve = (
  catalog: Record<string, unknown>,
  invoke: (key: string, formatter: (...args: never[]) => unknown) => unknown[],
) => {
  for (const [key, value] of Object.entries(catalog)) {
    if (typeof value === "function") {
      for (const rendered of invoke(
        key,
        value as (...args: never[]) => unknown,
      )) {
        assertResolvedValue(rendered);
      }
      continue;
    }

    assertResolvedValue(value);
  }
};

const invokeProviderSessionDetail = (
  key: string,
  formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof ProviderSessionDetailMessages) {
    case "transcriptLineNumberLabel":
      return [formatter({ lineNumber: 42 } as never)];
    case "lineLabel":
    case "unknownEventOnLineLabel":
      return [formatter({ lineNumber: 42 } as never)];
    case "orderLabel":
      return [
        formatter({ order: 1 } as never),
        formatter({ order: 2, turnIndex: 7 } as never),
      ];
    case "transcriptTimestampLabel":
      return [formatter({ timestamp: "2026-05-22T11:00:00Z" } as never)];
    case "transcriptToggleLabel":
      return [
        formatter({ expanded: false, section: "Arguments" } as never),
        formatter({ expanded: true, section: "Output" } as never),
      ];
    case "transcriptTurnLabel":
      return [formatter({ turnIndex: 3 } as never)];
    case "turnLabel":
      return [formatter({ index: 3 } as never)];
    default:
      throw new Error(`Unhandled provider-session formatter ${key}`);
  }
};

const invokeProviderSessionWidget = (
  key: string,
  _formatter: (...args: never[]) => unknown,
) => {
  switch (key satisfies keyof ProviderSessionWidgetMessages) {
    default:
      throw new Error(`Unhandled provider-session widget formatter ${key}`);
  }
};

describe("provider-session-detail message catalogs", () => {
  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s provider-session detail value", (locale) => {
    assertCatalogValuesResolve(
      getProviderSessionDetailMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeProviderSessionDetail,
    );
  });

  it.each(
    SUPPORTED_LOCALES,
  )("resolves every %s provider-session widget value", (locale) => {
    assertCatalogValuesResolve(
      getProviderSessionWidgetMessages(locale) as unknown as Record<
        string,
        unknown
      >,
      invokeProviderSessionWidget,
    );
  });
});
