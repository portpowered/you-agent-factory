import {
  buildCurrentSelectionSaveSuccessStableIdentity,
  buildSaveErrorStableIdentity,
  buildSaveErrorToastOptions,
  buildSaveNotificationDeliveryKey,
  buildSaveSuccessStableIdentity,
  buildSaveSuccessToastOptions,
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
  shouldDeliverSaveNotification,
} from "./save-notification-delivery-policy";

describe("save notification delivery policy", () => {
  const errorIdentity = buildSaveErrorStableIdentity({
    message: "The graph is invalid.",
    code: "VALIDATION_FAILED",
  });

  it("builds stable error identity from kind, message, and code", () => {
    expect(errorIdentity).toEqual({
      kind: "error",
      stableId: "VALIDATION_FAILED:The graph is invalid.",
    });
  });

  it("builds delivery keys that include save-attempt revision", () => {
    expect(buildSaveNotificationDeliveryKey(errorIdentity, 1)).toBe(
      "error:VALIDATION_FAILED:The graph is invalid.#1",
    );
    expect(buildSaveNotificationDeliveryKey(errorIdentity, 2)).toBe(
      "error:VALIDATION_FAILED:The graph is invalid.#2",
    );
  });

  it("suppresses duplicate delivery for the same revision and stable identity", () => {
    const deliveryKey = buildSaveNotificationDeliveryKey(errorIdentity, 3);

    expect(shouldDeliverSaveNotification(deliveryKey, deliveryKey)).toBe(false);
  });

  it("delivers again when save-attempt revision increments with the same stable identity", () => {
    const firstAttemptKey = buildSaveNotificationDeliveryKey(errorIdentity, 1);
    const secondAttemptKey = buildSaveNotificationDeliveryKey(errorIdentity, 2);

    expect(
      shouldDeliverSaveNotification(secondAttemptKey, firstAttemptKey),
    ).toBe(true);
  });

  it("always delivers when stable identity differs", () => {
    const firstIdentity = buildSaveErrorStableIdentity({
      message: "First error",
    });
    const secondIdentity = buildSaveErrorStableIdentity({
      message: "Second error",
    });
    const firstKey = buildSaveNotificationDeliveryKey(firstIdentity, 1);
    const secondKey = buildSaveNotificationDeliveryKey(secondIdentity, 1);

    expect(shouldDeliverSaveNotification(secondKey, firstKey)).toBe(true);
  });

  it("uses persistent duration for errors and global TTL for success", () => {
    expect(buildSaveErrorToastOptions("details")).toEqual({
      description: "details",
      duration: PERSISTENT_TOAST_DURATION_MS,
    });
    expect(buildSaveSuccessToastOptions("saved")).toEqual({
      description: "saved",
      duration: GLOBAL_TOAST_DURATION_MS,
    });
  });

  it("builds a fixed stable identity for graph save success", () => {
    expect(buildSaveSuccessStableIdentity()).toEqual({
      kind: "success",
      stableId: "graph-save-success",
    });
  });

  it("builds per-entity stable identities for current-selection save success", () => {
    expect(buildCurrentSelectionSaveSuccessStableIdentity("worker")).toEqual({
      kind: "success",
      stableId: "worker-save-success",
    });
    expect(
      buildCurrentSelectionSaveSuccessStableIdentity("workstation").stableId,
    ).not.toBe(buildSaveSuccessStableIdentity().stableId);
  });
});
