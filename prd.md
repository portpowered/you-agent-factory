# PRD: Current Selection Work Relationships Graph Repair

## Context

### Customer ask

On the current-selection work relationships graph, only one relationship is shown when multiple relationships of the same kind should be shown. For example, when a selected work item has two `DEPENDS_ON` relationships, only the first appears on the graph. The customer also wants the relationship nodes to be easier to read: use more distinctive colors similar to workstation nodes in the factory graph, and prevent node text from being cut off by wrapping it or reducing font size.

### Problem

The current-selection work-item detail uses a shared relationship graph surface, but repeated relationship instances are not fully represented to the customer. This creates an inaccurate graph for selected work items with multiple dependencies and can hide required upstream work. Separately, the relationship nodes are visually flat and label text clips inside the node shell, making the graph difficult to scan and hard to trust.

### Solution

Preserve every supported relationship instance from the selected-work relationship data through graph projection and rendering, including repeated `DEPENDS_ON` edges with different targets. Then update the current-selection relationship-node presentation to use a clearer palette aligned with the factory graph's workstation-node treatment and make node labels readable by wrapping or shrinking text within the node bounds. Lock the behavior with fixture-based regression coverage using the factory batch example the customer called out and direct browser verification on the current-selection graph.

## Project-level acceptance criteria

- [ ] The current-selection work relationships graph shows every supported relationship instance for the selected work item instead of collapsing repeated relationships of the same type.
- [ ] A selected work item with two distinct `DEPENDS_ON` relationships renders both dependency nodes and both dependency edges on the graph.
- [ ] The concrete batch example in [factory-batch-local-agent-cli-runtime.json](/Users/abdifamily/infinite-you/factory/inputs/BATCH/default/factory-batch-local-agent-cli-runtime.json) can be used to reproduce and verify that multiple relationships now appear together.
- [ ] Related-work node selection from the graph remains keyboard-accessible and continues to switch current selection to the clicked or activated related work item.
- [ ] Relationship graph nodes use a more distinctive non-neutral palette aligned with workstation-node readability in the factory graph while preserving clear emphasis for the currently selected work item.
- [ ] Long relationship-node labels remain readable at common current-selection card widths without clipping outside the node shell.
- [ ] Quality gate: UI typecheck, lint, relevant automated tests, and browser verification pass.

## Goals

- Make the current-selection work relationships graph complete and trustworthy for work items with repeated relationship types.
- Preserve the existing loading, empty, error, and selection behaviors while repairing the graph data/rendering path.
- Improve at-a-glance readability of relationship nodes through clearer color treatment and label layout.
- Keep the work scoped to the current-selection work-item relationship graph rather than broad trace or factory-graph redesign.

## User stories

### work-relationships-current-selection-graph-repair-001: Preserve all relationship instances for selected work

**Description:** As a maintainer, I want the selected-work relationship graph model to retain every supported relationship instance so the current-selection graph can represent multiple dependencies and parent relations accurately.

**Acceptance Criteria:**

- [x] Building the current-selection selected-work relationship graph keeps every supported relationship instance connected to the selected work item instead of collapsing repeated relationships by relationship type alone.
- [x] When one selected work item has two distinct `DEPENDS_ON` relations to two different target work items, the ready graph contains two dependency edges and both related work nodes.
- [x] Existing supported relationship kinds for this graph (`DEPENDS_ON`, `PARENT_CHILD` as rendered parent relationships) continue to project correctly alongside repeated dependency relationships.
- [x] The projected graph remains deterministic for the same input snapshot so reviewers can compare outputs reliably in tests.
- [x] Focused regression tests cover the repeated-`DEPENDS_ON` case and a mixed dependency-plus-parent case.
- [x] Typecheck passes
- [x] Tests pass

### work-relationships-current-selection-graph-repair-002: Render every relationship on the current-selection work graph

**Description:** As a dashboard operator, I want the work relationships graph in current selection to show every related work item and edge so I can understand all of the selected work's dependencies and lineage from one place.

**Acceptance Criteria:**

- [ ] In the current-selection work-item detail card, the Work relationships region renders all related nodes and all relationship edges present in the ready selected-work relationship graph.
- [ ] Repeated `DEPENDS_ON` relations do not disappear simply because they share the same relationship label; each distinct related work item is visible and selectable from the graph.
- [ ] Activating each related-work node by click or keyboard still calls the existing current-selection work-switch action for that related work item.
- [ ] Loading, empty, and error states for the Work relationships region remain explicit and unchanged in meaning.
- [ ] Browser-visible verification covers the concrete batch example and confirms that multiple dependency relationships appear at the same time in current selection.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

