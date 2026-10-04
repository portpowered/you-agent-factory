// Package service implements the parent-private Agent Client Protocol service.
// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/internal/service/cancelwindow"
	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"
)

// Command is one configured stdio ACP launch command.
type Command struct {
	Name string
	Args []string
}

// Service owns the configured ACP integrations and every live request-owned
// ACP attempt.
//
// Every Execute and Continue allocates one attempt that owns its own process,
// connection, client, standard streams, stderr capture, cancel window and
// lifecycle, and that attempt is torn down on every terminal path. Two attempts
// against the same canonical provider therefore share no mutable protocol state
// and never serialize on one another, and each attempt's own working directory
// and environment reach only its own process startup.
//
// The service lock guards exactly the configured integrations, their aliases,
// and the provider registry. It is never held across process or protocol I/O.
type Service struct {
	mu           sync.RWMutex
	integrations map[providers.ID]providers.ACPIntegration
	aliases      map[string]providers.ID
	providers    map[providers.ID]*provider
	newCommand   platformprocess.CommandFactory
	locator      platformprocess.ExecutableLocator
	stdioPipes   platformprocess.StdioPipeFactory
}

var _ acp.ContinuationService = (*Service)(nil)

// New constructs the ACP continuation owner. stdioPipes is the exact
// parent-owned standard-stream channel factory this service is allowed to use;
// it is never selected here, so a composition that omits it produces a clear
// dependency failure on the first execution rather than a service that quietly
// opens a host pipe for itself.
func New(
	integrations []providers.ACPIntegration,
	newCommand platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
	stdioPipes platformprocess.StdioPipeFactory,
) (acp.ContinuationService, error) {
	service := &Service{
		providers:  map[providers.ID]*provider{},
		newCommand: newCommand,
		locator:    locator,
		stdioPipes: stdioPipes,
	}
	if err := service.Configure(context.Background(), integrations); err != nil {
		return nil, err
	}
	return service, nil
}

// beginAttempt resolves id (including an accepted alias) to its canonical
// identity and registers one request-owned attempt against the provider owning
// that integration in a single read-locked step.
//
// Resolving and registering must not be separable: a Close or Configure that
// retired the provider between two independent operations would replace the
// registry entry the second one resolves, so the attempt it registers would
// spawn a process that nothing owns. Holding the read lock across both steps
// makes a concurrent Close or Configure wait until this attempt is registered
// against the provider it then retires, so a retired provider can never be the
// one a later spawn escapes through. The critical section is map lookup plus
// append only - it never spans process or protocol I/O.
func (service *Service) beginAttempt(id providers.ID, request providers.ExecuteRequest) (*attempt, providers.ID, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	canonical, ok := service.resolveLocked(id)
	if !ok {
		return nil, canonical, false
	}
	target := service.providers[canonical]
	if target == nil {
		return nil, canonical, false
	}
	return target.newAttempt(request), canonical, true
}

func (service *Service) Execute(ctx context.Context, id providers.ID, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return service.run(ctx, id, request, nil)
}

// Continue resumes the exact private Providers session reference without
// permitting an ordinary Execute caller to select a prior session.
func (service *Service) Continue(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (providers.ExecuteResult, error) {
	return service.run(ctx, id, request, &reference)
}

// run registers one request-owned attempt against the resolved canonical
// provider and executes it. It never takes, waits for, or shares another
// attempt's live state: the attempt is released - unregistering itself and
// tearing down the process it owns - on every terminal path, including an
// unexpected unwind and a rejected working directory.
//
// The attempt is registered before the request's working root is validated so a
// request rejected afterwards is still owned by a Close or Configure that is
// already draining this provider; it owns no process at that point, so its
// teardown completes immediately.
func (service *Service) run(
	ctx context.Context,
	id providers.ID,
	request providers.ExecuteRequest,
	resume *providers.SessionRef,
) (providers.ExecuteResult, error) {
	attempt, canonical, ok := service.beginAttempt(id, request)
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: fmt.Sprintf("ACP provider %q is unavailable", id)}
	}
	defer attempt.release()
	cwd, err := absoluteWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return providers.ExecuteResult{}, invalidFailure(err)
	}
	return attempt.execute(ctx, cwd, openCodeEnvironment(canonical, requestEnvironment(request)), resume)
}

// resolveProvider resolves id (including an accepted alias) to its canonical
// identity and the provider owning that integration's configuration and live
// attempts. It is the read-only resolution used by callers that inspect a
// provider rather than register work against it; a caller that spawns must use
// beginAttempt so registration cannot be separated from resolution.
func (service *Service) resolveProvider(id providers.ID) (target *provider, canonical providers.ID, ok bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	canonical, ok = service.resolveLocked(id)
	if !ok {
		return nil, canonical, false
	}
	target = service.providers[canonical]
	return target, canonical, target != nil
}

// NegotiatedCapabilities returns the exact AgentCapabilities id's provider
// negotiated the last time one of its own attempts completed an ACP initialize
// handshake successfully, without starting a connection or any other side
// effect. Those are observed negotiation facts, not a capability this service
// infers or assumes.
func (service *Service) NegotiatedCapabilities(id providers.ID) (acpsdk.AgentCapabilities, bool) {
	target, _, ok := service.resolveProvider(id)
	if !ok {
		return acpsdk.AgentCapabilities{}, false
	}
	return target.negotiatedCapabilities()
}

// Claim atomically captures the exact live cancel-window generation named by
// id/attemptID, if this service currently has an established session/prompt
// turn in flight for it. The generation is bound to the canonical provider and
// to the one attempt that opened it, so a stale claim can never be redirected
// to a different attempt - including a replacement attempt that reuses the
// identical provider and attempt ID strings.
func (service *Service) Claim(id providers.ID, attemptID string) (acp.Generation, bool) {
	target, _, ok := service.resolveProvider(id)
	if !ok {
		return nil, false
	}
	return target.claim(attemptID)
}

// TryCancel delivers a session/cancel notification to the exact generation
// captured by a prior Claim call and blocks (bounded by ctx) until it
// observes that generation's real recorded terminal outcome. It never
// re-resolves generation by identity strings, so it cannot be redirected to
// a different generation that later reuses the same provider/attemptID.
func (service *Service) TryCancel(ctx context.Context, generation acp.Generation) (bool, error) {
	session, ok := generation.(*cancelwindow.Session)
	if !ok || session == nil {
		return false, nil
	}
	return session.TryCancel(ctx)
}

// Close stops every registered attempt and forgets the configured integration
// set. The provider registry is taken under the service lock and each
// provider's attempts are retired outside it, so closing never holds the
// service lock across process or protocol I/O and never misses an attempt that
// registered before it spawned its process.
func (service *Service) Close(ctx context.Context) error {
	service.mu.Lock()
	retired := make([]*provider, 0, len(service.providers))
	for _, target := range service.providers {
		retired = append(retired, target)
	}
	service.providers = map[providers.ID]*provider{}
	service.integrations = map[providers.ID]providers.ACPIntegration{}
	service.aliases = map[string]providers.ID{}
	service.mu.Unlock()
	var first error
	for _, target := range retired {
		if err := target.stopAttempts(ctx); err != nil && first == nil {
			first = fmt.Errorf("close ACP provider %q: %w", target.id, err)
		}
	}
	return first
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (service *Service) Configure(ctx context.Context, integrations []providers.ACPIntegration) error {
	commands := make(map[providers.ID]Command, len(integrations))
	values := make(map[providers.ID]providers.ACPIntegration, len(integrations))
	aliases := make(map[string]providers.ID, len(integrations))
	for _, integration := range integrations {
		integration = integration.Clone()
		if err := integration.Name.Validate(); err != nil {
			return fmt.Errorf("configure ACP service: %w", err)
		}
		if integration.Transport != "stdio" {
			return fmt.Errorf("configure ACP provider %q: unsupported transport %q", integration.Name, integration.Transport)
		}
		parts, err := parseACPCommand(integration.Command)
		if err != nil || len(parts) == 0 {
			return fmt.Errorf("configure ACP provider %q: invalid command", integration.Name)
		}
		commandArgs := parts[1:]
		if integration.Arguments != nil {
			if !slices.Equal(commandArgs, integration.Arguments) {
				return fmt.Errorf("configure ACP provider %q: command arguments drift from its runtime projection", integration.Name)
			}
			commandArgs = append([]string(nil), integration.Arguments...)
		}
		if integration.RuntimePosture == "catalog_only" {
			return fmt.Errorf("configure ACP provider %q: catalog-only integrations are not selectable", integration.Name)
		}
		if _, exists := values[integration.Name]; exists {
			return fmt.Errorf("configure ACP provider %q: duplicate identity", integration.Name)
		}
		commands[integration.Name] = Command{Name: parts[0], Args: commandArgs}
		values[integration.Name] = integration
		aliases[strings.ToLower(integration.Name.String())] = integration.Name
		for _, alias := range integration.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				return fmt.Errorf("configure ACP provider %q: invalid blank alias", integration.Name)
			}
			if existing, exists := aliases[alias]; exists && existing != integration.Name {
				return fmt.Errorf("configure ACP provider %q: alias %q collides with %q", integration.Name, alias, existing)
			}
			aliases[alias] = integration.Name
		}
	}

	service.mu.Lock()
	next := make(map[providers.ID]*provider, len(commands))
	retired := make([]*provider, 0, len(commands))
	for id, command := range commands {
		if current, ok := service.providers[id]; ok && current.matches(values[id], command) {
			// An unchanged integration keeps its provider object, so the
			// attempts already registered against it - and the capability
			// facts its own handshakes negotiated - stay live and untouched.
			next[id] = current
			continue
		}
		next[id] = newProvider(id, values[id], command, service.newCommand, service.locator, service.stdioPipes)
		if current := service.providers[id]; current != nil {
			retired = append(retired, current)
		}
	}
	for id, current := range service.providers {
		if _, kept := next[id]; !kept {
			retired = append(retired, current)
		}
	}
	service.providers, service.integrations, service.aliases = next, values, aliases
	service.mu.Unlock()

	// Only attempts belonging to a removed or changed integration are stopped
	// here; an unchanged configuration preserves the attempts still running
	// against it. Validation above has already completed, so a rejected
	// replacement never reaches this point.
	var first error
	for _, target := range retired {
		if err := target.stopAttempts(ctx); err != nil && first == nil {
			first = fmt.Errorf("drain ACP provider %q: %w", target.id, err)
		}
	}
	return first
}

