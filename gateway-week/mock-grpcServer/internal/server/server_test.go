package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nafmockv1 "github.com/allenabishekGithub/platform_engineering_skills/gateway-week/mock-grpcServer/api/nafmock/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type harness struct {
	srv   *Server
	conn  *grpc.ClientConn
	admin *httptest.Server
}

func newHarness(t *testing.T, defaultBehavior string) *harness {
	t.Helper()
	reg := prometheus.NewRegistry()
	srv, err := New(reg, defaultBehavior, 100)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lis := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	nafmockv1.RegisterNAFMockServer(grpcServer, srv)
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	admin := httptest.NewServer(srv.AdminMux())
	t.Cleanup(admin.Close)

	return &harness{srv: srv, conn: conn, admin: admin}
}

func (h *harness) call(t *testing.T, ctx context.Context, op string, payload string, behaviorOverride string) (*nafmockv1.ApplyOperationResponse, error) {
	t.Helper()
	if behaviorOverride != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, BehaviorMetadataKey, behaviorOverride)
	}
	return nafmockv1.NewNAFMockClient(h.conn).ApplyOperation(ctx, &nafmockv1.ApplyOperationRequest{
		Operation: op,
		Payload:   []byte(payload),
	})
}

func (h *harness) executionsFor(t *testing.T, op string) float64 {
	t.Helper()
	return testutil.ToFloat64(h.srv.executions.WithLabelValues(op))
}

func (h *harness) history(t *testing.T) []HistoryEntry {
	t.Helper()
	resp, err := http.Get(h.admin.URL + "/history")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	defer resp.Body.Close()
	var entries []HistoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return entries
}

func TestBehaviorMatrix(t *testing.T) {
	tests := []struct {
		name          string
		def           string
		override      string
		ctxTimeout    time.Duration
		wantCodes     []codes.Code
		wantExecuted  bool
		wantExecTotal float64
	}{
		{
			name:          "ok succeeds and executes",
def:       "ok",
			wantCodes:     []codes.Code{codes.OK},
			wantExecuted:  true,
			wantExecTotal: 1,
		},
		{
			name:          "fail-retryable returns unavailable without executing",
def:       "fail-retryable",
			wantCodes:     []codes.Code{codes.Unavailable},
			wantExecuted:  false,
			wantExecTotal: 0,
		},
		{
			name:          "fail-nonretryable returns failed_precondition without executing",
def:       "fail-nonretryable",
			wantCodes:     []codes.Code{codes.FailedPrecondition},
			wantExecuted:  false,
			wantExecTotal: 0,
		},
		{
			name:          "flaky fails twice then succeeds on third attempt",
def:       "flaky:2",
			wantCodes:     []codes.Code{codes.Unavailable, codes.Unavailable, codes.OK},
			wantExecuted:  true,
			wantExecTotal: 1,
		},
		{
			name:          "delay longer than deadline gives deadline_exceeded",
def:       "delay:500ms,ok",
			ctxTimeout:    100 * time.Millisecond,
			wantCodes:     []codes.Code{codes.DeadlineExceeded},
			wantExecuted:  false,
			wantExecTotal: 0,
		},
		{
			name:          "hang until deadline",
def:       "hang",
			ctxTimeout:    150 * time.Millisecond,
			wantCodes:     []codes.Code{codes.DeadlineExceeded},
			wantExecuted:  false,
			wantExecTotal: 0,
		},
		{
			name:          "succeed-then-hang executes but client sees deadline",
def:       "succeed-then-hang",
			ctxTimeout:    150 * time.Millisecond,
			wantCodes:     []codes.Code{codes.DeadlineExceeded},
			wantExecuted:  true,
			wantExecTotal: 1,
		},
		{
			name:          "metadata override beats default",
def:       "ok",
			override:      "fail-nonretryable",
			wantCodes:     []codes.Code{codes.FailedPrecondition},
			wantExecuted:  false,
			wantExecTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.def)
			for i, wantCode := range tt.wantCodes {
				ctx := context.Background()
				if tt.ctxTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
					defer cancel()
				}
				_, err := h.call(t, ctx, "naf.test-op", "payload-stable", tt.override)
				gotCode := codes.OK
				if err != nil {
					gotCode = status.Code(err)
				}
				if gotCode != wantCode {
					t.Fatalf("attempt %d: code = %v, want %v (err %v)", i+1, gotCode, wantCode, err)
				}
			}
			exec := h.executionsFor(t, "naf.test-op")
			if exec != tt.wantExecTotal {
				t.Errorf("executions = %v, want %v", exec, tt.wantExecTotal)
			}
			entries := h.history(t)
			last := entries[len(entries)-1]
			if last.Executed != tt.wantExecuted {
				t.Errorf("history executed = %v, want %v", last.Executed, tt.wantExecuted)
			}
		})
	}
}

func TestCorrelationIDRecorded(t *testing.T) {
	h := newHarness(t, "ok")
	ctx := metadata.AppendToOutgoingContext(context.Background(), CorrelationIDKey, "corr-42")
	if _, err := h.call(t, ctx, "naf.test-op", "p", ""); err != nil {
		t.Fatalf("call: %v", err)
	}
	entries := h.history(t)
	if len(entries) != 1 || entries[0].CorrelationID != "corr-42" {
		t.Fatalf("history = %+v, want correlation_id corr-42", entries)
	}
}

func TestFlakyCounterKeyedByPayload(t *testing.T) {
	h := newHarness(t, "flaky:1")
	ctx := context.Background()
	steps := []struct {
		payload   string
		wantCode  codes.Code
	}{
		{"payload-a", codes.Unavailable},
		{"payload-a", codes.OK},
		{"payload-b", codes.Unavailable},
		{"payload-b", codes.OK},
	}
	for i, s := range steps {
		_, err := h.call(t, ctx, "naf.test-op", s.payload, "")
		if got := status.Code(err); got != s.wantCode {
			t.Fatalf("step %d (%s): code = %v, want %v (err %v)", i+1, s.payload, got, s.wantCode, err)
		}
	}
	if got := h.executionsFor(t, "naf.test-op"); got != 2 {
		t.Errorf("executions = %v, want 2", got)
	}
}

func TestAdminBehaviorChange(t *testing.T) {
	h := newHarness(t, "ok")
	if _, err := h.call(t, context.Background(), "naf.test-op", "p", ""); err != nil {
		t.Fatalf("call with ok: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, h.admin.URL+"/behavior", strings.NewReader("fail-retryable"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := http.DefaultClient.Do(req); err != nil {
		t.Fatalf("PUT behavior: %v", err)
	}
	if _, err := h.call(t, context.Background(), "naf.test-op", "p", ""); status.Code(err) != codes.Unavailable {
		t.Fatalf("call after PUT: want unavailable, got %v", err)
	}
}

func TestInvalidBehaviorRejected(t *testing.T) {
	h := newHarness(t, "ok")
	if _, err := h.call(t, context.Background(), "naf.test-op", "p", "explode"); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid override: want invalid argument, got %v", err)
	}
	req, _ := http.NewRequest(http.MethodPut, h.admin.URL+"/behavior", strings.NewReader("nope"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT invalid behavior: status %d, want 400", resp.StatusCode)
	}
}
