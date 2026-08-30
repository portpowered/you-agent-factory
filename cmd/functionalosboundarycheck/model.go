package main

import (
	"fmt"
	"slices"
	"strings"
)

const (
	inventoryFormatVersion = 3
	baselineFormatVersion  = 1
	spawnCountUnit         = "static-os-spawn-site"
	intentionalVerdict     = "INTENTIONAL-OS"
	accidentalVerdict      = "ACCIDENTAL-OS"
)

var allowedProperties = []string{
	"exit-status",
	"signal-delivery",
	"stream-file-descriptor-behavior",
	"pty",
	"crash-isolation",
	"descendant-cleanup",
	"executable-selection",
}

type baselineDocument struct {
	Version   int                  `json:"version"`
	CountUnit string               `json:"countUnit"`
	Packages  []baselinePackageRow `json:"packages"`
}

type baselinePackageRow struct {
	PackagePath string `json:"packagePath"`
	Count       int    `json:"count"`
}

type inventoryDocument struct {
	FormatVersion int                  `json:"formatVersion"`
	TestRows      []inventoryTestRow   `json:"testRows"`
	OSSpawnSites  []inventorySpawnSite `json:"osSpawnSites"`
}

type inventoryTestRow struct {
	Classification           string                `json:"classification"`
	OSBoundaryIntentionality *intentionalityRecord `json:"osBoundaryIntentionality"`
}

type intentionalityRecord struct {
	Verdict              string   `json:"verdict"`
	RequiredProperty     *string  `json:"requiredProperty"`
	AssertionEvidence    string   `json:"assertionEvidence"`
	ConversionObligation *string  `json:"conversionObligation"`
	SpawnSiteIDs         []string `json:"spawnSiteIds"`
}

type inventorySpawnSite struct {
	SiteID               string  `json:"siteId"`
	PackagePath          string  `json:"packagePath"`
	SourcePath           string  `json:"sourcePath"`
	SourceLine           int     `json:"sourceLine"`
	EnclosingIdentity    string  `json:"enclosingIdentity"`
	LauncherKind         string  `json:"launcherKind"`
	Occurrence           int     `json:"occurrence"`
	Verdict              string  `json:"verdict"`
	RequiredProperty     *string `json:"requiredProperty"`
	AssertionEvidence    string  `json:"assertionEvidence"`
	ConversionObligation *string `json:"conversionObligation"`
}

type spawnSite struct {
	SiteID            string
	PackagePath       string
	SourcePath        string
	SourceLine        int
	EnclosingIdentity string
	Occurrence        int
}

type violation struct {
	message string
}

func packageCounts(sites []spawnSite) map[string]int {
	counts := map[string]int{}
	for _, site := range sites {
		counts[site.PackagePath]++
	}
	return counts
}

func intentionalCounts(inventory inventoryDocument) map[string]int {
	counts := map[string]int{}
	for _, site := range inventory.OSSpawnSites {
		if site.Verdict == intentionalVerdict {
			counts[site.PackagePath]++
		}
	}
	return counts
}

func isAllowedProperty(value string) bool {
	return slices.Contains(allowedProperties, value)
}

func allowedPropertySummary() string {
	return strings.Join(allowedProperties, ", ")
}

func validateIntentionality(record *intentionalityRecord, label string) []string {
	if record == nil {
		return []string{fmt.Sprintf("%s is missing osBoundaryIntentionality", label)}
	}
	return validateVerdictFields(record.Verdict, record.RequiredProperty, record.AssertionEvidence, record.ConversionObligation, label)
}

func validateVerdictFields(verdict string, property *string, evidence string, obligation *string, label string) []string {
	var findings []string
	switch verdict {
	case intentionalVerdict:
		if property == nil || strings.TrimSpace(*property) == "" || !isAllowedProperty(*property) {
			findings = append(findings, fmt.Sprintf("%s intentional verdict must name one allowed OS property (%s)", label, allowedPropertySummary()))
		}
		if obligation != nil {
			findings = append(findings, fmt.Sprintf("%s intentional verdict must have conversionObligation=null", label))
		}
	case accidentalVerdict:
		if property != nil {
			findings = append(findings, fmt.Sprintf("%s accidental verdict must have requiredProperty=null", label))
		}
		if obligation == nil || strings.TrimSpace(*obligation) == "" {
			findings = append(findings, fmt.Sprintf("%s accidental verdict must name a conversionObligation", label))
		}
	default:
		findings = append(findings, fmt.Sprintf("%s has unknown verdict %q", label, verdict))
	}
	if strings.TrimSpace(evidence) == "" {
		findings = append(findings, fmt.Sprintf("%s must name assertionEvidence", label))
	}
	return findings
}

func sortViolations(findings []violation) {
	slices.SortFunc(findings, func(left, right violation) int {
		return strings.Compare(left.message, right.message)
	})
}
