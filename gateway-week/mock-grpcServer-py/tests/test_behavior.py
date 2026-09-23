import json
import sys
import threading
import time
import urllib.error
import urllib.request
from concurrent import futures
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import grpc
import pytest
from prometheus_client import CollectorRegistry, generate_latest
from prometheus_client.parser import text_string_to_metric_families

from api.nafmock.v1 import nafmock_pb2, nafmock_pb2_grpc
from nafmock.admin import make_admin
from nafmock.servicer import Servicer

OK = grpc.StatusCode.OK
UNAVAILABLE = grpc.StatusCode.UNAVAILABLE
FAILED_PRECONDITION = grpc.StatusCode.FAILED_PRECONDITION
DEADLINE_EXCEEDED = grpc.StatusCode.DEADLINE_EXCEEDED
INVALID_ARGUMENT = grpc.StatusCode.INVALID_ARGUMENT


class Harness:
    def __init__(self, default_behavior):
        self.registry = CollectorRegistry()
        self.servicer = Servicer(
            registry=self.registry,
            default_behavior=default_behavior,
            history_limit=100,
        )
        self.server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
        nafmock_pb2_grpc.add_NAFMockServicer_to_server(self.servicer, self.server)
        self.grpc_port = self.server.add_insecure_port("localhost:0")
        self.server.start()
        self.channel = grpc.insecure_channel(f"localhost:{self.grpc_port}")
        self.stub = nafmock_pb2_grpc.NAFMockStub(self.channel)
        self.admin = make_admin(self.servicer, self.registry, 0, host="127.0.0.1")
        threading.Thread(target=self.admin.serve_forever, daemon=True).start()
        self.base = f"http://127.0.0.1:{self.admin.server_address[1]}"

    def close(self):
        self.channel.close()
        self.server.stop(0).wait()
        self.admin.shutdown()
        self.admin.server_close()

    def call(self, operation="naf.test-op", payload="payload-stable",
             behavior_override=None, correlation_id=None, timeout=None):
        metadata = []
        if behavior_override is not None:
            metadata.append(("nafmock-behavior", behavior_override))
        if correlation_id is not None:
            metadata.append(("x-correlation-id", correlation_id))
        request = nafmock_pb2.ApplyOperationRequest(
            operation=operation, payload=payload.encode())
        return self.stub.ApplyOperation(
            request, timeout=timeout, metadata=tuple(metadata))

    def call_code(self, **kwargs):
        try:
            self.call(**kwargs)
            return OK
        except grpc.RpcError as exc:
            return exc.code()

    def executions_for(self, operation):
        text = generate_latest(self.registry).decode()
        for family in text_string_to_metric_families(text):
            for sample in family.samples:
                if (sample.name == "nafmock_executions_total"
                        and sample.labels.get("operation") == operation):
                    return sample.value
        return 0.0

    def wait_for_history(self, count, timeout=2.0):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            entries = self.history()
            if len(entries) >= count:
                return entries
            time.sleep(0.02)
        return self.history()

    def history(self):
        with urllib.request.urlopen(self.base + "/history") as resp:
            return json.loads(resp.read())

    def http(self, method, path, body=None):
        req = urllib.request.Request(self.base + path, data=body, method=method)
        try:
            with urllib.request.urlopen(req) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as exc:
            return exc.code, exc.read()


CASES = [
    ("ok", None, None, [OK], True, 1.0),
    ("fail-retryable", None, None, [UNAVAILABLE], False, 0.0),
    ("fail-nonretryable", None, None, [FAILED_PRECONDITION], False, 0.0),
    ("flaky:2", None, None, [UNAVAILABLE, UNAVAILABLE, OK], True, 1.0),
    ("delay:500ms,ok", None, 0.1, [DEADLINE_EXCEEDED], False, 0.0),
    ("hang", None, 0.15, [DEADLINE_EXCEEDED], False, 0.0),
    ("succeed-then-hang", None, 0.15, [DEADLINE_EXCEEDED], True, 1.0),
    ("ok", "fail-nonretryable", None, [FAILED_PRECONDITION], False, 0.0),
]

IDS = ["ok", "fail-retryable", "fail-nonretryable", "flaky-then-ok",
       "delay-deadline", "hang", "succeed-then-hang", "override-beats-default"]


@pytest.mark.parametrize("case", CASES, ids=IDS)
def test_matrix(case):
    default, override, timeout, want_codes, want_executed, want_exec_total = case
    h = Harness(default)
    try:
        for want_code in want_codes:
            got = h.call_code(behavior_override=override, timeout=timeout)
            assert got == want_code, f"want {want_code}, got {got}"
        entries = h.wait_for_history(len(want_codes))
        assert h.executions_for("naf.test-op") == want_exec_total
        assert len(entries) == len(want_codes)
        assert entries[-1]["executed"] == want_executed
    finally:
        h.close()


def test_correlation_id_recorded():
    h = Harness("ok")
    try:
        h.call(correlation_id="corr-42")
        entries = h.wait_for_history(1)
        assert len(entries) == 1 and entries[0]["correlation_id"] == "corr-42"
    finally:
        h.close()


def test_flaky_keyed_by_payload():
    h = Harness("flaky:1")
    try:
        steps = [("payload-a", UNAVAILABLE), ("payload-a", OK),
                 ("payload-b", UNAVAILABLE), ("payload-b", OK)]
        for payload, want in steps:
            got = h.call_code(payload=payload)
            assert got == want, f"{payload}: want {want}, got {got}"
        h.wait_for_history(len(steps))
        assert h.executions_for("naf.test-op") == 2.0
    finally:
        h.close()


def test_admin_behavior_change():
    h = Harness("ok")
    try:
        assert h.call_code() == OK
        status, _ = h.http("PUT", "/behavior", b"fail-retryable")
        assert status == 200
        assert h.call_code() == UNAVAILABLE
    finally:
        h.close()


def test_invalid_behavior_rejected():
    h = Harness("ok")
    try:
        assert h.call_code(behavior_override="explode") == INVALID_ARGUMENT
        status, _ = h.http("PUT", "/behavior", b"nope")
        assert status == 400
    finally:
        h.close()


def test_reset_clears_state():
    h = Harness("ok")
    try:
        assert h.call_code(payload="payload-a") == OK
        h.wait_for_history(1)
        assert h.executions_for("naf.test-op") == 1.0
        status, _ = h.http("POST", "/reset")
        assert status == 200
        assert h.executions_for("naf.test-op") == 0.0
        assert h.history() == []
    finally:
        h.close()