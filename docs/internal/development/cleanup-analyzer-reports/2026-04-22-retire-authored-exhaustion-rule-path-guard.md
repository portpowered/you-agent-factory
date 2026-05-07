# Cleanup Analyzer Report: Retired Authored Exhaustion-Rule Guard

Date: 2026-04-22

## Scope

Agent Factory cleanup evidence for retiring the remaining internal
customer-authored exhaustion-rule path. The supported authored loop-breaker
contract is a guarded `LOGICAL_MOVE` workstation with a `visit_count` guard.
This report records the active production-code inventory after the shared
config, mapper, validator, and flattening cleanup and names the only approved
`TransitionExhaustion` runtime sites that remain.

Historical cleanup reports under
`libraries/agent-factory/docs/development/cleanup-analyzer-reports/` are
excluded from active-symbol conclusions because the reports intentionally
preserve earlier analyzer output.

## Analyzer Commands

Retired authored-identifier inventory:

```powershell
rg -n "\bExhaustionRules\b|\bExhaustionRuleConfig\b" libraries/agent-factory/pkg -g "*.go" -g "!**/*_test.go"
```

Approved `TransitionExhaustion` production inventory:

```powershell
rg -n "TransitionExhaustion" libraries/agent-factory/pkg -g "*.go" -g "!**/*_test.go"
```

Retained raw boundary-rejection check:

```powershell
rg -n "exhaustion_rules|exhaustionRules" libraries/agent-factory/pkg/config/factory_config_mapping.go
```

## Final Inventory

The retired authored identifiers are absent from active production `pkg` code.

| Symbol | Matches | Files | Classification |
| --- | ---: | ---: | --- |
| `ExhaustionRules` | 0 | 0 | Removed from active production identifiers. |
| `ExhaustionRuleConfig` | 0 | 0 | Removed from active production identifiers. |

`TransitionExhaustion` remains in exactly three active production files:

| File | Matches | Classification |
| --- | ---: | --- |
| `pkg/config/config_mapper.go` | 1 | Approved system-owned time-expiry transition in `addDefaultTimeExpiryTransition`. |
| `pkg/factory/subsystems/circuitbreaker.go` | 1 | Approved circuit-breaker evaluation of exhaustion transitions. |
| `pkg/petri/transition.go` | 2 | Approved transition-type documentation and enum declaration. |

The raw boundary-rejection check remains intentionally in
`pkg/config/factory_config_mapping.go` and is limited to rejecting
`exhaustionRules` / `exhaustion_rules` input with migration guidance. It is not
an authored config shape, mapper expansion path, or internal compatibility
representation.

## Guard Coverage

`pkg/config/exhaustion_rule_contract_guard_test.go` enforces the production
inventory directly:

- it fails if any production `pkg` Go file reintroduces the exact retired
  authored identifiers `ExhaustionRules` or `ExhaustionRuleConfig`;
- it fails if `petri.TransitionExhaustion` appears outside the approved system
  time-expiry mapper path, circuit-breaker subsystem, or Petri enum
  declaration; and
- it intentionally excludes generated code so the guard stays focused on
  maintained production packages.

## Validation Commands

Commands were run from `libraries/agent-factory` unless noted.

```bash
go test ./pkg/config -count=1
go test ./pkg/config ./pkg/factory/subsystems ./pkg/petri -count=1
make lint
```
