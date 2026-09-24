package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type intentApplier interface {
	Apply(context.Context, outbox.Record) error
}

const callbackApplyTimeout = 15 * time.Second

// ApplyDurableCallbackIntent is the HTTP-callback variant of ApplyDurableIntent.
// Once a callback has reached the durable write-ahead boundary, a client
// disconnect must not cancel the billing mutation and be misclassified as a
// Redis outage. Keep request-scoped values, detach cancellation/deadline, and
// apply a bounded timeout so a genuinely unhealthy dependency still fails
// closed and remains recoverable through the durable outbox.
func ApplyDurableCallbackIntent(requestCtx context.Context, outboxStore *outbox.Store, applier intentApplier, record outbox.Record) error {
	base := context.Background()
	if requestCtx != nil {
		base = context.WithoutCancel(requestCtx)
	}
	applyCtx, cancel := context.WithTimeout(base, callbackApplyTimeout)
	defer cancel()
	return ApplyDurableIntent(applyCtx, outboxStore, applier, record)
}

// CallbackEventID derives the stable logical billing event ID from the stable
// winner/request ID supplied by the upstream callback. Retries of the same
// callback therefore reuse the same Redis and PostgreSQL idempotency key.
func CallbackEventID(source, format, winnerID string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	format = strings.ToUpper(strings.TrimSpace(format))
	winnerID = strings.TrimSpace(winnerID)
	sum := sha256.Sum256([]byte(source + "\x00" + format + "\x00" + winnerID))
	return "billing:v1:" + hex.EncodeToString(sum[:])
}

// ApplyDurableIntent implements the write-ahead boundary for billing. The
// intent is committed to Bolt before the first external spend mutation. It is
// deleted only after all idempotent side effects have completed successfully.
func ApplyDurableIntent(ctx context.Context, outboxStore *outbox.Store, applier intentApplier, record outbox.Record) error {
	if outboxStore == nil {
		return errors.New("ADV billing outbox is not initialized")
	}
	if applier == nil {
		return errors.New("ADV billing store is not initialized")
	}

	record.Kind = outbox.KindBilling
	requiresRecovery := true
	record.RequiresADVRecovery = &requiresRecovery
	if err := outboxStore.Save(record); err != nil {
		return fmt.Errorf("persist durable ADV billing intent: %w", err)
	}

	if err := applier.Apply(ctx, record); err != nil {
		if updateErr := outboxStore.UpdateFailure(record.EventID, err); updateErr != nil {
			return fmt.Errorf("apply ADV billing intent: %v; persist failure state: %w", err, updateErr)
		}
		return err
	}

	if err := outboxStore.Delete(record.EventID); err != nil {
		return fmt.Errorf("ack completed ADV billing intent: %w", err)
	}
	return nil
}
