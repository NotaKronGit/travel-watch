package gemini

import (
	"context"
	"errors"
	"fmt"
	"github.com/NotaKronGit/travel-watch/services/search/internal/evaluation"
	"github.com/NotaKronGit/travel-watch/services/search/internal/journeys"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProviderDiagnosticInReport(t *testing.T) {
	p := testPlanner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = fmt.Fprint(w, `{"error":{"code":403,"status":"PERMISSION_DENIED","message":"Access denied for test-secret\nretry","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_SERVICE_BLOCKED","metadata":{"secret":"never-log-me"}}]}}`)
	})
	reports, err := evaluation.Run(context.Background(), p, journeys.Limits{Routes: testLimits, MaxOffers: 100, MaxTransfers: 100, MaxCombinations: 1000, MaxJourneys: 20}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reports {
		msg := r.Alternative.Error
		for _, want := range []string{"HTTP 403", "code=403", "PERMISSION_DENIED", "API_KEY_SERVICE_BLOCKED", "Access denied"} {
			if !strings.Contains(msg, want) {
				t.Fatal("diagnostic missing", want)
			}
		}
		for _, bad := range []string{"test-secret", "never-log-me", "\n"} {
			if strings.Contains(msg, bad) {
				t.Fatal("unsafe report")
			}
		}
	}
}
func TestSanitize(t *testing.T) {
	got := sanitize("key=a%2Bb a+b\nBearer other-secret\x1b", "a+b", 1000)
	for _, bad := range []string{"a%2Bb", "a+b", "other-secret", "\n", "\x1b"} {
		if strings.Contains(got, bad) {
			t.Fatal("secret or control leaked")
		}
	}
	if len([]rune(sanitize(strings.Repeat("я", 2000), "", 1000))) != 1001 {
		t.Fatal("limit ignored")
	}
	if strings.Contains(providerError(503, []byte("<html>test-secret</html>"), "test-secret").Error(), "test-secret") {
		t.Fatal("raw body leaked")
	}
}
func TestOperationDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"}, {context.Canceled, "cancelled"},
		{&net.DNSError{Name: "secret", Err: "hidden"}, "dns_error"},
		{&net.OpError{Op: "dial", Err: errors.New("secret")}, "connect_error"},
		{errors.New("secret"), "transport_error"},
	} {
		got := operationError(context.Background(), tc.err, "response_body").Error()
		if !strings.Contains(got, tc.want) || !strings.Contains(got, "response_body") || strings.Contains(got, "secret") {
			t.Fatal(got)
		}
	}
}
