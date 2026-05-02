# Automat Portability Smoke Fixture

This fixture models one bounded `translate/automat` portability slice for Agent
Factory smoke coverage.

It proves a realistic multi-file layout with:

- bundled PowerShell scripts under `scripts/`
- bundled workflow notes under `docs/`
- a root dependency contract file at `portable-dependencies.json`
- restored expanded-layout paths at those same factory-relative locations

The fixture intentionally stops at dispatch readiness. It does not attempt full
translation, OCR, or image processing output verification.

The canonical portability-contract explanation lives in
`libraries/agent-factory/docs/workstations.md#automat-inspired-portability-smoke`.
Keep this README focused on the local fixture shape and workflow slice.

## Intentionally External Tools

The portable bundle does not ship `mangaka.exe` or `magick`. Those tools stay
external and are declared in `factory.json` `supportingFiles.requiredTools`.
The bundled `portable-dependencies.json` mirrors that contract for the bounded
fixture scripts.

## Workflow Slice

1. `prepare-automat-slice` stages the bounded runtime layout and verifies the
   bundled workflow guide and dependency contract are present.
2. `check-tool-contract` reads the dependency contract and surfaces the
   required external tools without trying to bundle or install them.

## Representative Restored Layout

- `scripts/prepare-automat-slice.ps1`
- `scripts/verify-external-tools.ps1`
- `docs/portable-workflow.md`
- `portable-dependencies.json`
