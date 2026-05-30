import {
  FactoryValidationAPIError,
  validateFactoryDefinition,
} from "./api";

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