func (service *Service) Integrations() []providers.ACPIntegration {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make([]providers.ACPIntegration, 0, len(service.integrations))
	for _, integration := range service.integrations {
		result = append(result, integration.Clone())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (service *Service) Resolve(id providers.ID) (providers.ID, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.resolveLocked(id)
}

func (service *Service) resolveLocked(id providers.ID) (providers.ID, bool) {
	canonical, ok := service.aliases[strings.ToLower(strings.TrimSpace(id.String()))]
	return canonical, ok
}

// provider is one configured ACP integration: its immutable launch
// configuration, the capability facts its own attempts actually negotiated, and
// the registry of live request-owned attempts currently executing against it.
// Its mutex guards only that registry; the facts have their own lock. Neither is
// ever held across process or protocol I/O.
type provider struct {
	id          providers.ID
	integration providers.ACPIntegration
	command     Command
	newCommand  platformprocess.CommandFactory
	locator     platformprocess.ExecutableLocator
	stdioPipes  platformprocess.StdioPipeFactory

	mu sync.Mutex
	// attempts is registration-ordered so Claim resolves an ambiguous
	// provider/attempt identity to the same attempt on every run rather than
	// to an arbitrary map-order member.
	attempts []*attempt

	factsMu sync.RWMutex
	facts   acpsdk.AgentCapabilities
	known   bool
}

func newProvider(
	id providers.ID,
	integration providers.ACPIntegration,
	command Command,
	newCommand platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
	stdioPipes platformprocess.StdioPipeFactory,
) *provider {
	return &provider{
		id:          id,
		integration: integration.Clone(),
		command:     Command{Name: command.Name, Args: append([]string(nil), command.Args...)},
		newCommand:  newCommand,
		locator:     locator,
		stdioPipes:  stdioPipes,
	}
}

// matches reports whether a reconfiguration leaves this provider's live
// attempts exactly as configured. Retention is decided by every field that
// decides what one of this provider's attempts is - the resolved launch
// executable and argv, plus the transport, runtime posture and implementation
// profile the rest of Providers uses to select and shape an ACP invocation -
// so a changed behavior field replaces the provider and drains its attempts
// instead of leaving work running against a configuration that no longer
// exists. Identity and aliases are deliberately not part of this comparison:
// they decide how a request resolves to this provider, which Configure rebuilds
// on every call, and never change what an attempt already running here is.
func (target *provider) matches(integration providers.ACPIntegration, command Command) bool {
	return target.command.Name == command.Name &&
		slices.Equal(target.command.Args, command.Args) &&
		target.integration.Transport == integration.Transport &&
		target.integration.RuntimePosture == integration.RuntimePosture &&
		target.integration.ImplementationProfile == integration.ImplementationProfile
}

// newAttempt allocates this request's attempt and registers it under the
// provider's short owner lock before any process is spawned, so Close and
// Configure can never miss an attempt that is about to own a process.
func (target *provider) newAttempt(request providers.ExecuteRequest) *attempt {
	lifecycle, cancelLifecycle := context.WithCancel(context.Background())
	owned := &attempt{
		provider:        target,
		request:         request,
		lifecycle:       lifecycle,
		cancelLifecycle: cancelLifecycle,
		startupSettled:  make(chan struct{}),
		teardownChanged: make(chan struct{}),
	}
	target.mu.Lock()
	target.attempts = append(target.attempts, owned)
	target.mu.Unlock()
	return owned
}

// claim returns the exact cancel-window generation one of this provider's live
// attempts currently has open for attemptID. The registry snapshot is taken
// under the short owner lock and each candidate's own synchronized window is
// consulted outside it, so a claim never blocks behind an unrelated attempt.
func (target *provider) claim(attemptID string) (acp.Generation, bool) {
	target.mu.Lock()
	live := append([]*attempt(nil), target.attempts...)
	target.mu.Unlock()
	for _, owned := range live {
		if generation, ok := owned.window.Claim(attemptID); ok {
			return generation, true
		}
	}
	return nil, false
}

// stopAttempts retires every attempt registered to this provider and then
// finishes tearing each one down. Cancellation is deliberately a separate pass:
// an attempt whose teardown has to wait for a peer or an in-flight startup must
// not hold back the cancellation of its siblings, or one stuck attempt would
// leave every other same-provider attempt running after this returned.
//
// Retire elects exactly one caller to begin each attempt's teardown, and the
// teardown claim keeps this from ever stopping an attempt twice, so this can
// neither double-stop an attempt that is already finishing on its own nor leave
// one unfinished. An attempt that never launched a process completes here
// immediately.
func (target *provider) stopAttempts(ctx context.Context) error {
	target.mu.Lock()
	live := target.attempts
	target.attempts = nil
	target.mu.Unlock()
	owned := make([]*attempt, 0, len(live))
	for _, current := range live {
		if current.retire() {
			owned = append(owned, current)
		}
	}
	var first error
	for _, current := range owned {
		if err := current.stop(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// negotiatedCapabilities returns the AgentCapabilities from the last successful
// initialize handshake one of this provider's own attempts completed, without
// touching any live attempt or causing a connection side effect. ok is false
// until the first successful handshake.
func (target *provider) negotiatedCapabilities() (acpsdk.AgentCapabilities, bool) {
	target.factsMu.RLock()
	defer target.factsMu.RUnlock()
	return target.facts, target.known
}

func (target *provider) recordNegotiated(capabilities acpsdk.AgentCapabilities) {
	target.factsMu.Lock()
	target.facts = capabilities
	target.known = true
	target.factsMu.Unlock()
}

// attempt is one request-owned ACP execution: it owns its own process,
// connection, client, standard streams, stderr capture, cancel window and
// lifecycle for exactly the span of one Execute or Continue, and it tears all
// of that down on every terminal path.
//
// All process state lives in one immutable handle bundle that startup publishes
// exactly once. Nothing ever clears or replaces a published handle, so the
// executing goroutine and a concurrent teardown may hold the same bundle
// without either of them observing the other's writes. stateMu guards only the
// lifecycle decisions around that publication - retiring, beginning a launch,
// startup having settled, and who currently owns the teardown - and its
// critical sections are memory-only, never process or protocol I/O.
type attempt struct {
	provider *provider
	request  providers.ExecuteRequest

	lifecycle       context.Context
	cancelLifecycle context.CancelFunc

	// stateMu guards retired, launching, handles, the settling of
	// startupSettled and the three teardown decisions below.
	stateMu sync.Mutex
	// retired is set once, by the single retire winner. From that moment this
	// attempt may no longer launch a process, which is what makes retirement a
	// complete answer to "may this attempt still spawn a process".
	retired bool
	// launching records that startup has claimed the right to launch and has
	// not yet settled, so a concurrent teardown knows a process can still
	// appear and must wait for publication instead of concluding there is
	// nothing to terminate.
	launching bool
	// handles is the published process bundle, or nil until startup publishes
	// one. It is written once and only read afterwards.
	handles *attemptHandles
	// startupSettled is closed exactly once, by startup, on every path -
	// success, refusal and failure - so a teardown waiting on a launch that
	// actually began is always released.
	startupSettled chan struct{}
	// teardownOwner elects the single caller currently performing this
	// attempt's process teardown. Retirement does not decide this: a Close or
	// Configure that retired an attempt can still give its teardown back when
	// its own context ends during startup, and the attempt's own release is
	// then the one caller that must finish the teardown.
	teardownOwner bool
	// teardownDone records that nothing is owned anymore - because a teardown
	// terminated the published process, or because startup settled without ever
	// publishing one - so no later caller has anything left to do.
	teardownDone bool
	// teardownChanged is closed and replaced on every change of the two
	// decisions above, so a caller waiting for a teardown it lost wakes for the
	// handover as well as for that teardown's completion.
	teardownChanged chan struct{}

	stderr syncBuffer

	// window tracks only this attempt's own live session/prompt turn.
	window cancelwindow.Window
}

// attemptHandles is the immutable set of process handles one attempt owns for
// the whole of its attempt: the launched process, its standard streams, its
// protocol connection and client, its wait channel and its supervised process
// tree. Teardown and execution may both hold this bundle concurrently, so every
// field is written before publication and never written again.
type attemptHandles struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	connection *acpsdk.ClientSideConnection
	client     *client
	finished   chan error
	tree       platformprocess.SubprocessTree
}

// release unregisters this attempt and completes the teardown of the process it
// owns. It runs on every terminal path of an Execute or Continue, including an
// unexpected unwind.
//
// Teardown is claimed, not assumed: when a Close or Configure already retired
// this attempt, either that teardown still owns it - and release waits for the
// completion it will reach - or that teardown gave it back because its own
// context ended while startup was still in flight, and release is the caller
// that must complete it. Releasing an attempt can therefore never leave a
// published process running, however its retirement was decided.
func (a *attempt) release() {
	a.provider.unregister(a)
	// Retirement only refuses later launches; the attempt's own terminal path
	// always offers to complete the teardown, and the claim below decides
	// whether this call is the one that performs it.
	a.retire()
	_ = a.stop(context.Background())
}

// retire cancels this attempt's own lifecycle, refuses any later launch, and
// elects the single caller that begins its process teardown. No caller can both
// retire and later spawn. Retirement decides only who starts the teardown, never
// that nobody finishes it: the teardown claim below is what carries an elected
// starter that hands its teardown back to the attempt's own release.
func (a *attempt) retire() bool {
	a.stateMu.Lock()
	if a.retired {
		a.stateMu.Unlock()
		return false
	}
	a.retired = true
	a.stateMu.Unlock()
	a.cancelLifecycle()
	return true
}

// beginLaunch claims this attempt's right to launch its process, and refuses
// once retirement has begun. Because that refusal is decided in the same
// critical section that records retirement, a spawn either happens entirely
// before a teardown observed this attempt - and is therefore published for that
// teardown - or does not happen at all.
func (a *attempt) beginLaunch() bool {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.retired {
		return false
	}
	a.launching = true
	return true
}

// publish records the launched process's immutable handle bundle. Publication
// happens exactly once per attempt, while this attempt's startup still holds
// the claim beginLaunch granted.
func (a *attempt) publish(handles *attemptHandles) {
	a.stateMu.Lock()
	a.handles = handles
	a.stateMu.Unlock()
}

// process returns the published handle bundle. A nil-error start guarantees one
// has been published, so every caller after start observes it non-nil.
func (a *attempt) process() *attemptHandles {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.handles
}

// settleStartup records that this attempt's startup will not touch a process
// again. Startup calls it exactly once, through a defer, on every path - a
// refusal, a failed launch and a successful launch alike - so a teardown that
// observed a launch in progress is always released, and one that observed no
// launch in progress never waits at all.
func (a *attempt) settleStartup() {
	a.stateMu.Lock()
	a.launching = false
	a.stateMu.Unlock()
	close(a.startupSettled)
}

func (target *provider) unregister(owned *attempt) {
	target.mu.Lock()
	defer target.mu.Unlock()
	for index, current := range target.attempts {
		if current == owned {
			target.attempts = slices.Delete(target.attempts, index, index+1)
			return
		}
	}
}

// execute runs one complete request-owned ACP attempt. Its defer makes every
// early return below leave nothing owned behind: the caller releases this
// attempt, which terminates this attempt's process.
func (a *attempt) execute(
	ctx context.Context,
	cwd string,
	environment []string,
	resume *providers.SessionRef,
) (providers.ExecuteResult, error) {
	request, id := a.request, a.provider.id
	executionCtx, cancelExecution := context.WithCancel(ctx)
	stopLifecycleWatch := context.AfterFunc(a.lifecycle, cancelExecution)
	defer func() {
		stopLifecycleWatch()
		cancelExecution()
	}()
	ctx = executionCtx
	if err := ctx.Err(); err != nil {
		return providers.ExecuteResult{}, nativeFailure(err)
	}
	if err := a.provider.preflight(ctx, cwd, environment); err != nil {
		return providers.ExecuteResult{}, err
	}
	prompt := promptBlocks(request)

	initialized, err := a.start(ctx, cwd, environment)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	// Every handle this attempt uses from here on comes from the one immutable
	// bundle startup published, so a concurrent Close or Configure that
	// terminates the process cannot race a read of this attempt's state.
	process := a.process()
	client, connection := process.client, process.connection

	client.reset(request.ProgressObserver)
	// Every exit below, including early failures that never reach a
	// completed/failed progress close, must stop this turn's delivery goroutine.
	defer client.release()
	session, err := a.openSession(ctx, cwd, connection, initialized, request, resume)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	client.suppressStartupChunk(providerStartupInfo(id, session.Meta))
	client.setSessionID(string(session.SessionId))
	request.ObserveSession(providers.SessionRef{
		Provider: id,
		Kind:     providers.SessionIDKind,
		ID:       string(session.SessionId),
	})
	modelConfig, err := applyAdvertisedModel(ctx, connection, session, request.Model)
	if err != nil {
		var failure providers.ExecuteFailure
		if errors.As(err, &failure) {
			return providers.ExecuteResult{}, withSessionRef(failure, id, string(session.SessionId))
		}
		return providers.ExecuteResult{}, withSessionRef(
			rpcFailure(ctx, "session/set_config_option", id, err, a.stderr.String(), request),
			id,
			string(session.SessionId),
		)
	}
	response, err := a.promptWithWindow(request.AttemptID, session.SessionId, connection, func() (acpsdk.PromptResponse, error) {
		return connection.Prompt(ctx, acpsdk.PromptRequest{SessionId: session.SessionId, Prompt: prompt})
	})
	if err != nil {
		if ctx.Err() != nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_ = connection.Cancel(cancelCtx, acpsdk.CancelNotification{SessionId: session.SessionId})
			cancel()
		}
		return providers.ExecuteResult{}, withPartial(rpcFailure(ctx, "session/prompt", id, err, a.stderr.String(), request), client, id)
	}
	if response.StopReason == acpsdk.StopReasonCancelled {
		return providers.ExecuteResult{}, withPartial(acpControlCanceledFailure(id), client, id)
	}
	if client.permissionDenied() && strings.TrimSpace(client.content()) == "" {
		return providers.ExecuteResult{}, withPartial(providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindUnknown,
			Message: fmt.Sprintf("ACP provider %q ended the turn without output after a permission request was denied", id),
		}, client, id)
	}
	return providers.ExecuteResult{Content: client.content(), SessionRef: &providers.SessionRef{Provider: id, Kind: providers.SessionIDKind, ID: string(session.SessionId)}, Diagnostics: &providers.ExecuteDiagnostics{Progress: client.completeProgress(), ProgressAlreadyObserved: request.ProgressObserver != nil, Metadata: map[string]string{"execution_kind": "acp", "protocol_version": fmt.Sprint(initialized.ProtocolVersion), "model_config": modelConfig, "completion_evidence": "provider_response"}}}, nil
}

// openSession resumes the exact private Provider Session reference through the native ACP
// session/load method with its exact opaque id forwarded unchanged, and its
// absence starts an ordinary fresh session/new. openSession never falls back
// from a requested resume to a fresh session - a session/load failure is
// returned as-is so a continuation can never silently adopt a different
// Provider Session, and whether this request's own peer supports session/load
// is answered by that peer on this attempt's own process rather than guessed
// from any assumed capability.
func (a *attempt) openSession(
	ctx context.Context,
	cwd string,
	connection *acpsdk.ClientSideConnection,
	initialized acpsdk.InitializeResponse,
	request providers.ExecuteRequest,
	resume *providers.SessionRef,
) (acpsdk.NewSessionResponse, error) {
	if resume != nil {
		if resumeID := strings.TrimSpace(resume.ID); resumeID != "" {
			loaded, err := connection.LoadSession(ctx, acpsdk.LoadSessionRequest{SessionId: acpsdk.SessionId(resumeID), Cwd: cwd, McpServers: []acpsdk.McpServer{}})
			if err != nil {
				return acpsdk.NewSessionResponse{}, a.sessionOpenFailure(ctx, "session/load", err, initialized, request)
			}
			return acpsdk.NewSessionResponse{Meta: loaded.Meta, ConfigOptions: loaded.ConfigOptions, Modes: loaded.Modes, SessionId: acpsdk.SessionId(resumeID)}, nil
		}
	}
	session, err := connection.NewSession(ctx, acpsdk.NewSessionRequest{Cwd: cwd, McpServers: []acpsdk.McpServer{}})
	if err != nil {
		return acpsdk.NewSessionResponse{}, a.sessionOpenFailure(ctx, "session/new", err, initialized, request)
	}
	return session, nil
}

const (
	// piAcpMetaKey and piAcpStartupInfoField address the `_meta.piAcp.startupInfo`
	// value pi-acp attaches to its session/new (and session/load) response. ACP
	// reserves `_meta` for agent-defined metadata, so this is a pi-acp
	// extension rather than a protocol guarantee.
	piAcpMetaKey          = "piAcp"
	piAcpStartupInfoField = "startupInfo"
)

func (target *provider) preflight(ctx context.Context, cwd string, environment []string) error {
	if target.id != providers.IDPi {
		return nil
	}
	return piPreflight(ctx, target.newCommand, target.locator, cwd, environment)
}

// pi-acp replays this session metadata as an agent message outside the prompt
// turn. It must not become the primary result when no model answer follows.
func providerStartupInfo(id providers.ID, meta map[string]any) string {
	if id != providers.IDPi {
		return ""
	}
	return piStartupInfo(meta)
}

// piStartupInfo returns the exact startup banner text an agent published under
// `_meta.piAcp.startupInfo`, or "" when the agent published none. The value is
// read from a freshly decoded JSON object, so the nested object arrives as
// map[string]any; the map[string]string case is accepted so a caller that
// built the response in process is not silently ignored.
func piStartupInfo(meta map[string]any) string {
	switch fields := meta[piAcpMetaKey].(type) {
	case map[string]any:
		startup, _ := fields[piAcpStartupInfoField].(string)
		return startup
	case map[string]string:
		return fields[piAcpStartupInfoField]
	}
	return ""
}

const (
	// acpErrorCodeResourceNotFound is the ACP-reserved JSON-RPC error code
	// (schema ErrorCodeResourceNotFound) an agent returns when a referenced
	// resource, including a session/load id, is not recognized.
	acpErrorCodeResourceNotFound = -32002

	// JSON-RPC reserves this inclusive range for server errors. ACP agents use
	// it for provider-side failures that are distinct from the standard JSON-RPC
	// protocol errors such as -32603.
	acpServerErrorMinimum = -32099
	acpServerErrorMaximum = -32000
)

// sessionOpenFailure normalizes a session/new or session/load failure. A
// session/load failure reporting ResourceNotFound means this attempt's own peer
// is healthy but does not recognize the exact requested Provider Session id, so
// it is reported as ExecuteFailureKindSessionNotFound instead of the generic RPC
// failure Continue would otherwise be unable to distinguish from any other
// dependency failure. Every other failure - including an authentication-required
// failure on an unusable connection - is reported through the same rpcFailure
// normalization every other ACP RPC failure in this service uses; the owned
// process is then retired by the caller's release on every one of these paths,
// so no unusable peer is ever retained.
func (a *attempt) sessionOpenFailure(
	ctx context.Context,
	method string,
	err error,
	initialized acpsdk.InitializeResponse,
	request providers.ExecuteRequest,
) error {
	var requestErr *acpsdk.RequestError
	if errors.As(err, &requestErr) && requestErr.Code == -32000 {
		// No login operation is exposed through this service, so this
		// connection cannot serve work until the operator authenticates. The
		// attempt's own release closes it now; the next request establishes a
		// fresh authenticated connection.
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: "ACP authentication required" + authenticationMethodHint(initialized.AuthMethods)}
	}
	if method == "session/load" && requestErr != nil && requestErr.Code == acpErrorCodeResourceNotFound {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindSessionNotFound,
			Message: fmt.Sprintf("ACP provider %q does not recognize the referenced Provider Session as live", a.provider.id),
		}
	}
	return rpcFailure(ctx, method, a.provider.id, err, a.stderr.String(), request)
}

