import { getSessionFactory, saveSessionFactory } from "./api";
import { SessionFactoryAPIError } from "./errors";
import { sessionFactoryAPIErrorMessages } from "./messages";
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

describe("getSessionFactory version normalization", () => {
  it("normalizes numeric logical version values from GET responses", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Numeric Version Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: 12,
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

    await expect(
      getSessionFactory("~default", { fetch }),
    ).resolves.toMatchObject({
      name: "Numeric Version Factory",
      version: {
        logical: "12",
        physical: "2026-05-18T14:25:00Z",
      },
    });
  });

  it("rejects GET responses with invalid version metadata", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Broken Version Factory",
              workers: [],
              workstations: [],
              workTypes: [],
              version: {
                logical: "9",
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

describe("getSessionFactory transport", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("throws NETWORK_ERROR when fetch is unavailable in the environment", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(getSessionFactory("~default")).rejects.toEqual(
      new SessionFactoryAPIError(
        sessionFactoryAPIErrorMessages.unavailableInEnvironment,
        {
          code: "NETWORK_ERROR",
        },
      ),
    );
  });
});

describe("getSessionFactory invalid responses", () => {
  it("rejects GET payloads that are not editable factory documents", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({ workers: [], workstations: [], workTypes: [] }),
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
});

describe("getSessionFactory HTTP error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("maps unrecognized API error codes on 4xx responses to INTERNAL_ERROR with API copy", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNKNOWN_CODE",
              message: "Unexpected failure.",
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
      message: "Unexpected failure.",
      status: 400,
    });
  });

  it("maps unrecognized API error codes on 5xx responses to rejected request copy", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNKNOWN_CODE",
              message: "Unexpected failure.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 500,
              statusText: "Internal Server Error",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: sessionFactoryAPIErrorMessages.rejectedRequest,
      status: 500,
    });
  });

  it.each([
    ["INVALID_FACTORY", sessionFactoryOperatorErrorMessages.INVALID_FACTORY],
    [
      "INVALID_FACTORY_NAME",
      sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
    ],
  ] as const)("maps %s GET failures to canonical operator copy", async (code, message) => {
    await expect(
      getSessionFactory("~default", {
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
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code,
      message,
      status: 400,
    });
  });
});

describe("saveSessionFactory HTTP error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    [
      "STALE_FACTORY_VERSION",
      sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
    ],
    ["FACTORY_NOT_IDLE", sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE],
    ["INVALID_FACTORY", sessionFactoryOperatorErrorMessages.INVALID_FACTORY],
    [
      "INVALID_FACTORY_NAME",
      sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
    ],
  ] as const)("maps %s PUT failures to canonical operator copy", async (code, message) => {
    await expect(
      saveSessionFactory(
        {
          sessionID: "session-review",
          factory: sessionFactoryFixture,
          mode: "REPLACE_CURRENT",
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
  });
});
