import { getCurrentFactory } from "./export";

const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;

describe("session factory export API", () => {
  it("reads the current factory as a direct canonical factory payload", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workTypes: [],
          workers: [],
          workstations: [],
          version: defaultSessionFactoryVersion,
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    await expect(
      getCurrentFactory({
        fetch: fetchMock,
      }),
    ).resolves.toEqual({
      name: "Current Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith("/factory-sessions/~default/factory", {
      method: "GET",
    });
  });

  it("reads the current factory through the session-scoped route for non-default sessions", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Session Factory",
          workTypes: [],
          workers: [],
          workstations: [],
          version: defaultSessionFactoryVersion,
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    await expect(
      getCurrentFactory({
        fetch: fetchMock,
        sessionID: "session-2",
      }),
    ).resolves.toEqual({
      name: "Session Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      {
        method: "GET",
      },
    );
  });

  it("maps NOT_FOUND current-factory failures to SessionFactoryAPIError", async () => {
    await expect(
      getCurrentFactory({
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
      message: "Session factory not found.",
      status: 404,
    });
  });

  it("maps INVALID_FACTORY_DEFINITION current-factory failures to INVALID_FACTORY", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Broken Factory",
              workers: [],
              workstations: [],
              workTypes: [],
              version: {
                logical: "1",
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

  it("rejects retired named-factory wrapper responses from the current factory endpoint", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              factory: {
                workTypes: [],
                workers: [],
                workstations: [],
              },
              name: "Current Factory",
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
      message: "The current factory editing API returned an invalid response.",
    });
  });
});
