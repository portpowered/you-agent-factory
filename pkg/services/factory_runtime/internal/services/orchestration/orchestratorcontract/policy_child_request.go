package orchestratorcontract

import (
	"fmt"
	"strings"
)

// ChildRequest is the policy-relevant subset of one child-agent host request.
type ChildRequest struct {
	FactoryName     string
	Label           string
	Model           string
	ReasoningEffort string
	Command         string
	Sandbox         string
	// SkipPermissions is a child-scoped provider permission override. It may
	// bypass provider sandbox restrictions for this child, but it does not
	// weaken routing, permission, or resource policy.
	SkipPermissions bool
	Concurrency     int
}

// ValidateChildRequest checks one child-agent request against effective policy
// before runtime side effects. It returns a stable diagnostic naming the denied
// policy field or capability and including safe request context.
func ValidateChildRequest(policy EffectivePolicy, req ChildRequest) error {
	if err := validateChildPermission(policy, req); err != nil {
		return err
	}
	if err := validateChildModel(policy, req); err != nil {
		return err
	}
	if err := validateChildReasoningEffort(policy, req); err != nil {
		return err
	}
	if err := validateChildCommand(policy, req); err != nil {
		return err
	}
	if err := validateChildSandbox(policy, req); err != nil {
		return err
	}
	if err := validateChildConcurrency(policy, req); err != nil {
		return err
	}
	return nil
}

func validateChildPermission(policy EffectivePolicy, req ChildRequest) error {
	if len(policy.AllowedPermissions) == 0 {
		return nil
	}

	requested := PermissionModeDefault
	if req.SkipPermissions {
		requested = PermissionModeSkipPermissions
	}
	for _, allowed := range policy.AllowedPermissions {
		if strings.TrimSpace(allowed) == requested {
			return nil
		}
	}
	return fmt.Errorf(
		"policy denied: Factory %q child %q requested permission %q not listed in allowedPermissions",
		safeFactoryName(req.FactoryName),
		safeChildLabel(req.Label),
		requested,
	)
}

func validateChildModel(policy EffectivePolicy, req ChildRequest) error {
	model := strings.TrimSpace(req.Model)
	if model == "" || len(policy.AllowedModels) == 0 {
		return nil
	}
	for _, allowed := range policy.AllowedModels {
		if strings.TrimSpace(allowed) == model {
			return nil
		}
	}
	return fmt.Errorf(
		"policy denied: model %q is not listed in allowedModels (label=%q)",
		model,
		safeChildLabel(req.Label),
	)
}

func validateChildReasoningEffort(policy EffectivePolicy, req ChildRequest) error {
	effort := strings.ToLower(strings.TrimSpace(req.ReasoningEffort))
	if effort == "" || len(policy.AllowedReasoningEfforts) == 0 {
		return nil
	}
	for _, allowed := range policy.AllowedReasoningEfforts {
		if strings.ToLower(strings.TrimSpace(allowed)) == effort {
			return nil
		}
	}
	return fmt.Errorf(
		"policy denied: reasoningEffort %q is not listed in allowedReasoningEfforts (label=%q)",
		req.ReasoningEffort,
		safeChildLabel(req.Label),
	)
}

func validateChildCommand(policy EffectivePolicy, req ChildRequest) error {
	command := strings.TrimSpace(req.Command)
	if command == "" || len(policy.AllowedCommands) == 0 {
		return nil
	}
	for _, allowed := range policy.AllowedCommands {
		if strings.TrimSpace(allowed) == command {
			return nil
		}
	}
	return fmt.Errorf(
		"policy denied: command %q is not listed in allowedCommands (label=%q)",
		command,
		safeChildLabel(req.Label),
	)
}

func validateChildSandbox(policy EffectivePolicy, req ChildRequest) error {
	sandbox := strings.TrimSpace(req.Sandbox)
	if sandbox == "" {
		return nil
	}
	if _, ok := knownSandboxModes[sandbox]; !ok {
		return fmt.Errorf(
			"policy denied: sandbox %q is unsupported (label=%q)",
			sandbox,
			safeChildLabel(req.Label),
		)
	}
	if req.SkipPermissions {
		return nil
	}
	if policySandbox := strings.TrimSpace(policy.SandboxMode); policySandbox != "" && sandbox != policySandbox {
		return fmt.Errorf(
			"policy denied: sandbox %q does not match effective sandboxMode %q (label=%q)",
			sandbox,
			policySandbox,
			safeChildLabel(req.Label),
		)
	}
	return nil
}

func validateChildConcurrency(policy EffectivePolicy, req ChildRequest) error {
	if req.Concurrency <= 0 {
		return nil
	}
	if req.Concurrency <= policy.Concurrency {
		return nil
	}
	return fmt.Errorf(
		"policy denied: requested concurrency %d exceeds policy concurrency %d (label=%q)",
		req.Concurrency,
		policy.Concurrency,
		safeChildLabel(req.Label),
	)
}

func safeChildLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "-"
	}
	return label
}

func safeFactoryName(name string) string {
	return safeChildLabel(name)
}
