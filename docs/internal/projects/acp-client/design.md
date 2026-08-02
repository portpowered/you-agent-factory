# problem statement

Using YOU is useful in the context of setting up a new entire package, but its kind of a pain in the ass when you want to use it for one off operations.

# recommended changes

To make it more useful for one offs, we recommend converting the YOU interface to a chat interface. What this means is that for customers who already have a chat interface with ACP support, they get YOU embedded directly in their program. For example:
1. obsidian
2. vscode
3. intellij
4. zed
5. neovim
6. unity
7. agent studio
8. telegram, wechat
9. aionui
10. codeg
11. deepchat

generally, we only do P0s.
# customer experience

customers use the ACP interface integrations against YOU. You abstracts the existing factory session and the correspondign provider session itno a new abstraction called a chat session.

customers from their IDE/ADE/whatever else integrates against the chat session. The new chat session is then initiated. The default experience is basically returns direct text describing how to make a factory and runs for a specific provdier if they want it.

We should add a nwe packaged factory as well that is like "@you/factory-builder" that is ingrained with the knowledge of how to invoke the you CLI for documentation. We should audit the YOU docs agents to make sure that it includes basically all the information needed for a factory builder to build and vlaidate a config whethere is a jascript or a graph basd factory and store it in the customer's home folder.

First thing the customer is they open their agents field.
When the ACP connection is initialized you agent factory is served internally via `you acp serve`.

ACP initializes, and YOU publishes the models/configurations for the session as whatever factory packages that we have installed via the factory configuration service's list factory operations.


Then a customer will set the model to their packaged factory or whatever from:

i.e. /@you/review

Then they submit a prompt, and the prompt is consumed in the chat service. The prompt is then transformed into the corresponding default invocation in the factory configuration for the session (wherever appropriate, i believe this is current factory session/factory runtime), and the system acks the request. The system then responds back via stdio a series of event updates.
- the chat service is also responsible for submitting the work to the work service for continual prompts.
- we stream the factory events as they appear, mapping them to appropriate text prompt formats and tol calls.
- factory workstation dispatches should be treated as separate tool calls, each with their own respective content event streams mapping the content events from each requested tool.
- the list of active work items should be enumerated as a plan.
- as the list of active items changed it should be updated as appropriate in the plan events.

- we should ensure that when we dispatch things, that we use very nice interfaces. i.e. we should put emojis/image and stuff next ot the tools marking the varous functions/icons if possible to denote a claude work dipsatch is a worker and a codex dispatch is x, etc.

# introduced systems changes
The main system changes that are involved with you are:
1. an introduction of a new transport called the ACP transport and exposure
2. an introduction of worker sesssions, chat, and the corresponding events journal services.
3. a series of service hooks in the runtime factory session to publish to the events journal as a continuity mechanism and publication mechanism.
4. corresponding integrations between workers providers to introduce service interruption/termination.
5. integrations between the CLI and the new ACP handler.

We look to conform to the general structural standards set forth in our architecture.

Each service defines a unary interface. Customers consume that unary interface directly, not via internal contents.

# itnerface changes
- CLi updates for new ACp serve
- event updates so that there is a correlation mechanism between the dispatch reuests and the dispatch responses


# services introduced

## worker session service

worker session service is roughly worker service, but with mechanisms for enumerating active worker executions that are currently in the runtime as well as the ability to perform control operations on the worker sessions. It can technically just be workers service if the workers service is small enough right now.

### interface TODO

type Service interface {
    Create(ctx, request) (response, error) // creates a session
    Resume(ctx, request) (response, error) // resumes an existing worker session, with a new initiation prompt
    Stop(ctx, request) (response, error) // stops currently active worker session.
    List(ctx, request) (response, error) // list current active sessions
}

type WorkerSessions truct {

    id string
    type string // enum of well known worker session types
    externalProviderId *string// the Id that the provider gives for us to use to resume a session if it exists, optional support
    eternalProvider *string // The id of the provider if it exists

    startTime
}
## events service

the events service is a journaled event log for worker (provider response events / worker events), and the corresponding events.

it exposes the interface to allow customers to subscribe for events and to wire up a registered event hook as well as to publish specific events.

The subscription registratio is done during initial service wiring, not during initialization.

### Interface TODO

type Service interface {
    Subscribe(ctx, request) (response, error)

}

Request {
    targetSubscriptionHookFunc
    topicIdentifier(sessionId?) <- optional, if none matches all such events
    messageTypeIdentifiers(x/y/z) <- optional, if none matches all such events. OR of message types.
}

Response {
    ok
}

## chat service

Teh chat service is responsible for transforming the internal business logic formations of the factory session, the worker sessions, and transforming them into a format that is amenable to the various chat protocols.

i.e. for the ACP protocol for a given factory session:

the chat service consumes the worker session event stream, and the provider session event stream.
teh chat service then transforms said event streams into something more amenable for usage.

### Interface TODO