// promptWithWindow opens the cancelable window for attemptID, runs prompt,
// and closes the window with the real recorded outcome - via defer, so an
// unexpected prompt unwind (including a panic) still closes the window
// instead of leaving it stale. A stale window would let a later execution
// that reuses this same attempt ID bind at the root and have a racing
// control claim, or hang indefinitely on, this dead session (see
// cancelwindow's package doc). The window is this attempt's own, so a
// concurrent attempt against the same provider is untouched by it.
// response/err are read by the deferred closure only after the assignment
// below completes, so a normal return still records the actual outcome.
func (a *attempt) promptWithWindow(
	attemptID string,
	sessionID acpsdk.SessionId,
	connection *acpsdk.ClientSideConnection,
	prompt func() (acpsdk.PromptResponse, error),
) (response acpsdk.PromptResponse, err error) {
	cancelable := a.window.Begin(attemptID, sessionID, connection)
	defer func() {
		a.window.End(cancelable, err == nil && response.StopReason == acpsdk.StopReasonCancelled)
	}()
	response, err = prompt()
	return response, err
}

func (target *provider) resolveLaunchName() (string, error) {
	name := target.command.Name
	if target.id == providers.ID("pi") && name == "you" && slices.Equal(target.command.Args, []string{"pi-acp"}) {
		current, ok := target.locator.(platformprocess.CurrentExecutableLocator)
		if !ok {
			return "", dependencyFailure("ACP current executable locator is unavailable")
		}
		resolved, err := current.CurrentExecutable()
		if err != nil {
			return "", dependencyFailure(fmt.Sprintf("resolve current executable for ACP pi bridge: %v", err))
		}
		if !filepath.IsAbs(resolved) {
			return "", dependencyFailure("ACP current executable locator returned a relative path")
		}
		return resolved, nil
	}
	if target.locator != nil {
		if _, err := target.locator.LookPath(name); err != nil {
			return "", providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindDependency,
				Message: fmt.Sprintf("ACP executable %q is unavailable", name),
				Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
					"work-failure-type": "missing_executable",
				}},
			}
		}
	}
	return name, nil
}

