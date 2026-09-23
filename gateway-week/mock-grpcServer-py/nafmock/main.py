import argparse
import json
import logging
import signal
import sys
import threading
from concurrent import futures

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc
from grpc_reflection.v1alpha import reflection
from prometheus_client import CollectorRegistry

from api.nafmock.v1 import nafmock_pb2, nafmock_pb2_grpc
from nafmock.admin import make_admin
from nafmock.servicer import Servicer


class JsonFormatter(logging.Formatter):
    def format(self, record):
        return json.dumps({
            "time": self.formatTime(record),
            "level": record.levelname,
            "msg": record.getMessage(),
            **getattr(record, "fields", {}),
        })


def setup_logging():
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter())
    logging.basicConfig(level=logging.INFO, handlers=[handler])


def port_of(addr):
    return int(addr.rsplit(":", 1)[1])


def main():
    parser = argparse.ArgumentParser(description="mock NAF/TFS platform")
    parser.add_argument("--grpc-addr", default=":50052")
    parser.add_argument("--http-addr", default=":9091")
    parser.add_argument("--behavior", default="ok")
    parser.add_argument("--history-limit", type=int, default=100)
    args = parser.parse_args()

    setup_logging()
    log = logging.getLogger("nafmock")

    registry = CollectorRegistry()
    try:
        servicer = Servicer(
            registry=registry,
            default_behavior=args.behavior,
            history_limit=args.history_limit,
        )
    except ValueError as exc:
        log.error("invalid default behavior", extra={"fields": {"err": str(exc)}})
        sys.exit(1)

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=16))
    nafmock_pb2_grpc.add_NAFMockServicer_to_server(servicer, server)
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    reflection.enable_server_reflection(
        ("nafmock.v1.NAFMock", reflection.SERVICE_NAME), server)

    grpc_port = port_of(args.grpc_addr)
    server.add_insecure_port(f"[::]:{grpc_port}")
    admin = make_admin(servicer, registry, port_of(args.http_addr))
    threading.Thread(target=admin.serve_forever, daemon=True).start()

    log.info("admin http listening", extra={"fields": {"addr": args.http_addr}})
    server.start()
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    log.info("nafmock listening", extra={"fields": {
        "grpc_addr": args.grpc_addr, "behavior": args.behavior}})

    stopped = threading.Event()

    def shutdown(signum, frame):
        if stopped.is_set():
            return
        stopped.set()
        log.info("shutting down")
        server.stop(5)
        admin.shutdown()

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    server.wait_for_termination()
    log.info("stopped")


if __name__ == "__main__":
    main()