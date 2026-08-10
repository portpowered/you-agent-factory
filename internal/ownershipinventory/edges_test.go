package ownershipinventory_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestValidateRequiresCrossServiceEdgeClassifications(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	inventory.CrossServiceEdges = nil

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed without cross-service edges")
	}
	if len(report.MissingCrossServiceEdges) == 0 && !report.MissingCrossServiceEdgeTable {
		t.Fatalf("expected missing cross-service edge table; report=%#v", report)
	}
}

func TestValidateFailsWhenEdgeLacksClassification(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.CrossServiceEdges) == 0 {
		t.Fatal("expected frozen cross-service edges")
	}
	inventory.CrossServiceEdges[0].Class = ""

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with blank edge class")
	}
	if len(report.InvalidEdgeClassifications) == 0 {
		t.Fatalf("invalid edge classifications empty; report=%#v", report)
	}
}

func TestValidateFailsWhenEdgeUsesUnknownClass(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.CrossServiceEdges) == 0 {
		t.Fatal("expected frozen cross-service edges")
	}
	inventory.CrossServiceEdges[0].Class = "rpc"

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with unknown edge class")
	}
	if len(report.InvalidEdgeClassifications) == 0 {
		t.Fatalf("invalid edge classifications empty; report=%#v", report)
	}
}

func TestValidateRequiresProcessEdgesEdgesAreConstructionOrExternalEffect(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	found := false
	for i := range inventory.CrossServiceEdges {
		edge := &inventory.CrossServiceEdges[i]
		if edge.FromOwner == ownershipinventory.DestinationEdges || edge.ToOwner == ownershipinventory.DestinationEdges {
			edge.Class = ownershipinventory.EdgeClassCommand
			edge.ArchitectureException = true
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one Process Edges cross-service edge in frozen inventory")
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with Process Edges edge classified as command")
	}
	if len(report.InvalidEdgeClassifications) == 0 {
		t.Fatalf("invalid edge classifications empty; report=%#v", report)
	}
}

func TestValidateFailsWhenCrossServiceEdgesNotStableSorted(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	if len(inventory.CrossServiceEdges) < 2 {
		t.Fatal("need at least two cross-service edges")
	}
	inventory.CrossServiceEdges[0], inventory.CrossServiceEdges[1] = inventory.CrossServiceEdges[1], inventory.CrossServiceEdges[0]

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly passed with unstable edge sort")
	}
	if !report.UnstableEdgeSort {
		t.Fatalf("did not flag unstable edge sort; report=%#v", report)
	}
}

