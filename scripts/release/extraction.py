"""Bounded reconciliation of one uniquely named, labelled extraction container."""
import argparse
import json
import re
import sys
import time

from process import run

OWNER_LABEL = "io.optimus.release.extraction"


def reconcile(name, known_created, *, timeout=30, command_timeout=5, poll_interval=0.25,
              clock=time.monotonic, sleep=time.sleep):
    if not re.fullmatch(r"optimus-release-extract-[0-9a-f]{24}", name):
        raise ValueError("refusing an invalid extraction container name")
    deadline = clock() + timeout
    removed = False

    def command(*args):
        remaining = deadline - clock()
        if remaining <= 0:
            raise RuntimeError("reconciliation deadline expired")
        return run("docker", *args, timeout=min(command_timeout, remaining))

    while clock() < deadline:
        try:
            # A successful empty listing establishes absence; an inspect error
            # alone cannot distinguish absence from a daemon/transport failure.
            ids = command("container", "ls", "--all", "--filter", "name=^/" + name + "$", "--quiet", "--no-trunc").splitlines()
            if not ids:
                if known_created or removed:
                    return
                # An uncertain request can still complete after initial absence.
            else:
                if len(ids) != 1 or not re.fullmatch(r"[0-9a-f]{64}", ids[0]):
                    raise ValueError("refusing an ambiguous extraction container listing")
                labels = json.loads(command("container", "inspect", "--format", "{{json .Config.Labels}}", ids[0]))
                if not isinstance(labels, dict) or labels.get(OWNER_LABEL) != name:
                    raise ValueError("refusing extraction container ownership mismatch: " + name)
                # Remove the inspected immutable ID, not a name that could be
                # reassigned to an unrelated container between inspect and rm.
                command("rm", "-f", ids[0])
                removed = True
                # Successful removal crossed the creation point. Confirm absence
                # immediately; no grace period is needed after that evidence.
                continue
        except (RuntimeError, OSError, json.JSONDecodeError):
            # Retry transport failures/timeouts within the same total deadline.
            # run() isolates, kills and reaps each timed-out command group.
            pass
        remaining = deadline - clock()
        if remaining > 0:
            sleep(min(poll_interval, remaining))
    raise RuntimeError("Uncertain extraction cleanup for " + name +
                       ": bounded reconciliation expired without proven cleanup; "
                       "a delayed daemon request may still complete. Inspect the ownership label before removal.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("name")
    parser.add_argument("known_created", choices=("0", "1"))
    args = parser.parse_args()
    try:
        reconcile(args.name, args.known_created == "1")
    except (RuntimeError, ValueError, OSError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
