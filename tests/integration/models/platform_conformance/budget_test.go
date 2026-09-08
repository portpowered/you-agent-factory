package platform_conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
)

// BudgetLocker is the exact cross-process effect needed by the durable
// ledger. The coordinator never implements a second in-memory lock because
// reservations may be made by separate runner processes.
type BudgetLocker interface {
	Lock(context.Context, string) (io.Closer, error)
}

type BudgetStore struct {
	locker BudgetLocker
}

func NewBudgetStore(locker BudgetLocker) (BudgetStore, error) {
	if locker == nil {
		return BudgetStore{}, errors.New("platform conformance budget locker is required")
	}
	return BudgetStore{locker: locker}, nil
}

func NewLocalBudgetStore() (BudgetStore, error) {
	locker, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		return BudgetStore{}, fmt.Errorf("create platform conformance budget locker: %w", err)
	}
	return NewBudgetStore(locker)
}

type ReservationRequest struct {
	LedgerPath string
	LedgerID   string
	RunID      string
	ID         string
	Kind       string
	Amount     int64
	Command    string
	Limits     BudgetLimits
}

func DefaultLedgerID(runID string) string { return "ledger-" + runID }

func NewBudgetLedger(ledgerID, runID string, limits BudgetLimits) (BudgetLedger, error) {
	ledger := BudgetLedger{
		Schema: BudgetSchemaV1, LedgerID: ledgerID, RunID: runID,
		Generation: 1, Reservations: []BudgetReservation{}, Limits: limits,
	}
	if err := ledger.validate(); err != nil {
		return BudgetLedger{}, fmt.Errorf("create budget ledger: %w", err)
	}
	return ledger, nil
}

// Reserve validates and durably records one reservation while holding the
// ledger lock. The returned reservation is only an admission token; no child
// process, network request, model call, or download is started here.
func (store BudgetStore) Reserve(ctx context.Context, request ReservationRequest) (BudgetReservation, BudgetLedger, error) {
	if err := validateReservationRequest(request); err != nil {
		return BudgetReservation{}, BudgetLedger{}, err
	}
	if err := rejectSymlinkComponents(request.LedgerPath); err != nil {
		return BudgetReservation{}, BudgetLedger{}, budgetFailure("ledger_path_unsafe", "ledgerPath", "absolute clean non-symlink path", "unsafe path", err)
	}
	lock, err := store.lock(ctx, request.LedgerPath)
	if err != nil {
		return BudgetReservation{}, BudgetLedger{}, err
	}
	defer lock.Close()

	ledger, previousBytes, err := readOrCreateLedger(request)
	if err != nil {
		return BudgetReservation{}, BudgetLedger{}, err
	}
	if err := validateLedgerForRequest(ledger, request); err != nil {
		return BudgetReservation{}, BudgetLedger{}, err
	}
	if ledger.Finalized {
		return BudgetReservation{}, BudgetLedger{}, budgetFailure("ledger_finalized", "finalized", "false", "true", nil)
	}
	for _, existing := range ledger.Reservations {
		if existing.ID == request.ID {
			return BudgetReservation{}, BudgetLedger{}, budgetFailure("reservation_reused", "reservation.id", "new reservation id", request.ID, nil)
		}
	}
	if !hasBudgetCapacity(ledger, request.Kind, request.Amount) {
		return BudgetReservation{}, BudgetLedger{}, budgetFailure("budget_exhausted", "reservation.amount", "amount within remaining budget", fmt.Sprintf("kind=%s amount=%d", request.Kind, request.Amount), nil)
	}

	reservation := BudgetReservation{
		ID: request.ID, Kind: request.Kind, Amount: request.Amount,
		State: ReservationStateReserved, Command: request.Command,
	}
	next := ledger
	next.Reservations = append(append([]BudgetReservation(nil), ledger.Reservations...), reservation)
	next.Generation++
	next.PreviousSHA256 = digestBytes(previousBytes)
	if ledger.Generation == 0 {
		next.Generation = 1
		next.PreviousSHA256 = ""
	}
	if err := next.validate(); err != nil {
		return BudgetReservation{}, BudgetLedger{}, budgetFailure("ledger_mutation_invalid", "ledger", "valid bounded ledger", "invalid reservation mutation", err)
	}
	if err := persistBudgetLedger(request.LedgerPath, next); err != nil {
		return BudgetReservation{}, BudgetLedger{}, budgetFailure("ledger_persist_failed", "ledgerPath", "atomic ledger publication", "write failed", err)
	}
	return reservation, next, nil
}

func (store BudgetStore) Release(ctx context.Context, ledgerPath, runID, reservationID string) (BudgetLedger, error) {
	return store.mutateReservation(ctx, ledgerPath, runID, reservationID, ReservationStateReleased)
}

func (store BudgetStore) Commit(ctx context.Context, ledgerPath, runID, reservationID string) (BudgetLedger, error) {
	return store.mutateReservation(ctx, ledgerPath, runID, reservationID, ReservationStateCommitted)
}