### work-relationships-current-selection-graph-repair-003: Make relationship nodes readable and scannable

**Description:** As a dashboard operator, I want relationship nodes in the current-selection graph to have clearer colors and readable labels so I can identify the selected work and related work items without fighting the layout.

**Acceptance Criteria:**

- [ ] Relationship nodes in current selection use a more distinctive palette aligned with the workstation-node readability standard in the factory graph rather than the current inscrutable neutral treatment.
- [ ] The currently selected work item still has a clearly stronger emphasis than surrounding related-work nodes after the palette update.
- [ ] Long node labels no longer truncate awkwardly; labels wrap to additional lines, use a smaller readable font, or both, while staying inside the node shell.
- [ ] Readability improvements hold at common current-selection card widths, including narrower dashboard layouts, without text overlapping adjacent chrome.
- [ ] Accessible contrast, focus styling, and keyboard activation remain intact after the node styling update.
- [ ] Focused component or integration coverage proves the new node classes and non-clipping text behavior for long labels.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill

## High-level technical design

1. **Canonical data boundary**: Treat the selected-work relationship model as the source of truth for the current-selection graph. The graph model must preserve every supported relationship instance from the snapshot or selected-work relation payload rather than deduplicating by label or relation type.
2. **Projected graph boundary**: The shared relation-graph surface should receive projected nodes and edges that keep identity per relation instance or per source-target pair, so two `DEPENDS_ON` edges from one work item to two different work items both survive projection and render.
3. **UI surface boundary**: The current-selection work-item card remains responsible for loading, empty, error, and success states plus current-selection callbacks, while the shared graph surface handles only visual graph rendering and node interaction.
4. **Visual treatment**: Reuse existing semantic palette tokens already trusted on graph surfaces where possible. Apply them to relationship nodes in a way that makes the selected node, related nodes, hover, focus, and keyboard activation states remain distinguishable.
5. **Label layout**: Prefer CSS/layout changes that keep labels readable inside node bounds before adding bespoke truncation rules. If line wrapping is used, the node height and hit target must still remain stable enough for the current-selection card layout.
6. **Verification**: Cover the bug at the graph-model layer and at the rendered current-selection card layer, then verify the visible repair in the browser using the cited batch example.

## Functional requirements

- **FR-1:** The current-selection work relationships graph MUST render every supported relationship instance for the selected work item.
- **FR-2:** Multiple `DEPENDS_ON` relationships from one selected work item to different target work items MUST all appear simultaneously on the graph.
- **FR-3:** Existing parent/child relationship rendering MUST continue to work when repeated dependency relationships are also present.
- **FR-4:** Selecting a related node from the relationship graph MUST continue to switch current selection to that work item by mouse and keyboard activation.
- **FR-5:** The current-selection relationship graph MUST preserve explicit loading, empty, and error states.
- **FR-6:** Relationship nodes MUST use a clearer palette that improves distinction between selected and related work nodes.
- **FR-7:** Relationship-node labels MUST remain readable within node bounds at common dashboard widths without clipped text.

## Non-goals

- Redesigning the trace drill-down graph, dispatch relationship graph, or factory topology graph outside this current-selection work-item surface.
- Adding new relationship kinds or changing relationship semantics in backend event, API, or CLI contracts.
- Reworking graph layout algorithms beyond what is necessary to show all existing relationships and readable labels.
- Introducing user-configurable node colors, zoom controls, or broader current-selection card information architecture changes.

## Supporting technical and UX considerations

- Keep terminology aligned with the product vocabulary in `docs/architecture/data-model.md`; this is customer-visible work-item relationship behavior, not an internal graph-theory refactor.
- Follow `docs/internal/standards/code/general-website-standards.md` for accessible interactive graph nodes, readable contrast, keyboard behavior, and responsive layout.
- Use the customer's cited factory batch example as a stable regression case, but prove the repair through observable rendered behavior rather than file inventory assertions.
- Avoid widening scope into unrelated trace-drilldown node styling so reviewers can validate this lane independently.

## Success metrics

- Operators can see all dependencies and parent relationships for a selected work item without leaving current selection.
- No repeated-relationship regression remains for the known batch example with multiple `DEPENDS_ON` edges.
- Long work-item names on relationship nodes are readable without clipped text in the dashboard card.

## Open questions

None.