// classifyNegotiatedVersion owns the decision to reject an initialize
// response this service cannot speak. An agent that negotiates another
// protocol version is a provider configuration the operator must correct, so
// it is reported as a misconfigured provider rather than a dependency outage.
// It returns nil for the version this SDK speaks.
func classifyNegotiatedVersion(id providers.ID, initialized acpsdk.InitializeResponse) error {
	if initialized.ProtocolVersion == acpsdk.ProtocolVersionNumber {
		return nil
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindMisconfigured,
		Message: fmt.Sprintf("ACP provider %q negotiated unsupported protocol version %v", id, initialized.ProtocolVersion),
	}
}

// newACPStdio opens this peer's parent-owned standard-stream channel. The
// factory is the exact effect injected at composition; a missing one is reported
// as a dependency failure rather than defaulted here, so this service never
// selects a pipe implementation for itself. Every attempt gets its own channel,
// so no two attempts can ever read or write through the same pipe.
func (target *provider) newACPStdio() (platformprocess.StdioChannel, error) {
	if target.stdioPipes == nil {
		return nil, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP standard stream channel is unavailable"}
	}
	channel, err := target.stdioPipes()
	if err != nil {
		return nil, dependencyFailure(err.Error())
	}
	if channel == nil {
		return nil, dependencyFailure("ACP standard stream channel factory returned nil")
	}
	return channel, nil
}

