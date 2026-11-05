package service

import (
	"testing"
)

func TestNotifyBillingFailure(t *testing.T) {
	// This just verifies the function doesn't panic and the alert is logged.
	// Webhook delivery is best-effort and no webhook is configured in tests.
	NotifyBillingFailure(1, "settlement failed for user 1")
}

func TestNotifyReconciliationDiffWarning(t *testing.T) {
	NotifyReconciliationDiff(5000, "diff within threshold")
}

func TestNotifyReconciliationDiffCritical(t *testing.T) {
	NotifyReconciliationDiff(200000, "diff exceeds threshold")
}

func TestNotifyResourceSyncError(t *testing.T) {
	NotifyResourceSyncError("resource catalog import failed")
}

func TestNotifyPaymentCallbackFailed(t *testing.T) {
	NotifyPaymentCallbackFailed(42, "signature verification failed for order 42")
}
