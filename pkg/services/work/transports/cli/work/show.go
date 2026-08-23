// Package work implements composition-facing facades over the Work-owned CLI adapter.
package work

import (
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type ShowConfig = workcli.ShowConfig

func NewShow(transport clihttp.Protocol) func(ShowConfig) error {
	return workcli.BindShow(transport)
}

type ListHumanApprovalsConfig = workcli.ListHumanApprovalsConfig
type ShowHumanApprovalConfig = workcli.ShowHumanApprovalConfig

func NewListHumanApprovals(transport clihttp.Protocol) func(ListHumanApprovalsConfig) error {
	return workcli.NewListHumanApprovals(transport)
}

func NewShowHumanApproval(transport clihttp.Protocol) func(ShowHumanApprovalConfig) error {
	return workcli.NewShowHumanApproval(transport)
}

func ListHumanApprovals(cfg ListHumanApprovalsConfig) error {
	return workcli.ListHumanApprovals(cfg)
}

func ShowHumanApproval(cfg ShowHumanApprovalConfig) error {
	return workcli.ShowHumanApproval(cfg)
}
