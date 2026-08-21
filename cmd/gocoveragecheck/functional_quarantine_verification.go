package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// functionalQuarantineSelectorVerification overlaps the retained runtime list
// with the other coverage-package setup work. Its result is still awaited
// after static inventory validation, so stale static selectors keep their
// existing diagnostics and runtime failures remain fail-closed.
type functionalQuarantineSelectorVerification struct {
	done          chan error
	started       time.Time
	selectorCount int
	once          sync.Once
	err           error
}

// Selector verification only compiles the packages named by test-level
// selectors and does not run instrumented tests, so it can use a small bounded
// build-only concurrency allowance without changing the coverage lane's jobs.
const maxFunctionalQuarantineVerificationJobs = 8

func functionalQuarantineVerificationJobs(jobs int) int {
	jobs = maxFunctionalDiscoveryJobs(jobs)
	if jobs >= maxFunctionalQuarantineVerificationJobs/2 {
		return maxFunctionalQuarantineVerificationJobs
	}
	return jobs * 2
}

func startFunctionalQuarantineSelectorVerification(cfg config, targetOS string, logicalCPUs int, repoRoot string) *functionalQuarantineSelectorVerification {
	if strings.TrimSpace(cfg.functionalQuarantine) == "" {
		return nil
	}

	path := functionalQuarantinePath(cfg, repoRoot)
	manifest, err := readFunctionalQuarantineFile(path)
	if err != nil {
		verification := newFunctionalQuarantineSelectorVerification(0)
		verification.done <- err
		return verification
	}
	selectorPackages := functionalTestSelectorPackages(manifest)
	if len(selectorPackages) == 0 {
		return nil
	}

	verification := newFunctionalQuarantineSelectorVerification(len(selectorPackages))
	fmt.Fprintf(stdoutWriter, "Functional quarantine selector verification: begin selectors=%d\n", verification.selectorCount)
	go func() {
		verification.done <- verifyFunctionalTestQuarantineSelectors(
			manifest,
			cfg.timeout,
			cfg.short,
			functionalQuarantineVerificationJobs(cfg.testJobs(targetOS, logicalCPUs)),
			repoRoot,
		)
	}()
	return verification
}

func functionalQuarantinePath(cfg config, repoRoot string) string {
	path := cfg.functionalQuarantine
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return path
}

func newFunctionalQuarantineSelectorVerification(selectorCount int) *functionalQuarantineSelectorVerification {
	return &functionalQuarantineSelectorVerification{
		done:          make(chan error, 1),
		started:       time.Now(),
		selectorCount: selectorCount,
	}
}

func (verification *functionalQuarantineSelectorVerification) wait() error {
	if verification == nil {
		return nil
	}
	verification.once.Do(func() {
		verification.err = <-verification.done
		status := "complete"
		if verification.err != nil {
			status = "failed"
		}
		fmt.Fprintf(
			stdoutWriter,
			"Functional quarantine selector verification: end status=%s elapsed=%.3fs selectors=%d\n",
			status,
			time.Since(verification.started).Seconds(),
			verification.selectorCount,
		)
	})
	return verification.err
}
