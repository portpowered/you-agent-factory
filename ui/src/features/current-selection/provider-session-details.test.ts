import { getLoadableProviderSessionRef } from "./provider-session-details";

describe("getLoadableProviderSessionRef", () => {
  it("returns the canonical typed request shape with dispatch context", () => {
    expect(
      getLoadableProviderSessionRef({
        dispatch_id: "dispatch-review-active",
        provider_session: {
          id: " sess_alpha ",
          kind: " session_id ",
          provider: " codex ",
        },
      }),
    ).toEqual({
      dispatchID: "dispatch-review-active",
      id: "sess_alpha",
      kind: "session_id",
      provider: "codex",
    });
  });

  it("returns null for non-loadable provider-session metadata", () => {
    expect(
      getLoadableProviderSessionRef({
        dispatch_id: "dispatch-review-active",
        provider_session: {
          id: "sess_alpha",
          kind: "path",
          provider: "codex",
        },
      }),
    ).toBeNull();
  });
});
