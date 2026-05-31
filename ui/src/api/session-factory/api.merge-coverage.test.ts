import { getSessionFactory, saveSessionFactory } from "./api";
import { SessionFactoryAPIError } from "./errors";
import { sessionFactoryAPIErrorMessages } from "./messages";

describe("saveSessionFactory environment and version edge cases", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("issues PUT without version in the factory payload when baseVersion is absent", async () => {
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
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        method: "PUT",
      }),
    );
  });

  it("throws NETWORK_ERROR when fetch is unavailable in the environment", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(
      saveSessionFactory({
        sessionID: "~default",
        factory: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        sessionFactoryAPIErrorMessages.unavailableInEnvironment,
        {
          code: "NETWORK_ERROR",
        },
      ),
    );
  });

  it("preserves non-ISO physical timestamps when incrementing version on save", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "10",
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
        mode: "REPLACE_CURRENT",
        factory: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
        baseVersion: {
          logical: "9",
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
              logical: "10",
              physical: "not-a-date",
            },
          },
        }),
        method: "PUT",
      }),
    );
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

describe("getSessionFactory structured error targets", () => {
  it("preserves validation targets on structured API error responses", async () => {
    const targets = [
      {
        code: "STALE_FACTORY_VERSION",
        message: "The session factory is stale.",
        severity: "ERROR",
        subject: {
          id: "factory",
          location: "factory.version",
          type: "FACTORY",
        },
      },
    ];

    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "STALE_FACTORY_VERSION",
              message: "The session factory is stale.",
              targets,
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 409,
              statusText: "Conflict",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      targets,
    });
  });
});
