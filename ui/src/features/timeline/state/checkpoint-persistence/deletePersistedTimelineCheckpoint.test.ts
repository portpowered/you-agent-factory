import { describe, expect, it } from "vitest";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import { deletePersistedTimelineCheckpoint } from "./deletePersistedTimelineCheckpoint";

describe("deletePersistedTimelineCheckpoint", () => {
  it("deletes only the envelope's existing storage key", async () => {
    const { indexedDB, records } =
      createTimelineCheckpointIndexedDBTestDouble();
    records.set("superseded-storage-key", {
      storageKey: "superseded-storage-key",
    });
    records.set("unaffected-storage-key", {
      storageKey: "unaffected-storage-key",
    });

    await deletePersistedTimelineCheckpoint(indexedDB, {
      storageKey: "superseded-storage-key",
    });

    expect(records.has("superseded-storage-key")).toBe(false);
    expect(records.has("unaffected-storage-key")).toBe(true);
  });

  it("preserves the envelope when an in-flight deletion is aborted", async () => {
    const persisted = { storageKey: "superseded-storage-key" };
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<typeof persisted>();
    records.set(persisted.storageKey, persisted);
    const controller = new AbortController();

    const deletion = deletePersistedTimelineCheckpoint(indexedDB, persisted, {
      signal: controller.signal,
    });
    await flushPromiseContinuations();
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["delete"]);

    controller.abort();
    controls.succeed("delete");
    await deletion;

    expect(records.get(persisted.storageKey)).toEqual(persisted);
  });
});