// start launches this request's own process with this request's own working
// directory and environment, completes the ACP initialize handshake on it, and
// returns the negotiated initialize response.
//
// Publication is what lets a Close or Configure that retires this attempt during
// startup still terminate the process this attempt created instead of missing
// it. The bundle is published - including the connection and client, so no
// handle is ever handed out ahead of the process that owns it - before the
// handshake begins, and the defer settles startup on every path, so a teardown
// racing startup is released either way.
func (a *attempt) start(ctx context.Context, cwd string, environment []string) (acpsdk.InitializeResponse, error) {
	request, id := a.request, a.provider.id
	if !a.beginLaunch() {
		// Retirement already refused this launch, so there is no process for
		// anyone to wait for: report the retirement rather than starting a peer
		// nothing owns.
		return acpsdk.InitializeResponse{}, dependencyFailure(fmt.Sprintf("ACP provider %q attempt was retired before it started", id))
	}
	defer a.settleStartup()
	if a.provider.newCommand == nil {
		return acpsdk.InitializeResponse{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "ACP command is unavailable"}
	}
	if a.provider.stdioPipes == nil {
		return acpsdk.InitializeResponse{}, dependencyFailure("ACP standard stream channel is unavailable")
	}
	launchName, err := a.provider.resolveLaunchName()
	if err != nil {
		return acpsdk.InitializeResponse{}, err
	}
	cmd := a.provider.newCommand(launchName, a.provider.command.Args...)
	if cmd == nil {
		return acpsdk.InitializeResponse{}, dependencyFailure("ACP command factory returned nil")
	}
	cmd.Dir, cmd.Env = cwd, append([]string(nil), environment...)
	a.stderr.Reset()
	cmd.Stderr = &a.stderr
	// The parent-owned channel keeps the response reader independent of
	// cmd.Wait: this process holds its own read end until the peer is retired,
	// and releases only its copies of the child's ends once the start succeeds.
	stdio, err := a.provider.newACPStdio()
	if err != nil {
		return acpsdk.InitializeResponse{}, err
	}
	stdio.Attach(cmd)
	platformprocess.ConfigureSubprocessTree(cmd)
	if err := cmd.Start(); err != nil {
		stdio.Close()
		return acpsdk.InitializeResponse{}, dependencyFailure(fmt.Sprintf("start ACP provider %q: %v", id, err))
	}
	stdio.Detach()
	tree, _ := platformprocess.AttachSubprocessTree(cmd)
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	requests, responses := stdio.Requests(), stdio.Responses()
	client := &client{}
	connection := acpsdk.NewClientSideConnection(client, requests, responses)
	a.publish(&attemptHandles{
		cmd: cmd, stdin: requests, stdout: responses,
		connection: connection, client: client, finished: finished, tree: tree,
	})
	initialized, err := connection.Initialize(ctx, acpsdk.InitializeRequest{
		ProtocolVersion:    acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpClientCapabilities(),
	})
	if err != nil {
		// An agent's final diagnostic reaches this attempt's stderr as it fails
		// the handshake, and os/exec finishes copying that stream only when the
		// process is reaped. Completing this attempt's own arbitrated teardown -
		// the stop the caller's release performs anyway - first lets that copy
		// finish, so the redacted stderr detail is captured instead of racing
		// it. Retirement is deliberately not performed here: this failure keeps
		// its original RPC cause, context and typed classification.
		_ = a.stop(context.Background())
		return acpsdk.InitializeResponse{}, rpcFailure(ctx, "initialize", id, err, a.stderr.String(), request)
	}
	if version := classifyNegotiatedVersion(id, initialized); version != nil {
		return acpsdk.InitializeResponse{}, version
	}
	a.provider.recordNegotiated(initialized.AgentCapabilities)
	return initialized, nil
}

// stop terminates the process this attempt owns and releases every stream it
// published. It waits only for a launch that has actually begun, so an attempt
// that never started one - a canceled or preflight-rejected request, or an
// attempt a Close or Configure retired before it could spawn - completes its
// teardown immediately instead of waiting on an operation that will never
// happen.
//
// Exactly one caller performs the teardown, so the process and its streams are
// terminated once however many callers reach here. If this caller's own context
// ends while startup is still in flight it hands the teardown back rather than
// terminating nothing, which is what keeps a later spawned process owned even
// when an external Close or Configure could not wait for it.
func (a *attempt) stop(ctx context.Context) error {
	owned, err := a.acquireTeardown(ctx)
	if !owned {
		// Another caller already completed this teardown, so nothing is owned;
		// or this caller's own context ended before it could take the teardown.
		return err
	}
	handles, err := a.awaitStartup(ctx)
	if err != nil {
		// Startup has not settled, so a process this attempt would own can still
		// be published. Handing the teardown back instead of abandoning it is
		// what makes the eventual caller that does reach a settled startup -
		// this attempt's own release - the one that terminates that process.
		a.handBackTeardown()
		return err
	}
	if handles == nil {
		// Startup settled without ever publishing a process, so this attempt owns
		// nothing and no later caller has anything left to do.
		a.completeTeardown()
		return nil
	}
	defer a.completeTeardown()
	return handles.terminate(ctx)
}

// acquireTeardown elects the single caller that performs this attempt's process
// teardown. owned is false, with a nil error, once teardown has completed,
// because then this attempt owns nothing any caller could terminate. A caller
// that finds another caller performing the teardown waits for that teardown to
// complete or hand ownership back, so a handover is always picked up by exactly
// one caller instead of being dropped between two.
func (a *attempt) acquireTeardown(ctx context.Context) (bool, error) {
	for {
		a.stateMu.Lock()
		if a.teardownDone {
			a.stateMu.Unlock()
			return false, nil
		}
		if !a.teardownOwner {
			a.teardownOwner = true
			a.stateMu.Unlock()
			return true, nil
		}
		changed := a.teardownChanged
		a.stateMu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

// completeTeardown records that this attempt owns nothing further: either its
// published process was terminated, or startup settled without ever publishing
// one. It wakes every caller waiting to learn the teardown's outcome.
func (a *attempt) completeTeardown() {
	a.stateMu.Lock()
	a.teardownOwner, a.teardownDone = false, true
	a.signalTeardownLocked()
	a.stateMu.Unlock()
}

// handBackTeardown returns an unfinished teardown to the pool of claimants,
// because this caller released it before terminating anything. No goroutine is
// left behind to finish it: the attempt's own release is the caller that
// reaches it, and it claims the teardown through the same critical section.
func (a *attempt) handBackTeardown() {
	a.stateMu.Lock()
	a.teardownOwner = false
	a.signalTeardownLocked()
	a.stateMu.Unlock()
}

// signalTeardownLocked wakes every caller waiting on the teardown decisions.
// Callers must hold stateMu.
func (a *attempt) signalTeardownLocked() {
	close(a.teardownChanged)
	a.teardownChanged = make(chan struct{})
}

// awaitStartup returns this attempt's published handle bundle, waiting only
// when startup has claimed a launch and has not settled yet.
//
// It returns the caller's own context error when that context ends before
// startup settles, including the case where a process was published in the same
// instant; the bundle is published exactly once and stays readable, so the
// caller that takes over the handed-back teardown still finds it.
func (a *attempt) awaitStartup(ctx context.Context) (*attemptHandles, error) {
	a.stateMu.Lock()
	handles, launching := a.handles, a.launching
	a.stateMu.Unlock()
	if handles != nil || !launching {
		return handles, nil
	}
	select {
	case <-a.startupSettled:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return a.process(), nil
}

// terminate ends the launched peer and releases the streams this attempt owns.
// The bundle is immutable, so this can run while the attempt's own goroutine is
// still issuing protocol calls: those calls fail against the closed stream and
// the process, which is the outcome that ends this attempt.
func (h *attemptHandles) terminate(ctx context.Context) error {
	if h == nil {
		return nil
	}
	_ = h.stdin.Close()
	var stopErr error
	exited := false
	select {
	case <-h.finished:
		exited = true
	case <-ctx.Done():
		stopErr = ctx.Err()
		_ = platformprocess.TerminateSubprocessTree(h.cmd, h.tree)
	case <-time.After(500 * time.Millisecond):
		_ = platformprocess.TerminateSubprocessTree(h.cmd, h.tree)
	}
	if !exited {
		select {
		case <-h.finished:
		case <-time.After(2 * time.Second):
			if stopErr == nil {
				stopErr = errors.New("ACP process did not exit after termination")
			}
		}
	}
	platformprocess.CloseSubprocessTree(h.cmd, h.tree)
	// The response reader outlives the peer only while the peer is retained;
	// releasing it here keeps a retired peer from holding an open pipe handle.
	_ = h.stdout.Close()
	return stopErr
}

func requestEnvironment(request providers.ExecuteRequest) []string {
	if request.EnvVars == nil {
		return append([]string(nil), request.ProcessEnvironment...)
	}
	values := append([]string(nil), request.ProcessEnvironment...)
	for key, value := range request.EnvVars {
		values = append(values, key+"="+value)
	}
	return values
}

func openCodeEnvironment(id providers.ID, environment []string) []string {
	if id != providers.IDOpenCode {
		return environment
	}
	for _, entry := range environment {
		name, _, assigned := strings.Cut(entry, "=")
		if assigned && strings.EqualFold(name, "OPENCODE_CONFIG_CONTENT") {
			return environment
		}
	}
	return append(environment, `OPENCODE_CONFIG_CONTENT={"snapshots":false}`)
}

func absoluteWorkingDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("ACP working directory is required")
	}
	return filepath.Abs(value)
}

func applyAdvertisedModel(ctx context.Context, connection *acpsdk.ClientSideConnection, session acpsdk.NewSessionResponse, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "not_requested", nil
	}
	for _, config := range session.ConfigOptions {
		if config.Select == nil || config.Select.Category == nil || *config.Select.Category != acpsdk.SessionConfigOptionCategoryModel || !selectOptionContains(config.Select.Options, acpsdk.SessionConfigValueId(model)) {
			continue
		}
		_, err := connection.SetSessionConfigOption(ctx, acpsdk.SetSessionConfigOptionRequest{ValueId: &acpsdk.SetSessionConfigOptionValueId{SessionId: session.SessionId, ConfigId: config.Select.Id, Value: acpsdk.SessionConfigValueId(model)}})
		if err != nil {
			return "failed", err
		}
		return "applied", nil
	}
	return "not_advertised", providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindInvalidRequest,
		Message: fmt.Sprintf("ACP session does not advertise requested model %q", model),
	}
}

