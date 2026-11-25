package controller

import (
	"net/http"
	"testing"
)

func TestEventCompletenessWindowValidation(t *testing.T) {
	newTestDB(t)
	expectStatus(t, call(EventCompleteness, testRequest{target: "/api/metric/events/completeness"}), http.StatusOK)
	for _, query := range []string{"start_timestamp=bad", "start_timestamp=1", "end_timestamp=1"} {
		expectStatus(t, call(EventCompleteness, testRequest{target: "/api/metric/events/completeness?" + query}), http.StatusBadRequest)
	}
}
