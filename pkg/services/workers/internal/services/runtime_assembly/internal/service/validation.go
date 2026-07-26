package service

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func snapshotAndValidate(
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildRequest, error) {
	if strings.TrimSpace(request.RunnerID) == "" {
		return workers.RuntimeBuildRequest{}, workers.ErrMissingRunnerSelection
	}
	if request.RunnerID != strings.TrimSpace(request.RunnerID) {
		return workers.RuntimeBuildRequest{}, fmt.Errorf(
			"%w: runner selection must not have surrounding whitespace",
			workers.ErrInvalidRuntimeBuildRequest,
		)
	}
	if len(request.Roles) == 0 {
		return workers.RuntimeBuildRequest{}, fmt.Errorf(
			"%w: at least one role is required",
			workers.ErrInvalidRuntimeBuildRequest,
		)
	}

	snapshot := request
	snapshot.Opening = cloneOpeningOptions(request.Opening)
	snapshot.Roles = append([]workers.RuntimeBuildRoleRequest(nil), request.Roles...)
	if snapshot.Opening.MockWorkers != nil {
		if err := snapshot.Opening.MockWorkers.Validate(); err != nil {
			return workers.RuntimeBuildRequest{}, fmt.Errorf(
				"%w: opening mock workers: %w",
				workers.ErrInvalidRuntimeBuildRequest,
				err,
			)
		}
	}
	if err := validateRoles(snapshot.Roles); err != nil {
		return workers.RuntimeBuildRequest{}, err
	}
	return snapshot, nil
}

func validateRoles(roles []workers.RuntimeBuildRoleRequest) error {
	kindsByName := make(map[string]workers.RuntimeBuildRoleKind, len(roles))
	for index, role := range roles {
		if strings.TrimSpace(role.Name) == "" ||
			role.Name != strings.TrimSpace(role.Name) {
			return fmt.Errorf(
				"%w: role %d has a malformed name",
				workers.ErrInvalidRuntimeBuildRequest,
				index,
			)
		}
		switch role.Kind {
		case workers.RuntimeBuildRoleKindWorker, workers.RuntimeBuildRoleKindWorkstation:
		default:
			return fmt.Errorf(
				"%w: role %q has unknown kind %q",
				workers.ErrInvalidRuntimeBuildRequest,
				role.Name,
				role.Kind,
			)
		}
		if existing, found := kindsByName[role.Name]; found {
			detail := "duplicate"
			if existing != role.Kind {
				detail = "conflicting"
			}
			return fmt.Errorf(
				"%w: %s role %q",
				workers.ErrInvalidRuntimeBuildRequest,
				detail,
				role.Name,
			)
		}
		kindsByName[role.Name] = role.Kind
	}
	return nil
}
