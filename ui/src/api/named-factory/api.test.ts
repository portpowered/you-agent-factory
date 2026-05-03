import {
  createFactory,
  getCurrentFactory,
  NamedFactoryAPIError,
} from "./api";

describe("factory API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts the direct canonical factory payload to /factory and returns the canonical response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createFactory({
        name: "Dropped Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      }),
    ).resolves.toEqual({
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("maps structured activation failures into a typed API error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "FACTORY_NOT_IDLE",
            message: "Current factory runtime must be idle before activation.",
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
    );

    await expect(
      createFactory({
        name: "Dropped Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Current factory runtime must be idle before activation.", {
        code: "FACTORY_NOT_IDLE",
        status: 409,
        statusText: "Conflict",
        responseBody: {
          code: "FACTORY_NOT_IDLE",
          message: "Current factory runtime must be idle before activation.",
        },
      }),
    );
  });

  it("reads the current factory as a direct canonical factory payload", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            name: "Current Factory",
            workTypes: [],
            workers: [],
            workstations: [],
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      ),
    );

    await expect(getCurrentFactory()).resolves.toEqual({
      name: "Current Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
  });

  it("rejects retired named-factory wrapper responses from the current factory endpoint", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
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
    );

    await expect(getCurrentFactory()).rejects.toEqual(
      new NamedFactoryAPIError("The current factory API returned an invalid response.", {
        code: "INTERNAL_ERROR",
        responseBody: {
          factory: {
            workTypes: [],
            workers: [],
            workstations: [],
          },
          name: "Current Factory",
        },
        status: 200,
        statusText: "OK",
      }),
    );
  });
});