func (store BudgetStore) Finalize(ctx context.Context, ledgerPath, runID string) (BudgetLedger, error) {
	if !validAbsolutePath(ledgerPath) || !validIdentifier(runID) {
		return BudgetLedger{}, budgetFailure("invalid_finalize_request", "ledger", "absolute path and run id", "invalid", nil)
	}
	lock, err := store.lock(ctx, ledgerPath)
	if err != nil {
		return BudgetLedger{}, err
	}
	defer lock.Close()
	ledger, previousBytes, err := readLedgerRequired(ledgerPath)
	if err != nil {
		return BudgetLedger{}, err
	}
	if ledger.RunID != runID {
		return BudgetLedger{}, budgetFailure("ledger_foreign", "ledger.runId", runID, "different run", nil)
	}
	if ledger.Finalized {
		return ledger, nil
	}
	for _, reservation := range ledger.Reservations {
		if reservation.State == ReservationStateReserved {
			return BudgetLedger{}, budgetFailure("reservations_active", "reservations", "no RESERVED entries", "active reservation", nil)
		}
	}
	next := ledger
	next.Finalized = true
	next.Generation++
	next.PreviousSHA256 = digestBytes(previousBytes)
	if err := persistBudgetLedger(ledgerPath, next); err != nil {
		return BudgetLedger{}, budgetFailure("ledger_persist_failed", "ledgerPath", "atomic ledger publication", "write failed", err)
	}
	return next, nil
}

func (store BudgetStore) mutateReservation(ctx context.Context, ledgerPath, runID, reservationID, state string) (BudgetLedger, error) {
	if !validAbsolutePath(ledgerPath) || !validIdentifier(runID) || !validIdentifier(reservationID) {
		return BudgetLedger{}, budgetFailure("invalid_mutation_request", "ledger", "absolute path and safe identifiers", "invalid", nil)
	}
	lock, err := store.lock(ctx, ledgerPath)
	if err != nil {
		return BudgetLedger{}, err
	}
	defer lock.Close()
	ledger, previousBytes, err := readLedgerRequired(ledgerPath)
	if err != nil {
		return BudgetLedger{}, err
	}
	if ledger.RunID != runID {
		return BudgetLedger{}, budgetFailure("ledger_foreign", "ledger.runId", runID, "different run", nil)
	}
	if ledger.Finalized {
		return BudgetLedger{}, budgetFailure("ledger_finalized", "finalized", "false", "true", nil)
	}
	next := ledger
	found := false
	for index := range next.Reservations {
		reservation := &next.Reservations[index]
		if reservation.ID != reservationID {
			continue
		}
		found = true
		if reservation.State != ReservationStateReserved {
			return BudgetLedger{}, budgetFailure("reservation_state", "reservation.state", ReservationStateReserved, reservation.State, nil)
		}
		if state == ReservationStateCommitted {
			// The reservation being committed is no longer an active hold when
			// checking the consumed total. Otherwise the same amount is counted
			// once as a RESERVED hold and again as the committed consumption.
			reservation.State = state
			if !hasBudgetCapacity(next, reservation.Kind, reservation.Amount) {
				return BudgetLedger{}, budgetFailure("budget_exhausted", "consumed", "committed amount within limit", "over limit", nil)
			}
			addConsumed(&next.Consumed, reservation.Kind, reservation.Amount)
		} else {
			reservation.State = state
		}
		break
	}
	if !found {
		return BudgetLedger{}, budgetFailure("reservation_not_found", "reservation.id", "existing RESERVED reservation", reservationID, nil)
	}
	next.Generation++
	next.PreviousSHA256 = digestBytes(previousBytes)
	if err := persistBudgetLedger(ledgerPath, next); err != nil {
		return BudgetLedger{}, budgetFailure("ledger_persist_failed", "ledgerPath", "atomic ledger publication", "write failed", err)
	}
	return next, nil
}

func (store BudgetStore) lock(ctx context.Context, ledgerPath string) (io.Closer, error) {
	if store.locker == nil {
		return nil, budgetFailure("missing_locker", "locker", "cross-process locker", "nil", nil)
	}
	owner, err := store.locker.Lock(ctx, ledgerPath+".lock")
	if err != nil {
		return nil, budgetFailure("lock_failed", "ledgerPath", "lock acquisition", "unavailable", err)
	}
	return owner, nil
}

func readOrCreateLedger(request ReservationRequest) (BudgetLedger, []byte, error) {
	ledger, body, err := readLedgerRequired(request.LedgerPath)
	if err == nil {
		return ledger, body, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return BudgetLedger{}, nil, err
	}
	ledgerID := request.LedgerID
	if ledgerID == "" {
		ledgerID = DefaultLedgerID(request.RunID)
	}
	ledger, err = NewBudgetLedger(ledgerID, request.RunID, request.Limits)
	if err != nil {
		return BudgetLedger{}, nil, budgetFailure("ledger_initialization_invalid", "ledger", "valid initial ledger", "invalid", err)
	}
	// Generation one is represented by the first reservation mutation below;
	// the empty byte slice makes its chain root explicit.
	ledger.Generation = 0
	ledger.PreviousSHA256 = ""
	return ledger, nil, nil
}

