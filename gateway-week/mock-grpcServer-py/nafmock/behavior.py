import re
from dataclasses import dataclass
from datetime import timedelta

FAIL_NONE = ""
FAIL_RETRYABLE = "retryable"
FAIL_NONRETRYABLE = "nonretryable"

_DURATION = re.compile(r"^(\d+(?:\.\d+)?)(ms|s|m|h)$")
_UNIT_SECONDS = {"ms": 1 / 1000, "s": 1, "m": 60, "h": 3600}


@dataclass
class Behavior:
    raw: str = "ok"
    delay: timedelta = timedelta(0)
    flaky_count: int = 0
    fail: str = FAIL_NONE
    hang: bool = False
    succeed_then_hang: bool = False


def parse_duration(text):
    match = _DURATION.match(text)
    if not match:
        raise ValueError(f"bad delay {text!r}")
    return timedelta(seconds=float(match.group(1)) * _UNIT_SECONDS[match.group(2)])


def parse_behavior(raw):
    behavior = Behavior(raw=raw)
    stripped = raw.strip() if raw else ""
    if not stripped:
        return behavior
    for token in (t.strip() for t in stripped.split(",")):
        if not token:
            raise ValueError("empty behavior token")
        if token == "ok":
            continue
        if token == "hang":
            behavior.hang = True
        elif token == "succeed-then-hang":
            behavior.succeed_then_hang = True
        elif token == "fail-retryable":
            behavior.fail = FAIL_RETRYABLE
        elif token == "fail-nonretryable":
            behavior.fail = FAIL_NONRETRYABLE
        elif token.startswith("flaky:"):
            count = token[len("flaky:"):]
            if not count.isdigit():
                raise ValueError(f"bad flaky count {token!r}")
            behavior.flaky_count = int(count)
        elif token.startswith("delay:"):
            behavior.delay = parse_duration(token[len("delay:"):])
        else:
            raise ValueError(f"unknown behavior token {token!r}")
    if behavior.hang and behavior.succeed_then_hang:
        raise ValueError("hang and succeed-then-hang are mutually exclusive")
    return behavior