func selectOptionContains(options acpsdk.SessionConfigSelectOptions, model acpsdk.SessionConfigValueId) bool {
	if options.Ungrouped != nil {
		for _, option := range *options.Ungrouped {
			if option.Value == model {
				return true
			}
		}
	}
	if options.Grouped != nil {
		for _, group := range *options.Grouped {
			for _, option := range group.Options {
				if option.Value == model {
					return true
				}
			}
		}
	}
	return false
}

func authenticationMethodHint(methods []acpsdk.AuthMethod) string {
	labels := []string{}
	for _, method := range methods {
		switch {
		case method.Agent != nil:
			labels = append(labels, method.Agent.Name)
		case method.EnvVar != nil:
			labels = append(labels, method.EnvVar.Name)
		case method.Terminal != nil:
			labels = append(labels, method.Terminal.Name)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "; advertised methods: " + strings.Join(labels, ", ")
}

func rpcFailure(ctx context.Context, method string, id providers.ID, err error, stderr string, request providers.ExecuteRequest) error {
	if ctx.Err() != nil {
		return nativeFailure(ctx.Err())
	}
	piModelConnection := isPiModelConnectionFailure(id, err)
	kind := rpcFailureKind(err, piModelConnection)
	message := fmt.Sprintf("ACP provider %q %s failed: %s", id, method, safeRPCMessage(err))
	if piModelConnection {
		message = fmt.Sprintf("ACP provider %q could not connect to the selected model endpoint; check the model endpoint and its configuration", id)
	} else if kind == providers.ExecuteFailureKindThrottled {
		message = fmt.Sprintf("ACP provider %q is temporarily unavailable due to usage or capacity limits", id)
	} else if isACPPeerClosedFailure(err) {
		message = fmt.Sprintf("ACP provider %q disconnected before responding; retry the request", id)
	}
	// A disconnect can accompany an agent's final stderr diagnostic. Keep
	// that redacted evidence without exposing private model/throttle details
	// or changing the authoritative RPC failure classification.
	if !piModelConnection && kind != providers.ExecuteFailureKindThrottled {
		if detail := safeACPStderr(stderr, request.EnvVars); detail != "" {
			message += " (stderr: " + detail + ")"
		}
	}
	if method == "initialize" {
		native := strings.ToLower(err.Error())
		if strings.Contains(native, "protocol version") || strings.Contains(native, "protocolversion") {
			kind = providers.ExecuteFailureKindMisconfigured
		}
	}
	errorCode := "ACP_" + strings.ToUpper(strings.ReplaceAll(method, "session/", "")) + "_FAILED"
	if piModelConnection {
		errorCode = "ACP_PI_MODEL_CONNECTION"
	} else if isACPPeerClosedFailure(err) {
		errorCode = "ACP_PEER_CLOSED"
	}
	return providers.ExecuteFailure{Kind: kind, Message: message, Diagnostics: &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{
		Phase: "failed", Detail: message, Metadata: map[string]string{
			"kind": "error", "native_type": method,
			"error_code": errorCode,
		},
	}}}}
}

func rpcFailureKind(err error, piModelConnection bool) providers.ExecuteFailureKind {
	var requestErr *acpsdk.RequestError
	if piModelConnection {
		return providers.ExecuteFailureKindMisconfigured
	} else if errors.As(err, &requestErr) && isACPRateLimitFailure(requestErr.Message) {
		return providers.ExecuteFailureKindThrottled
	} else if requestErr != nil && isACPServerFailure(requestErr.Code) {
		// A server-side ACP failure is a provider dependency outcome. Workers
		// classifies that outcome as retryable and, when a session was opened,
		// retains the exact Provider Session for the retry continuation.
		return providers.ExecuteFailureKindDependency
	} else if isACPPeerClosedFailure(err) {
		// The ACP SDK's structured peer-disconnect error is a provider
		// dependency outcome. It is classified separately from arbitrary
		// provider-authored -32603 errors so that only the exact SDK shape
		// is recognized.
		return providers.ExecuteFailureKindDependency
	}
	return providers.ExecuteFailureKindUnknown
}

// isPiModelConnectionFailure recognizes only Pi's typed bridge outcome. The
// provider-authored message and other data fields are never used as evidence.
func isPiModelConnectionFailure(id providers.ID, err error) bool {
	if id != "pi" {
		return false
	}
	var requestErr *acpsdk.RequestError
	if !errors.As(err, &requestErr) || requestErr.Code != -32603 {
		return false
	}
	data, ok := requestErr.Data.(map[string]any)
	if !ok {
		return false
	}
	return data["provider"] == "pi" && data["outcome"] == "error" && data["failureKind"] == "model_connection"
}

func isACPRateLimitFailure(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(normalized, "rate limit") || strings.Contains(normalized, "too many requests") ||
		strings.Contains(normalized, "resource exhausted") || strings.Contains(normalized, "at capacity") ||
		strings.Contains(normalized, "http 429")
}

func isACPServerFailure(code int) bool {
	return code >= acpServerErrorMinimum && code <= acpServerErrorMaximum &&
		code != -32000 && code != acpErrorCodeResourceNotFound
}

// isACPPeerClosedFailure reports whether err is the ACP SDK's structured
// peer-disconnect RequestError: code -32603 with Data carrying the exact
// "peer disconnected before response" or "peer disconnected while waiting
// for pre-response notifications" marker.
func isACPPeerClosedFailure(err error) bool {
	var requestErr *acpsdk.RequestError
	if !errors.As(err, &requestErr) {
		return false
	}
	if requestErr.Code != -32603 || requestErr.Message != "Internal error" {
		return false
	}
	data, ok := requestErr.Data.(map[string]any)
	if !ok {
		return false
	}
	errorValue, _ := data["error"].(string)
	switch errorValue {
	case "peer disconnected before response", "peer disconnected while waiting for pre-response notifications":
		return true
	}
	return false
}

func safeRPCMessage(err error) string {
	var requestErr *acpsdk.RequestError
	if errors.As(err, &requestErr) && strings.TrimSpace(requestErr.Message) != "" {
		return strings.TrimSpace(requestErr.Message)
	}
	message := strings.TrimSpace(err.Error())
	if message != "" && len(message) <= 256 && !strings.ContainsAny(message, "{}[]\\/") {
		return message
	}
	return "RPC request failed"
}

var renderedURL = regexp.MustCompile(`https?://[^\s\]\)]+`)

