"""Shared stdout/stderr encoding setup for LeGaJ's Python helpers.

On Windows, ``sys.stdout`` and ``sys.stderr`` default to the legacy ANSI code
page (typically cp1252), which raises ``UnicodeEncodeError`` the moment output
contains a character outside Latin-1. Real-world resumes and profiles routinely
include such characters: bullets (``●`` / ``•``), em/en dashes (``—`` / ``–``),
smart quotes, and accented names. The Go GUI captures each tool's stdout, so a
single unencodable glyph would crash the tool and surface as an opaque error.

Reconfiguring the streams to UTF-8 makes output reliable on every platform.
Each tool calls :func:`force_utf8_io` before it prints user-derived text; the
frozen ``legaj-tools`` dispatcher also calls it once so the fix applies even to
a tool that has not been updated individually.
"""

import sys


def force_utf8_io():
    """Reconfigure stdout/stderr to UTF-8 where the platform supports it.

    Safe to call multiple times and on streams that do not support
    reconfiguration (e.g. a stream already replaced by a plain buffer), in
    which case the stream is left untouched.
    """
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is None:
            continue
        try:
            reconfigure(encoding="utf-8")
        except (ValueError, OSError):
            pass
