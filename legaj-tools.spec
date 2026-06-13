# -*- mode: python ; coding: utf-8 -*-
#
# PyInstaller spec for LeGaJ's bundled Python helpers.
#
# Build a single self-contained executable that exposes every Python tool behind
# the legaj_tools dispatcher, so distributed builds need no Python install:
#
#     pyinstaller legaj-tools.spec
#
# Output: dist/legaj-tools(.exe). Copy it into the packaged app's tools/ folder.
#
# Notes:
#  * The individual scripts are referenced only by string in legaj_tools.TOOLS,
#    so they are listed explicitly under hiddenimports.
#  * reportlab and genanki ship data files (fonts, templates) that must be
#    collected, hence collect_all below.

from PyInstaller.utils.hooks import collect_all

# Tool modules invoked dynamically by the dispatcher.
hiddenimports = [
    "_encoding",
    "parse_resume",
    "tailor_resume",
    "generate_resume_pdf",
    "generate_cover_letter_pdf",
    "manage_applications",
    "prepare_interview",
    "search_jobs",
    "create_tracker",
]

datas = []
binaries = []

# Packages that bundle data files / need full collection.
for pkg in ("reportlab", "genanki", "pypdf", "docx", "bs4"):
    try:
        d, b, h = collect_all(pkg)
        datas += d
        binaries += b
        hiddenimports += h
    except Exception:
        # Optional dependency not installed; skip it.
        pass


a = Analysis(
    ["scripts/legaj_tools.py"],
    pathex=["scripts"],
    binaries=binaries,
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
)

pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name="legaj-tools",
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=True,          # tools communicate via stdout/stderr
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
)