// promptBlocks assembles one session/prompt turn's content blocks: an
// optional system-instructions block, the user message, then any input Work
// content and resource links derived from it.
func promptBlocks(request providers.ExecuteRequest) []acpsdk.ContentBlock {
	prompt := []acpsdk.ContentBlock{}
	if text := strings.TrimSpace(request.SystemPrompt); text != "" {
		prompt = append(prompt, acpsdk.TextBlock("System instructions:\n"+text))
	}
	prompt = append(prompt, acpsdk.TextBlock(request.UserMessage))
	prompt = append(prompt, inputWorkBlocks(request.InputTokens, request.UserMessage)...)
	prompt = append(prompt, resourceLinks(request.InputTokens, request.UserMessage)...)
	return prompt
}

func inputWorkBlocks(values []any, renderedPrompt string) []acpsdk.ContentBlock {
	blocks := make([]acpsdk.ContentBlock, 0)
	seen := map[string]struct{}{}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var token struct {
			Color struct {
				Name    string `json:"name"`
				WorkID  string `json:"work_id"`
				Payload []byte `json:"payload"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"color"`
		}
		if json.Unmarshal(encoded, &token) != nil {
			continue
		}
		name := strings.TrimSpace(token.Color.Name)
		if name == "" {
			name = strings.TrimSpace(token.Color.WorkID)
		}
		for _, content := range token.Color.Content {
			text := strings.TrimSpace(content.Text)
			if !strings.EqualFold(content.Type, "text") || text == "" || strings.Contains(renderedPrompt, text) {
				continue
			}
			key := name + "\x00" + text
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if name != "" {
				text = "Input work " + name + ":\n" + text
			} else {
				text = "Input work:\n" + text
			}
			blocks = append(blocks, acpsdk.TextBlock(text))
		}
		payload := strings.TrimSpace(string(token.Color.Payload))
		if payload != "" && !strings.Contains(renderedPrompt, payload) {
			key := name + "\x00" + payload
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				if name != "" {
					payload = "Input work " + name + ":\n" + payload
				} else {
					payload = "Input work:\n" + payload
				}
				blocks = append(blocks, acpsdk.TextBlock(payload))
			}
		}
	}
	return blocks
}

func resourceLinks(values []any, renderedPrompt string) []acpsdk.ContentBlock {
	var blocks []acpsdk.ContentBlock
	seen := map[string]struct{}{}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var decoded any
		if json.Unmarshal(encoded, &decoded) != nil {
			continue
		}
		for _, block := range resourceLinksFromJSON(decoded) {
			if _, ok := seen[block.ResourceLink.Uri]; ok {
				continue
			}
			seen[block.ResourceLink.Uri] = struct{}{}
			blocks = append(blocks, block)
		}
	}
	for _, url := range renderedURL.FindAllString(renderedPrompt, -1) {
		if _, ok := seen[url]; ok {
			continue
		}
		block := acpsdk.ResourceLinkBlock(url, url)
		ext := strings.ToLower(filepath.Ext(url))
		mime := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}[ext]
		if mime != "" {
			block.ResourceLink.MimeType = &mime
		}
		seen[url] = struct{}{}
		blocks = append(blocks, block)
	}
	return blocks
}

