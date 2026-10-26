#!/usr/bin/env python3
"""A stand-in terminal for the e2e suites.

usage: faketerm.py LOG COLS ROWS NAME CMD [ARGS...]

Runs CMD in a pty of COLS x ROWS cells (8 x 18 pixels each), answers tmux's XTVERSION query
(CSI > q) as NAME, e.g. "ghostty 1.3.1", and appends every byte CMD writes to LOG. That is what
reaches a real terminal, so a suite can check for kitty graphics commands that tmux let through.
"""
import fcntl
import os
import pty
import select
import struct
import sys
import termios

log, cols, rows, name, argv = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4], sys.argv[5:]
pid, fd = pty.fork()
if pid == 0:
    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, cols * 8, rows * 18))
    os.execvp(argv[0], argv)

reply = ("\x1bP>|%s\x1b\\" % name).encode()
tail = b""
with open(log, "ab", buffering=0) as out:
    while True:
        r, _, _ = select.select([fd], [], [], 1.0)
        if not r:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            break
        if not data:
            break
        out.write(data)
        seen = tail + data
        tail = seen[-8:]
        if b"\x1b[>q" in seen or b"\x1b[>0q" in seen:
            os.write(fd, reply)
            tail = b""  # answered: the same query must not be answered twice
os.waitpid(pid, 0)
