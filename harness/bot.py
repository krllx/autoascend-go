"""Arena bot shim: runs the Go AutoAscend port (./aabot) and talks to it over a pipe."""
from __future__ import annotations

import os
import struct
import subprocess

import numpy as np

KEYS = [('glyphs', np.int16), ('chars', np.uint8), ('specials', np.uint8), ('blstats', np.int64),
        ('message', np.uint8), ('tty_chars', np.uint8), ('tty_cursor', np.uint8), ('misc', np.int32),
        ('inv_strs', np.uint8), ('inv_letters', np.uint8), ('inv_oclasses', np.uint8), ('inv_glyphs', np.int16)]
_BIN = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'aabot')


def _pack(tag, obs):
    return tag + b''.join(np.ascontiguousarray(obs[k], dtype=t).tobytes() for k, t in KEYS)


class Bot:
    def __init__(self):
        self.proc = subprocess.Popen([_BIN], stdin=subprocess.PIPE, stdout=subprocess.PIPE, bufsize=0)

    def reset(self, obs):
        self.proc.stdin.write(_pack(b'R', obs))

    def _read(self, n):
        b = b''
        while len(b) < n:
            c = self.proc.stdout.read(n - len(b))
            if not c:
                raise RuntimeError('aabot exited')
            b += c
        return b

    def act(self, obs):
        self.proc.stdin.write(_pack(b'A', obs))
        return struct.unpack('<H', self._read(2))[0]

    def snapshot(self):
        """The bot state between two actions (bytes), or None if it cannot be saved."""
        self.proc.stdin.write(b'S')
        n = struct.unpack('<I', self._read(4))[0]
        return self._read(n) if n else None

    def restore(self, data):
        """Replace the bot state with a snapshot of the same aabot build (from any process)."""
        self.proc.stdin.write(b'L' + struct.pack('<I', len(data)) + data)
        return self._read(1) == b'\x01'

    def close(self):
        try:
            self.proc.stdin.write(b'C')
            self.proc.stdin.close()
        except OSError:
            pass
        self.proc.wait()


def make_agent():
    return Bot()
