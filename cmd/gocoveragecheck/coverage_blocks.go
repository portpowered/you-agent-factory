package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const maxUncoveredCoverageBlocks = 25

type coverageBlock struct {
	canonicalPath  string
	importPath     string
	rangeSpec      string
	statementCount int
	executionCount int
}

func uncoveredCoverageBlocks(coverageBlocks map[string]coverageBlock, importPath string) []coverageBlock {
	blocks := make([]coverageBlock, 0)
	for _, block := range coverageBlocks {
		if block.importPath != importPath || block.executionCount != 0 {
			continue
		}
		blocks = append(blocks, block)
	}
	slices.SortFunc(blocks, compareCoverageBlocks)
	return blocks
}

func compareCoverageBlocks(left coverageBlock, right coverageBlock) int {
	if comparison := strings.Compare(left.canonicalPath, right.canonicalPath); comparison != 0 {
		return comparison
	}
	leftLine, leftColumn := coverageBlockStartPosition(left)
	rightLine, rightColumn := coverageBlockStartPosition(right)
	if leftLine != rightLine {
		return leftLine - rightLine
	}
	if leftColumn != rightColumn {
		return leftColumn - rightColumn
	}
	return strings.Compare(left.rangeSpec, right.rangeSpec)
}

func coverageBlockStartPosition(block coverageBlock) (int, int) {
	start := block.rangeSpec
	if separator := strings.IndexByte(start, ','); separator >= 0 {
		start = start[:separator]
	}
	separator := strings.IndexByte(start, '.')
	if separator < 0 {
		line, _ := strconv.Atoi(start)
		return line, 0
	}
	line, _ := strconv.Atoi(start[:separator])
	column, _ := strconv.Atoi(start[separator+1:])
	return line, column
}

func formatUncoveredCoverageBlocks(coverageBlocks map[string]coverageBlock, importPath string) string {
	blocks := uncoveredCoverageBlocks(coverageBlocks, importPath)
	if len(blocks) == 0 {
		return ""
	}
	limit := min(len(blocks), maxUncoveredCoverageBlocks)
	entries := make([]string, 0, limit)
	for _, block := range blocks[:limit] {
		filePath := strings.TrimPrefix(block.canonicalPath, modulePath+"/")
		statementLabel := "statements"
		if block.statementCount == 1 {
			statementLabel = "statement"
		}
		entries = append(entries, fmt.Sprintf("%s:%d (%d %s)", filePath, coverageBlockStartLine(block), block.statementCount, statementLabel))
	}
	detail := "uncovered blocks: " + strings.Join(entries, ", ")
	if omitted := len(blocks) - limit; omitted > 0 {
		detail += fmt.Sprintf(", ... and %d more", omitted)
	}
	return detail
}

func coverageBlockStartLine(block coverageBlock) int {
	line, _ := coverageBlockStartPosition(block)
	return line
}