func TestValidateRejectsReclassifyingUnresolvedBidirectionalDebtAsAcceptable(t *testing.T) {
	inventory, packages := loadedInventoryAndPackages(t)
	first := -1
	for i := range inventory.CrossServiceEdges {
		if inventory.CrossServiceEdges[i].Bidirectional {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("expected at least one unresolved bidirectional edge")
	}
	from := inventory.CrossServiceEdges[first].FromOwner
	to := inventory.CrossServiceEdges[first].ToOwner
	for i := range inventory.CrossServiceEdges {
		edge := &inventory.CrossServiceEdges[i]
		if !((edge.FromOwner == from && edge.ToOwner == to) ||
			(edge.FromOwner == to && edge.ToOwner == from)) {
			continue
		}
		// A valid interaction class does not resolve a reciprocal dependency.
		edge.Class = ownershipinventory.EdgeClassQuery
		edge.Bidirectional = false
		edge.Unresolved = false
	}

	report := ownershipinventory.ValidateInventory(inventory, packages)
	if report.OK() {
		t.Fatal("ValidateInventory() unexpectedly accepted an unmarked reciprocal edge")
	}
	if len(report.InvalidBidirectionalEdges) == 0 {
		t.Fatalf("invalid bidirectional edges empty; report=%#v", report)
	}
}

func TestClassifyEdgeMarksProcessEdgesAsArchitectureException(t *testing.T) {
	cases := []struct {
		from string
		to   string
		want string
	}{
		{from: "root", to: ownershipinventory.DestinationEdges, want: ownershipinventory.EdgeClassConstruction},
		{from: "wire", to: ownershipinventory.DestinationEdges, want: ownershipinventory.EdgeClassConstruction},
		{from: ownershipinventory.DestinationEdges, to: "workers", want: ownershipinventory.EdgeClassExternalEffect},
		{from: ownershipinventory.DestinationEdges, to: "platform", want: ownershipinventory.EdgeClassExternalEffect},
	}
	for _, tc := range cases {
		got := ownershipinventory.ClassifyEdge(tc.from, tc.to)
		if got.Class != tc.want {
			t.Fatalf("ClassifyEdge(%q,%q).Class = %q, want %q", tc.from, tc.to, got.Class, tc.want)
		}
		if !got.ArchitectureException {
			t.Fatalf("ClassifyEdge(%q,%q) should mark architecture exception", tc.from, tc.to)
		}
	}
}

func TestAllowedEdgeClassesAreClosed(t *testing.T) {
	allowed := map[string]struct{}{}
	for _, class := range ownershipinventory.AllowedEdgeClasses {
		allowed[class] = struct{}{}
	}
	for _, want := range []string{
		ownershipinventory.EdgeClassCommand,
		ownershipinventory.EdgeClassQuery,
		ownershipinventory.EdgeClassEvent,
		ownershipinventory.EdgeClassProtocolComposition,
		ownershipinventory.EdgeClassConstruction,
		ownershipinventory.EdgeClassLifecycle,
		ownershipinventory.EdgeClassExternalEffect,
	} {
		if _, ok := allowed[want]; !ok {
			t.Fatalf("AllowedEdgeClasses missing %q", want)
		}
	}
	if len(ownershipinventory.AllowedEdgeClasses) != 7 {
		t.Fatalf("AllowedEdgeClasses = %v, want exactly 7 classes", ownershipinventory.AllowedEdgeClasses)
	}
}

func TestDiscoverCrossServiceEdgesCoversDistinctOwners(t *testing.T) {
	root := repositoryRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	edges, err := ownershipinventory.DiscoverCrossServiceEdges(root, inventory.Packages)
	if err != nil {
		t.Fatalf("DiscoverCrossServiceEdges() error = %v", err)
	}
	if len(edges) == 0 {
		t.Fatal("expected discovered cross-service edges")
	}
	seenEdges := false
	seenBidirectional := false
	for _, edge := range edges {
		if edge.FromOwner == "" || edge.ToOwner == "" {
			t.Fatalf("edge missing owners: %#v", edge)
		}
		if edge.FromOwner == edge.ToOwner {
			t.Fatalf("edge is not cross-owner: %#v", edge)
		}
		if strings.TrimSpace(edge.Class) == "" {
			t.Fatalf("edge missing class: %#v", edge)
		}
		if edge.Bidirectional {
			seenBidirectional = true
			if !edge.Unresolved {
				t.Fatalf("bidirectional edge is not visible as unresolved debt: %#v", edge)
			}
		}
		if edge.FromOwner == ownershipinventory.DestinationEdges || edge.ToOwner == ownershipinventory.DestinationEdges {
			seenEdges = true
			if !edge.ArchitectureException {
				t.Fatalf("Process Edges edge missing architectureException: %#v", edge)
			}
			if edge.Class != ownershipinventory.EdgeClassConstruction && edge.Class != ownershipinventory.EdgeClassExternalEffect {
				t.Fatalf("Process Edges edge class = %q; %#v", edge.Class, edge)
			}
		}
	}
	if !seenEdges {
		t.Fatal("expected at least one Process Edges edge in discovery")
	}
	if !seenBidirectional {
		t.Fatal("expected at least one unresolved bidirectional edge in discovery")
	}
}
