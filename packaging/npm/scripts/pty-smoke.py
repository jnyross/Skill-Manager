#!/usr/bin/env python3
"""Start Skillet on a real pseudo-terminal, send q, and require a clean exit."""

import os
import pty
import select
import signal
import sys
import time


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: pty-smoke.py /path/to/skillet", file=sys.stderr)
        return 2

    pid, fd = pty.fork()
    if pid == 0:
        os.execv(sys.argv[1], [sys.argv[1]])

    output = bytearray()
    deadline = time.monotonic() + 10
    os.write(fd, b"q")
    while time.monotonic() < deadline:
        waited, status = os.waitpid(pid, os.WNOHANG)
        if waited == pid:
            if os.WIFEXITED(status) and os.WEXITSTATUS(status) == 0:
                return 0
            print(output.decode(errors="replace"), file=sys.stderr)
            if os.WIFSIGNALED(status):
                print(f"skillet terminated by signal {os.WTERMSIG(status)}", file=sys.stderr)
                return 128 + os.WTERMSIG(status)
            return os.WEXITSTATUS(status)
        readable, _, _ = select.select([fd], [], [], 0.1)
        if readable:
            try:
                output.extend(os.read(fd, 65536))
            except OSError:
                pass

    os.kill(pid, signal.SIGKILL)
    os.waitpid(pid, 0)
    print("skillet did not quit within 10 seconds", file=sys.stderr)
    print(output.decode(errors="replace"), file=sys.stderr)
    return 124


if __name__ == "__main__":
    raise SystemExit(main())
