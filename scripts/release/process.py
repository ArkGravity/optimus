"""Run owned commands outside the caller's process group, with bounded teardown."""
import os
import signal
import subprocess
import sys


class CommandFailed(RuntimeError):
    def __init__(self, args, returncode):
        super().__init__("smoke command failed: " + " ".join(args[:3]))
        self.returncode = returncode


def run(*args, cwd=None, env=None, timeout=180):
    # A process-group signal must not kill cleanup CLIs while their Python
    # caller defers termination. setsid(2) is portable on macOS and Linux; no
    # external setsid utility or inherited CLI signal behavior is required.
    with subprocess.Popen(args, cwd=cwd, env=env, start_new_session=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) as process:
        try:
            output, _error = process.communicate(timeout=timeout)
        except BaseException as error:
            # Kill and reap the owned command, including descendants still in
            # its group, before returning or unwinding into resource cleanup.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.communicate()
            if isinstance(error, subprocess.TimeoutExpired):
                raise RuntimeError("smoke command timed out: " + " ".join(args[:3])) from None
            raise
        if process.returncode:
            raise CommandFailed(args, process.returncode)
        return output.strip()


if __name__ == "__main__":
    try:
        run(*sys.argv[1:])
    except CommandFailed as error:
        print(str(error), file=sys.stderr)
        sys.exit(error.returncode if error.returncode >= 0 else 128 - error.returncode)