func readLedgerRequired(path string) (BudgetLedger, []byte, error) {
	body, err := canonicalBytes(path)
	if err != nil {
		return BudgetLedger{}, nil, fmt.Errorf("read budget ledger bytes: %w", err)
	}
	var ledger BudgetLedger
	if err := decodeCanonical(body, &ledger, MaxJSONBytes); err != nil {
		return BudgetLedger{}, nil, budgetFailure("ledger_corrupt", "ledger", "canonical JSON", "decode failed", err)
	}
	if err := ledger.validate(); err != nil {
		return BudgetLedger{}, nil, budgetFailure("ledger_corrupt", "ledger", "valid bounded ledger", "validation failed", err)
	}
	return ledger, body, nil
}

func validateReservationRequest(request ReservationRequest) error {
	if !validAbsolutePath(request.LedgerPath) {
		return budgetFailure("invalid_request", "ledgerPath", "absolute clean path", "invalid", nil)
	}
	if !validIdentifier(request.RunID) {
		return budgetFailure("invalid_request", "runId", "safe non-empty identifier", request.RunID, nil)
	}
	if request.LedgerID != "" && !validIdentifier(request.LedgerID) {
		return budgetFailure("invalid_request", "ledgerId", "safe non-empty identifier", request.LedgerID, nil)
	}
	if !validIdentifier(request.ID) {
		return budgetFailure("invalid_request", "reservation.id", "safe non-empty identifier", request.ID, nil)
	}
	if !validBudgetKind(request.Kind) || request.Amount <= 0 || strings.TrimSpace(request.Command) == "" {
		return budgetFailure("invalid_request", "reservation", "known kind, positive amount, and command", "invalid reservation", nil)
	}
	if err := validateLedgerLimits(request.Limits); err != nil {
		return budgetFailure("invalid_request", "limits", "bounded ledger limits", "invalid", err)
	}
	return nil
}

func validateLedgerForRequest(ledger BudgetLedger, request ReservationRequest) error {
	if ledger.RunID != request.RunID {
		return budgetFailure("ledger_foreign", "ledger.runId", request.RunID, "different run", nil)
	}
	if request.LedgerID != "" && ledger.LedgerID != request.LedgerID {
		return budgetFailure("ledger_foreign", "ledger.ledgerId", request.LedgerID, "different ledger", nil)
	}
	if !sameLimits(ledger.Limits, request.Limits) {
		return budgetFailure("ledger_drift", "ledger.limits", "limits match reservation request", "different limits", nil)
	}
	return nil
}

func hasBudgetCapacity(ledger BudgetLedger, kind string, amount int64) bool {
	if amount <= 0 {
		return false
	}
	current := ledger.Consumed
	for _, reservation := range ledger.Reservations {
		if reservation.State == ReservationStateReserved {
			if !addConsumedWithinLimit(&current, kind, reservation.Amount, ledger.Limits) {
				return false
			}
		}
	}
	return addConsumedWithinLimit(&current, kind, amount, ledger.Limits)
}

func addConsumedWithinLimit(consumed *BudgetConsumed, kind string, amount int64, limits BudgetLimits) bool {
	if amount <= 0 {
		return false
	}
	var current, limit int64
	switch kind {
	case BudgetKindDownloadBytes:
		current, limit = consumed.DownloadBytes, limits.DownloadBytes
	case BudgetKindModelCalls:
		current, limit = consumed.ModelCalls, limits.ModelCalls
	case BudgetKindNetworkRequests:
		current, limit = consumed.NetworkRequests, limits.NetworkRequests
	case BudgetKindChildProcesses:
		current, limit = consumed.ChildProcesses, limits.MaxChildProcesses
	case BudgetKindTemporaryBytes:
		current, limit = consumed.TemporaryBytes, limits.TemporaryBytes
	default:
		return false
	}
	if current < 0 || amount > limit || current > limit-amount {
		return false
	}
	addConsumed(consumed, kind, amount)
	return true
}

func persistBudgetLedger(path string, ledger BudgetLedger) error {
	body, err := MarshalCanonical(ledger)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, body, nil)
}

// LedgerFileIdentity supplies the redacted identity used by a report after a
// successful reservation or later lifecycle mutation.
func LedgerFileIdentity(path string) (LedgerIdentity, error) {
	body, err := canonicalBytes(path)
	if err != nil {
		return LedgerIdentity{}, fmt.Errorf("read ledger identity: %w", err)
	}
	var ledger BudgetLedger
	if err := decodeCanonical(body, &ledger, MaxJSONBytes); err != nil {
		return LedgerIdentity{}, fmt.Errorf("decode ledger identity: %w", err)
	}
	if err := ledger.validate(); err != nil {
		return LedgerIdentity{}, fmt.Errorf("validate ledger identity: %w", err)
	}
	return LedgerIdentity{PathIdentity: PathIdentity(path), SHA256: digestBytes(body), Generation: ledger.Generation}, nil
}
