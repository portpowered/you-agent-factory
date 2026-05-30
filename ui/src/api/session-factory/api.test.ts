import {
  getSessionFactory,
  SessionFactoryAPIError,
  saveSessionFactory,
} from "./api";

const sessionFactoryFixture = {
  name: "Current Factory",
  workers: [],
  workstations: [],
  workTypes: [],
  version: {
    logical: "7",
    physical: "2026-05-18T14:22:00Z",
  },
};

describe("getSessionFactory", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("GETs the default session factory through the session-scoped route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(sessionFactoryFixture), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 200,
        statusText: "OK",
      }),
    );

    const document = await getSessionFactory("~default", {
      fetch: fetchMock,
    });

    expect(document).toEqual(sessionFactoryFixture);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      {
        method: "GET",
      },
    );
  });

  it("GETs a non-default session factory through the session-scoped route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(sessionFactoryFixture), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 200,
        statusText: "OK",
      }),
    );

    await getSessionFactory("session-2", {
      fetch: fetchMock,
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      {
        method: "GET",
      },
    );
  });
});

describe("saveSessionFactory", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("PUTs REPLACE_CURRENT with the wrapped factory payload on the default session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ...sessionFactoryFixture,
          version: {
            logical: "8",
            physical: "2026-05-18T14:23:00Z",
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
        factory: sessionFactoryFixture,
      },
      {
        fetch: fetchMock,
      },
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      {
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: sessionFactoryFixture,
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });

  it("PUTs UPSERT_NAMED_AND_ACTIVATE on a non-default session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(sessionFactoryFixture), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 200,
        statusText: "OK",
      }),
    );

    await saveSessionFactory(
      {
        sessionID: "session-2",
        mode: "UPSERT_NAMED_AND_ACTIVATE",
        factory: {
          ...sessionFactoryFixture,
          name: "imported-factory",
        },
      },
      {
        fetch: fetchMock,
      },
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      {
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            ...sessionFactoryFixture,
            name: "imported-factory",
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });

  it("omits mode from the PUT body when save mode is not provided", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(sessionFactoryFixture), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 200,
        statusText: "OK",
      }),
    );

    await saveSessionFactory(
      {
        sessionID: "~default",
        factory: sessionFactoryFixture,
      },
      {
        fetch: fetchMock,
      },
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      {
        body: JSON.stringify({
          factory: sessionFactoryFixture,
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });
});

describe("saveSessionFactory errors", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("surfaces session factory transport failures with the original API error code", async () => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "~default",
          factory: sessionFactoryFixture,
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "STALE_FACTORY_VERSION",
                message: "Factory version is stale.",
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
        },
      ),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      status: 409,
    });
  });
});

describe("getSessionFactory errors", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("throws SessionFactoryAPIError when fetch is unavailable", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: undefined,
      }),
    ).rejects.toBeInstanceOf(SessionFactoryAPIError);
  });

  it("maps transport failures to NETWORK_ERROR when fetch rejects", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockRejectedValue(new Error("connection refused")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
    });
  });

  it("maps unknown API error codes to INTERNAL_ERROR", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNEXPECTED_CODE",
              message: "Something went wrong.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "Something went wrong.",
      status: 400,
    });
  });

  it("rejects successful responses that are not session factory documents", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ workers: [] }), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
            statusText: "OK",
          }),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
    });
  });
});
