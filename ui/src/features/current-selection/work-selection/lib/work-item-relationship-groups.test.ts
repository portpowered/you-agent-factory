import { getCurrentSelectionDispatchHistoryMessages } from "../../base/messages/current-selection-dispatch-history";
import type { SelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import {
  buildRelationshipGroups,
  buildWorkRelationships,
  findRelationshipItems,
  relationshipDirectionGlyph,
  relationshipDirectionTone,
  relationshipPillTone,
} from "./work-item-relationship-groups";

const messages = getCurrentSelectionDispatchHistoryMessages("en");

describe("work item relationship groups", () => {
  it("projects ready relationship graphs into ordered relationship items", () => {
    const graph: SelectedWorkRelationshipGraph = {
      edges: [
        {
          relationship: "DEPENDS_ON",
          requiredState: "done",
          sourceWorkID: "work-active",
          targetWorkID: "work-api",
        },
        {
          relationship: "PARENT",
          sourceWorkID: "work-active",
          targetWorkID: "work-epic",
        },
        {
          relationship: "CHILD",
          sourceWorkID: "work-active",
          targetWorkID: "work-missing",
        },
      ],
      relatedWork: [
        {
          label: "API contract",
          state: "blocked",
          traceID: "trace-api",
          workID: "work-api",
          workTypeID: "task",
        },
        {
          label: "Epic",
          state: "ready",
          workID: "work-epic",
          workTypeID: "epic",
        },
      ],
      selectedWork: {
        label: "Active Story",
        state: "running",
        workID: "work-active",
        workTypeID: "story",
      },
      status: "ready",
    };

    const relationships = buildWorkRelationships(graph, messages);
    const groups = buildRelationshipGroups(relationships);

    expect(relationships).toHaveLength(2);
    expect(relationships.map((item) => item.group)).toEqual([
      "depends-on",
      "parent",
    ]);
    expect(findRelationshipItems(groups, "depends-on")[0]?.description).toBe(
      "Depends on (done)",
    );
    expect(findRelationshipItems(groups, "parent")[0]?.workLabel).toBe("Epic");
    expect(findRelationshipItems(groups, "child")).toEqual([]);
  });

  it("returns no relationships for loading or missing graphs", () => {
    expect(buildWorkRelationships(undefined, messages)).toEqual([]);
    expect(buildWorkRelationships({ status: "loading" }, messages)).toEqual([]);
  });

  it("maps relationship visual tokens from labels and groups", () => {
    expect(
      relationshipDirectionGlyph(messages.relationshipParentLabel, messages),
    ).toBe("↑");
    expect(
      relationshipDirectionTone(messages.relationshipDependsOnLabel, messages),
    ).toBe("warning");
    expect(relationshipPillTone("child")).toBe("active");
    expect(relationshipPillTone("related")).toBe("neutral");
  });
});
