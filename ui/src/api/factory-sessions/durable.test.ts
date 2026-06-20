import {
  getDurableFactorySession,
  getDurableFactorySessionResult,
  isDurableFactorySessionID,
} from "./durable";

describe("durable factory sessions API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("identifies durable factory session ids by prefix", () => {
    expect(isDurableFactorySessionID("dur-sess-petri-success-001")).toBe(true);
    expect(isDurableFactorySessionID("session-beta")).toBe(false);
    expect(isDurableFactorySessionID("~default")).toBe(false);
  });

  it("loads one durable factory session from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          orchestratorKind: "PETRI",
          resolvedSource: {
            kind: "FACTORY_ID",
            sourceRef: "factory/customer-support-triage",
          },
          sessionId: "dur-sess-petri-success-001",
          status: "SUCCEEDED",
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
      getDurableFactorySession("dur-sess-petri-success-001"),
    ).resolves.toEqual({
      orchestratorKind: "PETRI",
      resolvedSource: {
        kind: "FACTORY_ID",
        sourceRef: "factory/customer-support-triage",
      },
      sessionId: "dur-sess-petri-success-001",
      status: "SUCCEEDED",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-petri-success-001",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("loads durable final and partial result surfaces from the typed API", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            mode: "final",
            resultStatus: "FINAL",
            sessionId: "dur-sess-petri-success-001",
            sessionStatus: "SUCCEEDED",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            mode: "partial",
            resultStatus: "PARTIAL",
            sessionId: "dur-sess-petri-success-001",
            sessionStatus: "RUNNING",
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
      getDurableFactorySessionResult("dur-sess-petri-success-001", {
        mode: "final",
      }),
    ).resolves.toEqual({
      mode: "final",
      resultStatus: "FINAL",
      sessionId: "dur-sess-petri-success-001",
      sessionStatus: "SUCCEEDED",
    });
    await expect(
      getDurableFactorySessionResult("dur-sess-petri-success-001", {
        mode: "partial",
      }),
    ).resolves.toEqual({
      mode: "partial",
      resultStatus: "PARTIAL",
      sessionId: "dur-sess-petri-success-001",
      sessionStatus: "RUNNING",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/factory-sessions/dur-sess-petri-success-001/results?mode=final",
      expect.objectContaining({ method: "GET" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/dur-sess-petri-success-001/results?mode=partial",
      expect.objectContaining({ method: "GET" }),
    );
  });
});
