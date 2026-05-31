import { saveSessionFactory } from "./api";
import { sessionFactoryOperatorErrorMessages } from "./operator-errors";

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

describe("saveSessionFactory version metadata", () => {
  it("preserves non-timestamp physical values when incrementing version metadata", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "2",
            physical: "legacy-physical",
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
          physical: "legacy-physical",
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
              physical: "legacy-physical",
            },
          },
        }),
      }),
    );
  });
});

describe("saveSessionFactory transport errors", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("throws NETWORK_ERROR when save fetch is unavailable", async () => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "~default",
          factory: sessionFactoryFixture,
        },
        {
          fetch: undefined,
        },
      ),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
    });
  });

  it("maps transport failures to NETWORK_ERROR", async () => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "~default",
          factory: sessionFactoryFixture,
        },
        {
          fetch: vi.fn().mockRejectedValue(new Error("connection refused")),
        },
      ),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
    });
  });

  it("preserves validation targets on session factory save failures", async () => {
    const validationTarget = {
      code: "STALE_FACTORY_VERSION",
      message: "The editable definition is stale.",
      severity: "error",
      subject: {
        id: "factory.version",
        location: "factory.version",
        type: "field",
      },
    };

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
                targets: [validationTarget],
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
      targets: [validationTarget],
    });
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
      message:
        "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
      status: 409,
    });
  });
});

describe("saveSessionFactory operator error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    ["FACTORY_NOT_IDLE", sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE],
    ["INVALID_FACTORY", sessionFactoryOperatorErrorMessages.INVALID_FACTORY],
    [
      "INVALID_FACTORY_NAME",
      sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
    ],
  ] as const)(
    "maps %s PUT failures to canonical operator copy",
    async (code, message) => {
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
                  code,
                  message: "Ignored API diagnostic.",
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
        code,
        message,
        status: 409,
      });
    },
  );
});
