import { describe, expect, it } from "vitest";

import {
  FACTORY_SESSION_ID_SEARCH_PARAM,
  readFactorySessionIDSearchParam,
} from "./factory-session-search-param";

describe("readFactorySessionIDSearchParam", () => {
  it("returns the trimmed factory-session id from the dashboard URL search", () => {
    expect(
      readFactorySessionIDSearchParam(
        `?locale=en&${FACTORY_SESSION_ID_SEARCH_PARAM}= dur-sess-js-success-002 `,
      ),
    ).toBe("dur-sess-js-success-002");
  });

  it("returns null when the dashboard URL search does not select a session", () => {
    expect(readFactorySessionIDSearchParam("?locale=en")).toBeNull();
    expect(
      readFactorySessionIDSearchParam(
        `?${FACTORY_SESSION_ID_SEARCH_PARAM}=   `,
      ),
    ).toBeNull();
    expect(readFactorySessionIDSearchParam(null)).toBeNull();
  });
});
