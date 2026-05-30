import { factoryValidationAPIErrorMessages } from "./messages";
import { validateFactoryDefinition } from "./api";

const minimalFactoryDefinition = {
  name: "alpha",
  workTypes: [],
  workers: [],
  workstations: [],
};

function resetFetchStub() {
  vi.unstubAllGlobals();
}

describe("validateFactoryDefinition successful validation", () => {
  afterEach(resetFetchStub);

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
});

describe("validateFactoryDefinition invalid validation payloads", () => {
  afterEach(resetFetchStub);

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
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: factoryValidationAPIErrorMessages.invalidResponse,
    });
  });

  it("rejects validation targets that are not objects", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            targets: ["invalid-target"],
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
            statusText: "OK",
          },
        ),
      ),
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: factoryValidationAPIErrorMessages.invalidResponse,
    });
  });

  it("rejects validation targets with malformed subject payloads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            targets: [
              {
                code: "factory.invalid",
                message: "Invalid target.",
                severity: "error",
                subject: { id: "draft", type: "WORKSTATION" },
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
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: factoryValidationAPIErrorMessages.invalidResponse,
    });
  });
});

describe("validateFactoryDefinition transport failures", () => {
  afterEach(resetFetchStub);

  it("maps network failures to typed API errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new Error("connection refused")),
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: factoryValidationAPIErrorMessages.network,
    });
  });

  it("rejects environments without fetch", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: factoryValidationAPIErrorMessages.emptyEnvironment,
    });
  });
});

describe("validateFactoryDefinition HTTP error responses", () => {
  afterEach(resetFetchStub);

  it("maps 400 responses to BAD_REQUEST with API error messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            message: "Factory definition is not valid JSON.",
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 400,
            statusText: "Bad Request",
          },
        ),
      ),
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      message: "Factory definition is not valid JSON.",
      status: 400,
    });
  });

  it("falls back to rejected-request copy for 400 responses without messages", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({}), {
          headers: { "Content-Type": "application/json" },
          status: 400,
          statusText: "Bad Request",
        }),
      ),
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      message: factoryValidationAPIErrorMessages.rejectedRequest,
      status: 400,
    });
  });

  it("maps non-400 failures to INTERNAL_ERROR", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: "upstream unavailable" }), {
          headers: { "Content-Type": "application/json" },
          status: 503,
          statusText: "Service Unavailable",
        }),
      ),
    );

    await expect(
      validateFactoryDefinition(minimalFactoryDefinition),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "upstream unavailable",
      status: 503,
    });
  });
});
