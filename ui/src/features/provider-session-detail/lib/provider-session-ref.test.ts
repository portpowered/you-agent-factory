import { getLoadableProviderSessionRef } from "./provider-session-ref";

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

  it("returns a loadable Cursor session ref when provider metadata is cursor", () => {
    expect(
      getLoadableProviderSessionRef({
        dispatch_id: "dispatch-cursor-active",
        provider_session: {
          id: "cursor_sess_01",
          kind: "session_id",
          provider: " cursor ",
        },
      }),
    ).toEqual({
      dispatchID: "dispatch-cursor-active",
      id: "cursor_sess_01",
      kind: "session_id",
      provider: "cursor",
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
    expect(
      getLoadableProviderSessionRef({
        dispatch_id: "dispatch-review-active",
        provider_session: {
          id: "../cursor_sess",
          kind: "session_id",
          provider: "cursor",
        },
      }),
    ).toBeNull();
  });
});
