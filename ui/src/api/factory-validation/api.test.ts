import { FactoryValidationAPIError, validateFactoryDefinition } from "./api";
import { factoryValidationAPIErrorMessages } from "./messages";

describe("validateFactoryDefinition", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns canonical validation targets for invalid factories", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
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
                type: "WORKSTATION",
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
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await validateFactoryDefinition({
      name: "alpha",
      workTypes: [],
      workers: [],
      workstations: [
        {
          inputs: [],
          name: "repeater",
          outputs: [],
          type: "REPEATER_WORKSTATION",
          worker: "worker-a",
        },
      ],
    });

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/factory-validations"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(result.targets).toHaveLength(1);
    expect(result.targets[0]?.subject.location).toBe("ON_REJECTION");
  });

  it("surfaces malformed validation responses as typed API errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ targets: "invalid" }), {
          headers: { "Content-Type": "application/json" },
          status: 200,
          statusText: "OK",
        }),
      ),
    );

    await expect(
      validateFactoryDefinition({
        name: "alpha",
        workTypes: [],
        workers: [],
        workstations: [],
      }),
    ).rejects.toBeInstanceOf(FactoryValidationAPIError);
  });
});

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
                  subject: "not-a-target",
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
