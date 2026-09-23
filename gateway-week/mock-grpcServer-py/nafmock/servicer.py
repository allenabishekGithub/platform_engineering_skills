import hashlib
import logging
import secrets
import threading
import time
from collections import deque
from datetime import datetime, timedelta, timezone

import grpc
from prometheus_client import CollectorRegistry, Counter, Histogram

from api.nafmock.v1 import nafmock_pb2, nafmock_pb2_grpc
from nafmock.behavior import FAIL_NONRETRYABLE, FAIL_RETRYABLE, parse_behavior

BEHAVIOR_METADATA_KEY = "nafmock-behavior"
CORRELATION_ID_KEY = "x-correlation-id"

_CAMEL = {
    grpc.StatusCode.OK: "OK",
    grpc.StatusCode.INVALID_ARGUMENT: "InvalidArgument",
    grpc.StatusCode.UNAVAILABLE: "Unavailable",
    grpc.StatusCode.FAILED_PRECONDITION: "FailedPrecondition",
    grpc.StatusCode.DEADLINE_EXCEEDED: "DeadlineExceeded",
    grpc.StatusCode.CANCELLED: "Canceled",
    grpc.StatusCode.INTERNAL: "Internal",
}

_POLL_INTERVAL = 0.02


def camel(code):
    return _CAMEL.get(code, code.name)


class Servicer(nafmock_pb2_grpc.NAFMockServicer):
    def __init__(self, registry=None, default_behavior="ok", history_limit=100):
        self._registry = registry or CollectorRegistry()
        try:
            self._default = parse_behavior(default_behavior)
        except ValueError as exc:
            raise ValueError(f"default behavior: {exc}") from exc
        self._lock = threading.Lock()
        self._flaky_calls = {}
        self._history = deque(maxlen=history_limit)
        self.executions = Counter(
            "nafmock_executions", "Side effects actually applied by the mock.",
            ["operation"], registry=self._registry)
        self.requests = Counter(
            "nafmock_requests", "Requests received by the mock, by outcome code.",
            ["code", "operation"], registry=self._registry)
        self.duration = Histogram(
            "nafmock_request_duration_seconds", "Time spent handling ApplyOperation.",
            registry=self._registry)
        self.log = logging.getLogger("nafmock")

    def registry(self):
        return self._registry

    def ApplyOperation(self, request, context):
        start = time.monotonic()
        behavior, corr_id, override_error = self._resolve(context)
        response = None
        executed = False
        code = grpc.StatusCode.OK
        details = ""
        if override_error is not None:
            code = grpc.StatusCode.INVALID_ARGUMENT
            details = override_error
            behavior_raw = behavior.raw if behavior is not None else "-"
        else:
            behavior_raw = behavior.raw
            response, code, details, executed = self._apply(request, context, behavior)
        elapsed = time.monotonic() - start
        self._record({
            "time": datetime.now(timezone.utc).isoformat(),
            "correlation_id": corr_id,
            "operation": request.operation,
            "behavior": behavior_raw,
            "code": camel(code),
            "executed": executed,
        })
        self.requests.labels(camel(code), request.operation).inc()
        self.duration.observe(elapsed)
        self.log.info("apply_operation", extra={"fields": {
            "correlation_id": corr_id,
            "operation": request.operation,
            "behavior": behavior_raw,
            "code": camel(code),
            "executed": executed,
        }})
        if code != grpc.StatusCode.OK:
            context.abort(code, details or f'nafmock behavior "{behavior_raw}"')
        return response

    def _apply(self, request, context, behavior):
        operation = request.operation
        if behavior.delay.total_seconds() > 0:
            if not _wait_for(context, behavior.delay.total_seconds()):
                return None, _dead_code(context), "", False
        if behavior.fail == FAIL_RETRYABLE:
            return None, grpc.StatusCode.UNAVAILABLE, "", False
        if behavior.fail == FAIL_NONRETRYABLE:
            return None, grpc.StatusCode.FAILED_PRECONDITION, "", False
        if behavior.hang:
            _wait_for(context, None)
            return None, _dead_code(context), "", False
        if behavior.flaky_count > 0:
            key = _flaky_key(request)
            with self._lock:
                self._flaky_calls[key] = self._flaky_calls.get(key, 0) + 1
                calls = self._flaky_calls[key]
            if calls <= behavior.flaky_count:
                return None, grpc.StatusCode.UNAVAILABLE, "", False
        if behavior.succeed_then_hang:
            self.executions.labels(operation).inc()
            _wait_for(context, None)
            return None, _dead_code(context), "", True
        self.executions.labels(operation).inc()
        return nafmock_pb2.ApplyOperationResponse(
            handle="naf-" + secrets.token_hex(8),
            applied_at=datetime.now(timezone.utc).isoformat(),
            echo=request.payload,
        ), grpc.StatusCode.OK, "", True

    def _resolve(self, context):
        corr_id = ""
        md = dict(context.invocation_metadata())
        corr_id = md.get(CORRELATION_ID_KEY, "")
        override = md.get(BEHAVIOR_METADATA_KEY)
        if override is not None:
            try:
                return parse_behavior(override), corr_id, None
            except ValueError as exc:
                return None, corr_id, f"nafmock-behavior: {exc}"
        with self._lock:
            default = self._default
        return default, corr_id, None

    def _record(self, entry):
        with self._lock:
            self._history.append(entry)

    def history(self):
        with self._lock:
            return list(self._history)

    def default_behavior(self):
        with self._lock:
            return self._default.raw

    def set_default_behavior(self, raw):
        behavior = parse_behavior(raw)
        with self._lock:
            self._default = behavior

    def reset(self):
        with self._lock:
            self._flaky_calls = {}
            self._history.clear()
        self.executions.clear()
        self.requests.clear()


def _wait_for(context, seconds):
    if seconds is None:
        while context.is_active():
            time.sleep(_POLL_INTERVAL)
        return False
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if not context.is_active():
            return False
        time.sleep(min(_POLL_INTERVAL, deadline - time.monotonic()))
    return True


def _dead_code(context):
    remaining = context.time_remaining()
    if remaining is not None and remaining <= 0:
        return grpc.StatusCode.DEADLINE_EXCEEDED
    return grpc.StatusCode.CANCELLED


def _flaky_key(request):
    digest = hashlib.sha256(request.payload).hexdigest()
    return request.operation.lower() + "/" + digest