type Service interface {
    InitializeConnection()
    NewSession()
    LoadSession(ctx Context, LoadRequest) (Response, error)
    PromptSession(etc,)
    CancelSession(etc)
    DeleteSession()
    ListSessions()
}

### data modle

a chat session is roughly mapping to a factory session + the individual worker sessions as tool calls.

a SessionResponse in ACP from new session would enumerate a series of options like:

1. models -> mapping to factory names
2. default provider -> sets the default provider in case of no configurations
3. default model -> sets the default model in case of no models active

### slash commands

the caht service intercepts the following slash commands by default:

/config:
- which exposes the current system information and the curernt factory information like what is running and what is possible

/inputs:
- which shows the current set of parameterized inputs.

/model $0:
- which sets the current model for the current target session execution

/model-provider $0:
- which sets the model provider current dispatch

/worktree $0:
- which sets the model provider worktree

# package changes

pkg/transports/acp
- this package contains the new STDIO acp handler
- wire to the chat service for integrations

pkg/services/chat
- exposes the chat logical interface.
- implement the chat service
- wire to the application, and various work submission interfaces.
- this subcribes to the events service during wiring consturction such that session events and worker events are pushed to some function on the chat service instance.

pkg/services/events
- exopses subscriptions
- the workers serivce needs to be pushing events to the events service in a consistently ordered way
- need to ensure we have persisted event streams
- needs to be wired to the appropriate subscriber services.

pkg/services/factory-definitions
- we should ensure the primary factory definition interface has the appropriate factory enumeration and factory loading APIs to support chat's operations. We shoudl not use the non root packages as those are tech debt. Similar for other service definitions.

pkg/services/workersessions
- we should wire this appropriately to the workers API or extend the workers API for session enumeration, resumption, cancellation, and creation/deletion.

# vertical slice implementations to test e2e

when we're implementing we want to iterate via veritcal slices to not implement in too much chaos, so we target implementing things in separate segments when possible.

## ACP Factory Session Slices
- make acp work with current factory sessions and new cahat service (without plans/updates, just fire and forget), with a default model set to the new basic informational chat factory.
-- test switching to a more complex session works
-- test multi session submissions works
- make acp work with current factory sessions and ensure that we are able to enumerate the factory models
- make acp work with current factory sessions as well as plan updates,
- make acp work with current factory sessions with tool calls (without deltas)

## WOrker event slices
- make the event streams from teh CLI or the API (need to pick) work for the worker sessions for a given session. ensure that we have a mechanism for enumerating those sessions.
- have the workers test out all the various functions.

## Aggregate Slices (both worker events and afctory sessions are needed)

- make acp worker with current factory sessions with tool calls (with deltas)
# test changes

## functional tests
tests/functional/acp
    - validate that we are able to initiate with a real ACP test client
    - validate that we are able to enumerate sessions
    - validate that we are able to create a sesion
    - validate that we are able to terminate a session
    - validate that within a session we are sreaming the active work items as plan updates
    - validate that within a session we are streaming the dispatches as tool calls
    - validate that we are able to udpate teh dispatches for tool calls as streaming context updates as tool update messages
    - validate that we present new dispatches as additional tool calls
    - validat ethat we are within a session enumerating that we are thinkign the entire time
    - validate that the final output of a factory session is responded as part of the stdout
    - session pause
    (p1): session resume
    (p1): session pause
tests/functional/worker-session
as part of testing worker sessions as an independent servic ecomponent, we should expose as part of the CLI or the API a event stream that we can parse that represents the worker sessions.
    - test streaming works for ACP agents
    - test streaming works for claude
    - test streaming works for cursor
    - test it handles partial failure on cursor/claude/ACP
    - test that it handles partial failure for workers

## e2e tests
(AI will do)
- test that acpx works for a basic test case
- test that acpx works for session termination
- test that acpx plan updates owrk
- test that acpx worker session tools are enumeration
- test that acpx model enumeration works for packaged factories
- test that acpx worker sessions are able to be streamed as too lresponses
- tes that sending second prompt to a session will generate a new work item, not resume an existing item.

(Human will do)
- test integration works with zed
- test integraiton works with neovim
- test integration works with obsidian
# dangerous edge changes

# appendix

## alternative approaches

### 1:- use only packaged installation of UI

In this proposal, we install our own UI layer and have customers use it.

The general reason we don't do this is because its a pain to manage and everyone has their own use cases and flows.
Having gone through testing other people's factory engines, they generally are truly a pain to work with as I don't want to learn another person's UX, i just want to run vscode, zed, obsidian, intellij or whatever, and not deal with it.

While we have more control and flexibility, we make a tradeoff on breadth. We aren't going to write custom integrations to obsidian, slack, whatsapp, and a whole slew of other integration platforms.

### 2:- use CLI only

the functionality works e2e already on the CLI, its more of an operator use case at this point.

### 3: - model the session as a function / slash command that can then be invoked via a primary client.

This works as well, but necessarily requires that all invocations go via an agent as the primary orchestrator, which is kind of expensive and non intuitive.