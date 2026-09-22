package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	nafmockv1 "github.com/allenabishekGithub/platform_engineering_skills/gateway-week/mock-grpcServer/api/nafmock/v1"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	BehaviorMetadataKey = "nafmock-behavior"
	CorrelationIDKey    = "x-correlation-id"
)

type HistoryEntry struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
	Operation     string `json:"operation"`
	Behavior      string `json:"behavior"`
	Code          string `json:"code"`
	Executed      bool   `json:"executed"`
}

type Server struct {
	nafmockv1.UnimplementedNAFMockServer

	mu              sync.Mutex
	defaultBehavior Behavior
	flakyCalls      map[string]int
	history         []HistoryEntry
	historyLimit    int

	executions *prometheus.CounterVec
	requests   *prometheus.CounterVec
	duration   prometheus.Histogram
}

func New(reg *prometheus.Registry, defaultBehavior string, historyLimit int) (*Server, error) {
	b, err := ParseBehavior(defaultBehavior)
	if err != nil {
		return nil, fmt.Errorf("default behavior: %w", err)
	}
	s := &Server{
		defaultBehavior: b,
		flakyCalls:     map[string]int{},
		historyLimit:   historyLimit,
		executions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nafmock_executions_total",
			Help: "Side effects actually applied by the mock platform.",
		}, []string{"operation"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nafmock_requests_total",
			Help: "Requests received by the mock, by outcome code.",
		}, []string{"code", "operation"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "nafmock_request_duration_seconds",
			Help: "Time spent handling ApplyOperation.",
		}),
	}
	reg.MustRegister(s.executions, s.requests, s.duration)
	return s, nil
}

func (s *Server) ApplyOperation(ctx context.Context, req *nafmockv1.ApplyOperationRequest) (*nafmockv1.ApplyOperationResponse, error) {
	start := time.Now()
	behavior, corrID, err := s.resolveBehavior(ctx)
	if err != nil {
		s.record(HistoryEntry{Time: start.Format(time.RFC3339Nano), CorrelationID: corrID, Operation: req.GetOperation(), Behavior: "-", Code: codes.InvalidArgument.String()})
		s.requests.WithLabelValues(codes.InvalidArgument.String(), req.GetOperation()).Inc()
		return nil, err
	}

	resp, code, executed := s.apply(ctx, req, behavior, corrID)

	s.record(HistoryEntry{
		Time:          start.Format(time.RFC3339Nano),
		CorrelationID: corrID,
		Operation:     req.GetOperation(),
		Behavior:      behavior.Raw,
		Code:          code.String(),
		Executed:      executed,
	})
	s.requests.WithLabelValues(code.String(), req.GetOperation()).Inc()
	s.duration.Observe(time.Since(start).Seconds())
	slog.InfoContext(ctx, "apply_operation",
		"correlation_id", corrID,
		"operation", req.GetOperation(),
		"behavior", behavior.Raw,
		"code", code.String(),
		"executed", executed,
	)
	if code != codes.OK {
		return nil, status.Error(code, fmt.Sprintf("nafmock behavior %q", behavior.Raw))
	}
	return resp, nil
}

func (s *Server) apply(ctx context.Context, req *nafmockv1.ApplyOperationRequest, b Behavior, corrID string) (*nafmockv1.ApplyOperationResponse, codes.Code, bool) {
	if b.Delay > 0 {
		if err := waitCtx(ctx, b.Delay); err != nil {
			return nil, codeFromCtxErr(err), false
		}
	}
	switch b.Fail {
	case failRetryable:
		return nil, codes.Unavailable, false
	case failNonRetryable:
		return nil, codes.FailedPrecondition, false
	}
	if b.Hang {
		if err := waitCtx(ctx, 0); err != nil {
			return nil, codeFromCtxErr(err), false
		}
		return nil, codes.Canceled, false
	}
	if b.FlakyCount > 0 {
		key := flakyKey(req)
		s.mu.Lock()
		s.flakyCalls[key]++
		calls := s.flakyCalls[key]
		s.mu.Unlock()
		if calls <= b.FlakyCount {
			return nil, codes.Unavailable, false
		}
	}
	if b.SucceedThenHang {
		s.executions.WithLabelValues(req.GetOperation()).Inc()
		if err := waitCtx(ctx, 0); err != nil {
			return nil, codeFromCtxErr(err), true
		}
		return nil, codes.Canceled, true
	}
	s.executions.WithLabelValues(req.GetOperation()).Inc()
	handle, err := newHandle()
	if err != nil {
		return nil, codes.Internal, false
	}
	return &nafmockv1.ApplyOperationResponse{
		Handle:    handle,
		AppliedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Echo:      req.GetPayload(),
	}, codes.OK, true
}

func (s *Server) resolveBehavior(ctx context.Context) (Behavior, string, error) {
	corrID := ""
	behavior := s.DefaultBehavior()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(CorrelationIDKey); len(v) > 0 {
			corrID = v[0]
		}
		if v := md.Get(BehaviorMetadataKey); len(v) > 0 {
			b, err := ParseBehavior(v[0])
			if err != nil {
				return Behavior{Raw: v[0]}, corrID, status.Errorf(codes.InvalidArgument, "nafmock-behavior: %v", err)
			}
			return b, corrID, nil
		}
	}
	return behavior, corrID, nil
}

func (s *Server) record(e HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, e)
	if len(s.history) > s.historyLimit {
		s.history = s.history[len(s.history)-s.historyLimit:]
	}
}

func (s *Server) DefaultBehavior() Behavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaultBehavior
}

func (s *Server) SetDefaultBehavior(raw string) error {
	b, err := ParseBehavior(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.defaultBehavior = b
	s.mu.Unlock()
	return nil
}

func (s *Server) History() []HistoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HistoryEntry, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flakyCalls = map[string]int{}
	s.history = nil
}

func (s *Server) ResetCounters() {
	s.executions.Reset()
	s.requests.Reset()
}

func waitCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func codeFromCtxErr(err error) codes.Code {
	if err == context.DeadlineExceeded {
		return codes.DeadlineExceeded
	}
	return codes.Canceled
}

func flakyKey(req *nafmockv1.ApplyOperationRequest) string {
	h := sha256.Sum256(req.GetPayload())
	return strings.ToLower(req.GetOperation()) + "/" + hex.EncodeToString(h[:])
}

func newHandle() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "naf-" + hex.EncodeToString(b), nil
}
