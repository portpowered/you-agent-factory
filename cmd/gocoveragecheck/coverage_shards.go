package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

func runGoTestCoverageShards(baseArgs []string, testPackages []string, jobs int, profilePath string, repoRoot string, coverPackages []string) error {
	if jobs < 1 {
		jobs = 1
	}
	if jobs > len(testPackages) {
		jobs = len(testPackages)
	}
	shardDir, err := os.MkdirTemp("", "go-coverage-shards-*")
	if err != nil {
		return fmt.Errorf("create go coverage shard directory: %w", err)
	}
	defer os.RemoveAll(shardDir)

	testShards := make([][]string, jobs)
	for index, testPackage := range testPackages {
		testShards[index%jobs] = append(testShards[index%jobs], testPackage)
	}
	coverPackageShards := [][]string{coverPackages}
	if runtime.GOOS == "windows" {
		coverPackageShards = partitionCoveragePackages(coverPackages, windowsCoveragePackageArgumentLimit)
	}
	totalShards := len(testShards) * len(coverPackageShards)
	profiles := make([]string, totalShards)
	errs := make([]error, totalShards)
	var wait sync.WaitGroup
	for coverIndex, coverShard := range coverPackageShards {
		for testIndex, packages := range testShards {
			index := coverIndex*len(testShards) + testIndex
			profiles[index] = filepath.Join(shardDir, fmt.Sprintf("shard-%d.out", index+1))
			args := replaceCoveragePackageArgument(baseArgs, coverShard)
			args = append(args, fmt.Sprintf("-coverprofile=%s", profiles[index]))
			args = append(args, packages...)
			wait.Add(1)
			go func(index int, args []string, selectedCoverPackages []string) {
				defer wait.Done()
				_, _, errs[index] = runGoTestCoverageLane(args, fmt.Sprintf("run go test coverage shard %d/%d", index+1, totalShards))
				if errs[index] == nil && len(coverPackageShards) > 1 {
					errs[index] = canonicalizeCoverageProfile(profiles[index], repoRoot, selectedCoverPackages)
				}
			}(index, args, slices.Clone(coverShard))
		}
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return mergeCoverageProfiles(profiles, profilePath, repoRoot, coverPackages)
}

func partitionCoveragePackages(packages []string, argumentLimit int) [][]string {
	var shards [][]string
	var shard []string
	shardLength := len("-coverpkg=")
	for _, importPath := range packages {
		addedLength := len(importPath)
		if len(shard) > 0 {
			addedLength++
		}
		if len(shard) > 0 && shardLength+addedLength > argumentLimit {
			shards = append(shards, shard)
			shard = nil
			shardLength = len("-coverpkg=")
		}
		shard = append(shard, importPath)
		shardLength += addedLength
	}
	if len(shard) > 0 {
		shards = append(shards, shard)
	}
	return shards
}

func replaceCoveragePackageArgument(args []string, packages []string) []string {
	replaced := slices.Clone(args)
	for index, arg := range replaced {
		if strings.HasPrefix(arg, "-coverpkg=") {
			replaced[index] = "-coverpkg=" + strings.Join(packages, ",")
			return replaced
		}
	}
	return append(replaced, "-coverpkg="+strings.Join(packages, ","))
}