func resourceLinksFromJSON(value any) []acpsdk.ContentBlock {
	var blocks []acpsdk.ContentBlock
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			blocks = append(blocks, resourceLinksFromJSON(child)...)
		}
	case map[string]any:
		url, _ := typed["url"].(string)
		if strings.TrimSpace(url) != "" {
			name, _ := typed["label"].(string)
			if strings.TrimSpace(name) == "" {
				name = url
			}
			block := acpsdk.ResourceLinkBlock(name, url)
			if mime, ok := typed["contentType"].(string); ok && mime != "" {
				block.ResourceLink.MimeType = &mime
			}
			blocks = append(blocks, block)
		}
		for key, child := range typed {
			if key != "url" {
				blocks = append(blocks, resourceLinksFromJSON(child)...)
			}
		}
	}
	return blocks
}
func invalidFailure(err error) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: err.Error()}
}
func dependencyFailure(message string) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: message}
}
func nativeFailure(err error) error {
	kind := providers.ExecuteFailureKindUnknown
	if errors.Is(err, context.Canceled) {
		kind = providers.ExecuteFailureKindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = providers.ExecuteFailureKindTimeout
	}
	return providers.ExecuteFailure{Kind: kind, Message: err.Error()}
}
func withSessionRef(err error, provider providers.ID, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return err
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		return err
	}
	failure = failure.Clone()
	if failure.SessionRef == nil {
		failure.SessionRef = &providers.SessionRef{
			Provider: provider,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return failure
}

func withPartial(err error, c *client, provider providers.ID) error {
	var f providers.ExecuteFailure
	if errors.As(err, &f) {
		f = f.Clone()
		var failureProgress []providers.ExecuteProgress
		if f.Diagnostics != nil {
			for _, progress := range f.Diagnostics.Progress {
				failureProgress = append(failureProgress, progress.Clone())
			}
		}
		progress := c.failProgress(failureProgress...)
		f.Diagnostics = &providers.ExecuteDiagnostics{
			Progress:                progress,
			ProgressAlreadyObserved: c.streamedProgress(),
			Metadata:                map[string]string{"partial_content": c.content()},
		}
		if f.SessionRef == nil {
			f.SessionRef = c.sessionRef(provider)
		}
		return f
	}
	return err
}

func safeACPStderr(value string, env map[string]string) string {
	for name, secret := range env {
		if sensitiveEnvironmentName(name) && len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
func sensitiveEnvironmentName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"API_KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

type client struct {
	mu                  sync.Mutex
	permissionWasDenied bool
	text                strings.Builder
	sessionID           string
	// startupChunk is the exact startup banner text this turn's session
	// metadata identified, empty when the provider published none. Chunks
	// matching it are never accumulated as result content.
	startupChunk string
	stream       *promptProgressStream
}

// reset begins one turn's progress stream. observe may be nil, in which case
// the turn still normalizes its facts but delivers them only in the returned
// diagnostics.
func (c *client) reset(observe providers.ProgressObserver) {
	c.mu.Lock()
	c.permissionWasDenied = false
	c.text.Reset()
	c.sessionID = ""
	c.startupChunk = ""
	previous := c.stream
	c.stream = newPromptProgressStream(observe)
	c.mu.Unlock()
	// A turn replaced before it was closed would otherwise strand its delivery
	// goroutine. Close is idempotent and flushes queued facts, and runs outside
	// the lock so a slow observer cannot block the client.
	if previous != nil {
		previous.close()
	}
}

// release stops the current turn's delivery goroutine after flushing queued
// facts. It is idempotent and safe after completeProgress or failProgress.
func (c *client) release() {
	c.mu.Lock()
	stream := c.stream
	c.mu.Unlock()
	if stream != nil {
		stream.close()
	}
}

// suppressStartupChunk records the exact startup banner text the session
// metadata identified and removes it from the text this turn has accumulated
// so far.
//
// Order matters and both orders are handled. The banner reaches the client as
// an ordinary agent_message_chunk, and an agent that emits it outside the
// prompt turn can deliver that notification before session/new has returned -
// that chunk is already inside c.text and can only be withdrawn here. A
// notification that arrives later is dropped by SessionUpdate instead, because
// this call has already recorded the text. Both paths mutate only c.text, so
// real prompt output and the source-native progress facts the observer
// receives are untouched either way.
func (c *client) suppressStartupChunk(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startupChunk = strings.TrimSpace(value)
	accumulated := c.text.String()
	remainder := strings.Replace(accumulated, value, "", 1)
	if remainder == accumulated {
		return
	}
	c.text.Reset()
	// A turn whose only accumulated text was the banner must read as empty, not
	// as the banner's trailing whitespace, so a caller judging whether the
	// provider produced any output at all is not misled.
	if strings.TrimSpace(remainder) != "" {
		c.text.WriteString(remainder)
	}
}

// isStartupChunkLocked reports whether text is the exact startup banner this
// turn's session metadata identified. Callers must hold c.mu.
func (c *client) isStartupChunkLocked(text string) bool {
	return c.startupChunk != "" && strings.TrimSpace(text) == c.startupChunk
}

func (c *client) SessionUpdate(_ context.Context, n acpsdk.SessionNotification) error {
	p, text := mapSessionUpdate(n.Update)
	c.mu.Lock()
	if text != "" && !c.isStartupChunkLocked(text) {
		c.text.WriteString(text)
	}
	stream := c.stream
	var pending []providers.ExecuteProgress
	if stream != nil {
		stream.Observe(p)
		pending = stream.takePending()
	}
	c.mu.Unlock()
	// Handed off after the lock is released. This callback runs on the ACP
	// connection's notification path, and the SDK holds a turn's
	// session/prompt response until every pre-response notification handler
	// returns, so downstream publishing must not happen inline here.
	if stream != nil {
		stream.Deliver(pending)
	}
	return nil
}
func (c *client) setSessionID(v string) { c.mu.Lock(); c.sessionID = v; c.mu.Unlock() }
func (c *client) content() string       { c.mu.Lock(); defer c.mu.Unlock(); return c.text.String() }
func (c *client) permissionDenied() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.permissionWasDenied
}
func (c *client) sessionRef(provider providers.ID) *providers.SessionRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(c.sessionID) == "" {
		return nil
	}
	return &providers.SessionRef{Provider: provider, Kind: providers.SessionIDKind, ID: c.sessionID}
}

// completeProgress closes the turn normally and returns its full ordered fact
// list, which is exactly the sequence already streamed to the observer.
func (c *client) completeProgress() []providers.ExecuteProgress {
	return c.closeProgress(func(stream *promptProgressStream) []providers.ExecuteProgress {
		return stream.Complete()
	})
}

// failProgress closes a turn that never reached its own completed marker,
// folding the transport's own failure diagnostics into the same stream so the
// returned slice still matches what was observed.
func (c *client) failProgress(extra ...providers.ExecuteProgress) []providers.ExecuteProgress {
	return c.closeProgress(func(stream *promptProgressStream) []providers.ExecuteProgress {
		return stream.Fail(extra...)
	})
}

// closeProgress runs one turn-closing step under the client lock, then hands
// off and joins delivery outside it, so every fact this turn reports has
// reached the observer by the time the caller inspects the returned slice.
func (c *client) closeProgress(
	finish func(*promptProgressStream) []providers.ExecuteProgress,
) []providers.ExecuteProgress {
	c.mu.Lock()
	stream := c.stream
	var facts, pending []providers.ExecuteProgress
	if stream != nil {
		facts = finish(stream)
		pending = stream.takePending()
	}
	c.mu.Unlock()
	if stream == nil {
		return nil
	}
	stream.Deliver(pending)
	stream.close()
	return facts
}

// streamedProgress reports whether this turn delivered its facts live, so a
// caller knows not to publish the returned slice a second time.
func (c *client) streamedProgress() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream != nil && c.stream.observe != nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func mapSessionUpdate(update acpsdk.SessionUpdate) ([]providers.ExecuteProgress, string) {
	phase, detail, kind, itemID := "update", "", "unknown", ""
	metadata := map[string]string{}
	switch {
	case update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil:
		kind = "message"
		metadata["native_type"] = "agent_message_chunk"
		phase = "delta"
		detail = update.AgentMessageChunk.Content.Text.Text
		itemID = optionalItemID(update.AgentMessageChunk.MessageId, "assistant-message")
	case update.AgentThoughtChunk != nil && update.AgentThoughtChunk.Content.Text != nil:
		kind = "reasoning"
		metadata["native_type"] = "agent_thought_chunk"
		phase = "delta"
		detail = update.AgentThoughtChunk.Content.Text.Text
		itemID = optionalItemID(update.AgentThoughtChunk.MessageId, "assistant-reasoning")
	case update.ToolCall != nil:
		kind = "tool"
		metadata["native_type"] = "tool_call"
		phase = "started"
		detail = update.ToolCall.Title
		itemID = string(update.ToolCall.ToolCallId)
		metadata["status"] = string(update.ToolCall.Status)
		encodeMetadata(metadata, "raw_input", update.ToolCall.RawInput)
	case update.ToolCallUpdate != nil:
		kind, phase = "tool", "updated"
		metadata["native_type"] = "tool_call_update"
		itemID = string(update.ToolCallUpdate.ToolCallId)
		if update.ToolCallUpdate.Title != nil {
			detail = *update.ToolCallUpdate.Title
		}
		if update.ToolCallUpdate.Status != nil {
			metadata["status"] = string(*update.ToolCallUpdate.Status)
			if *update.ToolCallUpdate.Status == acpsdk.ToolCallStatusCompleted {
				phase = "completed"
			}
		}
		encodeMetadata(metadata, "raw_output", update.ToolCallUpdate.RawOutput)
		metadata["kind"], metadata["item_id"], metadata["provider_session_id"] = kind, itemID, ""
		progress := []providers.ExecuteProgress{{Phase: phase, Detail: detail, Metadata: metadata}}
		for _, content := range update.ToolCallUpdate.Content {
			if content.Diff == nil {
				continue
			}
			operation := "modified"
			if content.Diff.OldText == nil {
				operation = "created"
			}
			progress = append(progress, providers.ExecuteProgress{
				Phase:  "updated",
				Detail: content.Diff.Path,
				Metadata: map[string]string{
					"kind":                "file_change",
					"item_id":             "file:" + content.Diff.Path,
					"native_type":         "tool_call_update",
					"provider_session_id": "",
					"path":                content.Diff.Path,
					"operation":           operation,
					// ACP models a diff as content inside the tool call that
					// produced it. Carrying the owning call's id keeps that
					// ownership, so a consumer can attach the change to its
					// tool call instead of presenting an orphaned edit.
					"tool_call_id": string(update.ToolCallUpdate.ToolCallId),
				},
			})
		}
		return progress, ""
	case update.Plan != nil:
		kind = "plan"
		metadata["native_type"] = "plan"
		phase = "updated"
		itemID = "plan"
		encodeMetadata(metadata, "entries", update.Plan.Entries)
	case update.UsageUpdate != nil:
		kind = "usage"
		metadata["native_type"] = "usage_update"
		phase = "updated"
		itemID = "usage"
		metadata["used_tokens"] = fmt.Sprint(update.UsageUpdate.Used)
		metadata["max_tokens"] = fmt.Sprint(update.UsageUpdate.Size)
	case update.SessionInfoUpdate != nil:
		kind = "session"
		metadata["native_type"] = "session_info_update"
		phase = "updated"
		itemID = "session"
		if update.SessionInfoUpdate.Title != nil {
			detail = *update.SessionInfoUpdate.Title
			metadata["title_present"] = "true"
		}
	default:
		return nil, ""
	}
	metadata["kind"], metadata["item_id"], metadata["provider_session_id"] = kind, itemID, ""
	return []providers.ExecuteProgress{{Phase: phase, Detail: detail, Metadata: metadata}}, func() string {
		if kind == "message" {
			return detail
		}
		return ""
	}()
}
func encodeMetadata(target map[string]string, key string, value any) {
	if value == nil {
		return
	}
	if data, err := json.Marshal(value); err == nil {
		target[key] = string(data)
	}
}
func optionalItemID(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return strings.TrimSpace(*value)
	}
	return fallback
}

func (c *client) RequestPermission(ctx context.Context, request acpsdk.RequestPermissionRequest) (acpsdk.RequestPermissionResponse, error) {
	if ctx.Err() != nil {
		return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	// ACP invocations are noninteractive. Grant the broadest advertised allow
	// choice so the peer can continue without a permission prompt.
	for _, kind := range []acpsdk.PermissionOptionKind{
		acpsdk.PermissionOptionKindAllowAlways,
		acpsdk.PermissionOptionKindAllowOnce,
	} {
		for _, option := range request.Options {
			if option.Kind == kind {
				return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
			}
		}
	}
	for _, option := range request.Options {
		if option.Kind == acpsdk.PermissionOptionKindRejectOnce || option.Kind == acpsdk.PermissionOptionKindRejectAlways {
			c.mu.Lock()
			c.permissionWasDenied = true
			c.mu.Unlock()
			return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
		}
	}
	c.mu.Lock()
	c.permissionWasDenied = true
	c.mu.Unlock()
	return acpsdk.RequestPermissionResponse{Outcome: acpsdk.NewRequestPermissionOutcomeCancelled()}, nil
}
func (*client) ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	return acpsdk.ReadTextFileResponse{}, errors.New("ACP filesystem reads are not supported")
}
func (*client) WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	return acpsdk.WriteTextFileResponse{}, errors.New("ACP filesystem writes are not supported")
}
func (*client) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	return acpsdk.KillTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, errors.New("ACP terminals are not supported")
}
func (*client) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, errors.New("ACP terminals are not supported")
}

var _ acpsdk.Client = (*client)(nil)

// syncBuffer is a bytes.Buffer guarded for concurrent use.
//
// The daemon hands its stderr buffer to os/exec, which copies the child's
// stderr on its own goroutine for the process' whole lifetime, while the
// executing goroutine reads the accumulated text whenever it builds a failure
// diagnostic. Those are genuinely concurrent, so the buffer cannot be a bare
// bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
