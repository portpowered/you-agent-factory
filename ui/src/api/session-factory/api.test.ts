import { getSessionFactory, saveSessionFactory } from "./api";
import { SessionFactoryAPIError } from "./errors";

describe("getSessionFactory routing", () => {
  it("issues GET to the default session factory route", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await getSessionFactory("~default", { fetch });

    expect(fetch).toHaveBeenCalledWith("/factory-sessions/~default/factory", {
      method: "GET",
    });
  });

  it("issues GET to a non-default session factory route", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Scoped Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "3",
            physical: "2026-05-18T14:24:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await getSessionFactory("session-2", { fetch });

    expect(fetch).toHaveBeenCalledWith("/factory-sessions/session-2/factory", {
      method: "GET",
    });
  });
});

describe("getSessionFactory environment errors", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("throws when fetch is unavailable in the environment", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(getSessionFactory("~default")).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      name: "SessionFactoryAPIError",
    });
  });
});

describe("getSessionFactory response validation", () => {
  it("throws for invalid session factory response shape", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              version: {
                logical: "1",
                physical: "2026-05-18T14:25:00Z",
              },
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 200,
              statusText: "OK",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      name: "SessionFactoryAPIError",
    });
  });

  it("maps invalid factory definitions to INVALID_FACTORY", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Current Factory",
              workers: "not-an-array",
              workstations: [],
              workTypes: [],
              version: {
                logical: "1",
                physical: "2026-05-18T14:25:00Z",
              },
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 200,
              statusText: "OK",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY",
      name: "SessionFactoryAPIError",
    });
  });
});

describe("getSessionFactory API errors", () => {
  it("surfaces NOT_FOUND from the session factory API", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "NOT_FOUND",
              message: "Session factory not found.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 404,
              statusText: "Not Found",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "NOT_FOUND",
      name: "SessionFactoryAPIError",
      status: 404,
    });
  });
});

describe("saveSessionFactory routing", () => {
  it("issues PUT with REPLACE_CURRENT mode and incremented version on the default session", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "10",
            physical: "2026-05-18T14:40:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
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
          logical: "9",
          physical: "2026-05-18T14:25:00Z",
        },
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:25:00.001Z",
            },
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      }),
    );
  });

  it("issues PUT with UPSERT_NAMED_AND_ACTIVATE on a non-default session without version", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "1",
            physical: "2026-05-18T14:41:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await saveSessionFactory(
      {
        sessionID: "session-2",
        mode: "UPSERT_NAMED_AND_ACTIVATE",
        factory: {
          name: "Imported Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        includeVersion: false,
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Imported Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        method: "PUT",
      }),
    );
  });
});

describe("saveSessionFactory version handling", () => {
  it("issues PUT without version when baseVersion is absent", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "1",
            physical: "2026-05-18T14:25:00Z",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
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
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
      }),
    );
  });

  it("preserves non-ISO physical version timestamps when incrementing", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "2",
            physical: "not-a-date",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
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

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
            version: {
              logical: "2",
              physical: "not-a-date",
            },
          },
        }),
      }),
    );
  });
});

describe("saveSessionFactory network failures", () => {
  it("throws SessionFactoryAPIError for network failures", async () => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "~default",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockRejectedValue(new Error("socket closed")),
        },
      ),
    ).rejects.toBeInstanceOf(SessionFactoryAPIError);
  });
});
