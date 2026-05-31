import {
  getSessionFactory,
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
} from "./index";

describe("session-factory index exports", () => {
  it("re-exports session factory transport and error helpers", async () => {
    expect(normalizeSessionFactoryAPIErrorCode("STALE_FACTORY_VERSION")).toBe(
      "STALE_FACTORY_VERSION",
    );
    expect(
      new SessionFactoryAPIError("save failed", { code: "BAD_REQUEST" }).code,
    ).toBe("BAD_REQUEST");

    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: { logical: "1", physical: "2026-05-31T00:00:00Z" },
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await getSessionFactory("~default", { fetch });
    expect(fetch).toHaveBeenCalledOnce();
  });
});
