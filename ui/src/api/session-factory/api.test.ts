import { getSessionFactory } from "./api";

describe("getSessionFactory", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

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
