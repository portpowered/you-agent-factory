import { validateFactoryDefinition } from "./api";
import { factoryValidationAPIErrorMessages } from "./messages";

const emptyFactory = {
  name: "alpha",
  workTypes: [],
  workers: [],
  workstations: [],
} as const;

describe("validateFactoryDefinition error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("throws NETWORK_ERROR when fetch is unavailable in the environment", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(validateFactoryDefinition(emptyFactory)).rejects.toMatchObject(
      {
        code: "NETWORK_ERROR",
        message: factoryValidationAPIErrorMessages.emptyEnvironment,
      },
    );
  });

  it("maps transport failures to NETWORK_ERROR", async () => {
    await expect(
      validateFactoryDefinition(emptyFactory, {
        fetch: vi.fn().mockRejectedValue(new Error("connection refused")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: factoryValidationAPIErrorMessages.network,
    });
  });

  it("falls back to rejected request copy for 400 responses without API message", async () => {
    await expect(
      validateFactoryDefinition(emptyFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ code: "INVALID_FACTORY" }), {
            headers: { "Content-Type": "application/json" },
            status: 400,
            statusText: "Bad Request",
          }),
        ),
      }),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      message: factoryValidationAPIErrorMessages.rejectedRequest,
      status: 400,
    });
  });

  it("maps 400 validation failures to BAD_REQUEST with API copy", async () => {
    await expect(
      validateFactoryDefinition(emptyFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "INVALID_FACTORY",
              message: "Factory definition rejected.",
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      message: "Factory definition rejected.",
      status: 400,
    });
  });

  it("maps non-400 validation failures to INTERNAL_ERROR with fallback copy", async () => {
    await expect(
      validateFactoryDefinition(emptyFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(null, {
            status: 503,
            statusText: "Service Unavailable",
          }),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: factoryValidationAPIErrorMessages.rejectedRequest,
      status: 503,
    });
  });

  it("forwards abort signals to the validation request", async () => {
    const controller = new AbortController();
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          targets: [],
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    await validateFactoryDefinition(emptyFactory, {
      fetch,
      signal: controller.signal,
    });

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/factory-validations"),
      expect.objectContaining({
        signal: controller.signal,
      }),
    );
  });

  it("rejects validation payloads with malformed targets", async () => {
    await expect(
      validateFactoryDefinition(emptyFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              targets: [
                {
                  code: "factory.workstation.missingRejectionRoute",
                  message: "Workstation repeater must define a reject route.",
                  severity: "error",
                  subject: {
                    id: "repeater",
                    location: "ON_REJECTION",
                  },
                },
              ],
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 200,
              statusText: "OK",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: factoryValidationAPIErrorMessages.invalidResponse,
    });
  });
});
