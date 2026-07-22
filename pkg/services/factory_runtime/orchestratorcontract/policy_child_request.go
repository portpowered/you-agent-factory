package orchestratorcontract

import (
	"fmt"
	"strings"
)

// ChildRequest is the policy-relevant subset of one child-agent host request.
type ChildRequest struct {
	Label           string
	Model           string
	ReasoningEffort string
	Command         string
	Sandbox         string
	WritableRoots   []string
	AllowNetwork    bool
	Concurrency     int
}

// ValidateChildRequest checks one child-agent request against effective policy
// before runtime side effects. It returns a stable diagnostic naming the denied
// policy field or capability and including safe request context.
func ValidateChildRequest(policy EffectivePolicy, req ChildRequest) error {
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
	if err := validateChildWritableRoots(policy, req); err != nil {
		return err
	}
	if err := validateChildNetwork(policy, req); err != nil {
		return err
	}
	if err := validateChildConcurrency(policy, req); err != nil {
		return err
	}
	return nil
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
	if sandbox == "workspace-write" && policy.Mode == ModeReadOnly {
		return fmt.Errorf(
			"policy denied: sandbox %q is not allowed when policy.mode is READ_ONLY (label=%q)",
			sandbox,
			safeChildLabel(req.Label),
		)
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

func validateChildWritableRoots(policy EffectivePolicy, req ChildRequest) error {
	if len(req.WritableRoots) == 0 {
		return nil
	}
	if policy.Mode == ModeReadOnly || len(policy.WritableRoots) == 0 {
		return fmt.Errorf(
			"policy denied: writableRoots are not allowed by effective policy (label=%q roots=%v)",
			safeChildLabel(req.Label),
			req.WritableRoots,
		)
	}
	for _, root := range req.WritableRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !writableRootAllowed(policy.WritableRoots, root) {
			return fmt.Errorf(
				"policy denied: writableRoot %q is not listed in policy.writableRoots (label=%q)",
				root,
				safeChildLabel(req.Label),
			)
		}
	}
	return nil
}

func validateChildNetwork(policy EffectivePolicy, req ChildRequest) error {
	if !req.AllowNetwork || policy.AllowNetwork {
		return nil
	}
	return fmt.Errorf(
		"policy denied: network access is not allowed by effective policy (label=%q)",
		safeChildLabel(req.Label),
	)
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

func writableRootAllowed(allowed []string, root string) bool {
	for _, candidate := range allowed {
		if strings.TrimSpace(candidate) == root {
			return true
		}
	}
	return false
}

func safeChildLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "-"
	}
	return label
}
