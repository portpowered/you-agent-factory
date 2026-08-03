package processlocal_test

import (
	"errors"
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
)

func TestStoreRoundTripsOpaqueEnvelopeByCheckpointID(t *testing.T) {
	t.Parallel()

	store := checkpointrecoverywire.NewProcessLocalCheckpointStore()
	envelope := checkpointrecovery.Envelope{
		CheckpointID:  "checkpoint-1",
		SchemaVersion: 1,
		StrategyKind:  "opaque",
		Payload:       []byte("opaque-payload"),
	}
	if err := store.Put(envelope); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get("checkpoint-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.CheckpointID != envelope.CheckpointID ||
		got.SchemaVersion != envelope.SchemaVersion ||
		got.StrategyKind != envelope.StrategyKind ||
		string(got.Payload) != string(envelope.Payload) {
		t.Fatalf("Get() = %#v, want %#v", got, envelope)
	}

	envelope.Payload[0] = 'X'
	gotAgain, err := store.Get("checkpoint-1")
	if err != nil {
		t.Fatalf("Get() after mutation error = %v", err)
	}
	if string(gotAgain.Payload) != "opaque-payload" {
		t.Fatalf("stored payload alias leaked mutation: %q", gotAgain.Payload)
	}

	got.Payload[0] = 'Y'
	gotOnceMore, err := store.Get("checkpoint-1")
	if err != nil {
		t.Fatalf("Get() after returned-copy mutation error = %v", err)
	}
	if string(gotOnceMore.Payload) != "opaque-payload" {
		t.Fatalf("returned payload alias leaked mutation into stored state: %q", gotOnceMore.Payload)
	}
}

func TestStoreIsolatesIndependentlyConstructedStores(t *testing.T) {
	t.Parallel()

	storeA := checkpointrecoverywire.NewProcessLocalCheckpointStore()
	storeB := checkpointrecoverywire.NewProcessLocalCheckpointStore()

	if err := storeA.Put(checkpointrecovery.Envelope{
		CheckpointID:  "checkpoint-1",
		SchemaVersion: 1,
		StrategyKind:  "opaque",
		Payload:       []byte("store-a-payload"),
	}); err != nil {
		t.Fatalf("storeA.Put() error = %v", err)
	}

	if _, err := storeB.Get("checkpoint-1"); !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("storeB.Get() error = %v, want ErrCheckpointNotFound (store isolation violated)", err)
	}

	if err := storeB.Put(checkpointrecovery.Envelope{
		CheckpointID:  "checkpoint-1",
		SchemaVersion: 1,
		StrategyKind:  "opaque",
		Payload:       []byte("store-b-payload"),
	}); err != nil {
		t.Fatalf("storeB.Put() error = %v", err)
	}

	gotA, err := storeA.Get("checkpoint-1")
	if err != nil {
		t.Fatalf("storeA.Get() error = %v", err)
	}
	if string(gotA.Payload) != "store-a-payload" {
		t.Fatalf("storeA.Get() payload = %q, want unaffected by storeB.Put (store isolation violated)", gotA.Payload)
	}
}

func TestStoreRejectsCorruptEnvelopesOnPut(t *testing.T) {
	t.Parallel()

	store := checkpointrecoverywire.NewProcessLocalCheckpointStore()
	tests := []struct {
		name      string
		envelope  checkpointrecovery.Envelope
		wantError error
	}{
		{
			name:      "empty checkpoint id",
			envelope:  checkpointrecovery.Envelope{SchemaVersion: 1, Payload: []byte("payload")},
			wantError: checkpointrecovery.ErrCorruptCheckpoint,
		},
		{
			name:      "non-positive schema version",
			envelope:  checkpointrecovery.Envelope{CheckpointID: "checkpoint-1", Payload: []byte("payload")},
			wantError: checkpointrecovery.ErrCorruptCheckpoint,
		},
		{
			name:      "empty payload",
			envelope:  checkpointrecovery.Envelope{CheckpointID: "checkpoint-1", SchemaVersion: 1},
			wantError: checkpointrecovery.ErrCorruptCheckpoint,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := store.Put(tc.envelope)
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("Put() error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestStoreReportsMissingCheckpointIdentity(t *testing.T) {
	t.Parallel()

	store := checkpointrecoverywire.NewProcessLocalCheckpointStore()

	_, err := store.Get("")
	if !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Get(\"\") error = %v, want ErrCheckpointNotFound", err)
	}

	_, err = store.Get("missing-checkpoint")
	if !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrCheckpointNotFound", err)
	}
}
