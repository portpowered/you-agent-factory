import { sessionFactoryAPIErrorMessages } from "./messages";
import { getSessionFactory, saveSessionFactory } from "./api";
describe("getSessionFactory response normalization", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("rejects a successful response without an editable factory shape", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ version: { logical: "1", physical: "2026-05-31T00:00:00Z" } }), {
            headers: { "Content-Type": "application/json" },
            status: 200,
            statusText: "OK",
          }),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: sessionFactoryAPIErrorMessages.invalidResponse,
    });
  });

  it("surfaces NETWORK_ERROR when fetch is unavailable", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: null as never,
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      name: "SessionFactoryAPIError",
    });
  });

  it("surfaces NETWORK_ERROR when fetch rejects", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockRejectedValue(new Error("offline")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: sessionFactoryAPIErrorMessages.network,
      name: "SessionFactoryAPIError",
    });
  });
});

describe("saveSessionFactory version increment", () => {
  it("keeps a non-parseable physical version unchanged when incrementing", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: { logical: "2", physical: "not-a-date" },
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await saveSessionFactory(
      {
        sessionID: "~default",
        factory: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        baseVersion: {
          logical: "1",
          physical: "not-a-date",
        },
      },
      { fetch },
    );

    const requestBody = JSON.parse(
      (fetch.mock.calls[0]?.[1] as RequestInit | undefined)?.body as string,
    );
    expect(requestBody.factory.version).toEqual({
      logical: "2",
      physical: "not-a-date",
    });
  });
});
