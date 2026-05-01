import {
  createNamedFactory,
  NamedFactoryAPIError,
} from "./api";

describe("createNamedFactory", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts the typed factory activation payload to /factory and returns the canonical response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factory: {
            workTypes: [],
            workers: [],
            workstations: [],
          },
          name: "Dropped Factory",
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
      createNamedFactory({
        factory: {
          workTypes: [],
          workers: [],
          workstations: [],
        },
        name: "Dropped Factory",
      }),
    ).resolves.toEqual({
      factory: {
        workTypes: [],
        workers: [],
        workstations: [],
      },
      name: "Dropped Factory",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory",
      expect.objectContaining({
        body: JSON.stringify({
          factory: {
            workTypes: [],
            workers: [],
            workstations: [],
          },
          name: "Dropped Factory",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("maps structured API failures into a typed activation error", async () => {
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
      createNamedFactory({
        factory: {
          workTypes: [],
          workers: [],
          workstations: [],
        },
        name: "Dropped Factory",
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
});
