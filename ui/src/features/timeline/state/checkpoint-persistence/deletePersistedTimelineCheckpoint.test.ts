import { describe, expect, it } from "vitest";
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
});
