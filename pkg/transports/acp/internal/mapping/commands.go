package mapping

import acpsdk "github.com/coder/acp-go-sdk"

// FactoryCommandName is the one prompt command this transport implements.
// Advertising is derived from it rather than duplicating the literal, so the
// advertised set cannot drift from what the prompt parser actually accepts.
const FactoryCommandName = "factory"

// ProjectAvailableCommands returns the available-commands advertisement for a
// session.
//
// It advertises exactly the commands the prompt parser implements -- today,
// only /factory. Advertising anything a client could type but the server would
// then reject would be worse than advertising nothing, so this list is
// deliberately derived from the parser's own constant rather than from a
// wish list.
func ProjectAvailableCommands() acpsdk.SessionUpdate {
	return acpsdk.SessionUpdate{
		AvailableCommandsUpdate: &acpsdk.SessionAvailableCommandsUpdate{
			AvailableCommands: []acpsdk.AvailableCommand{{
				Name: FactoryCommandName,
				Description: "Switch this session to another installed Factory, " +
					"for example: /factory factory:@you/plan-parallel",
				Input: &acpsdk.AvailableCommandInput{
					Unstructured: &acpsdk.UnstructuredCommandInput{
						Hint: "factory:<name>",
					},
				},
			}},
		},
	}
}
