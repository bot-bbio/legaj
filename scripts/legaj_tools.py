#!/usr/bin/env python3
"""legaj_tools — single entry point for LeGaJ's Python helpers.

When LeGaJ is packaged for distribution, every Python script is frozen into one
PyInstaller executable (``legaj-tools``) so end users do not need Python or any
pip dependencies installed. The Go GUI invokes this binary with the tool name as
the first argument, e.g.::

    legaj-tools parse_resume <resume_path>
    legaj-tools generate_resume_pdf <profile_json> <output_pdf>
    legaj-tools manage_applications <tracker_json> list

Each tool's own ``__main__`` block is executed unchanged via ``runpy``; this
dispatcher only rewrites ``sys.argv`` so the target sees the arguments it
expects. In development you can still run the individual ``scripts/*.py`` files
directly — the dispatcher is only used by packaged builds.
"""

import runpy
import sys

from _encoding import force_utf8_io

# Reconfigure stdout/stderr to UTF-8 once for the whole process. runpy runs each
# tool in this same process, so this covers every dispatched tool — even one
# that has not been individually updated — in frozen builds.
force_utf8_io()

# Map dispatch token -> module name (modules live alongside this file).
TOOLS = {
    "parse_resume": "parse_resume",
    "tailor_resume": "tailor_resume",
    "generate_resume_pdf": "generate_resume_pdf",
    "generate_cover_letter_pdf": "generate_cover_letter_pdf",
    "manage_applications": "manage_applications",
    "prepare_interview": "prepare_interview",
    "search_jobs": "search_jobs",
    "create_tracker": "create_tracker",
}


def main() -> int:
    if len(sys.argv) < 2 or sys.argv[1] not in TOOLS:
        valid = ", ".join(sorted(TOOLS))
        sys.stderr.write(
            "usage: legaj-tools <tool> [args...]\n"
            f"valid tools: {valid}\n"
        )
        return 2

    tool = sys.argv[1]
    # Present the target module with argv as if it had been launched directly:
    # argv[0] = tool name, argv[1:] = the tool's own arguments.
    sys.argv = [tool] + sys.argv[2:]
    runpy.run_module(TOOLS[tool], run_name="__main__")
    return 0


if __name__ == "__main__":
    sys.exit(main())
