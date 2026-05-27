#!/usr/bin/env bash
#
# LeGaJ macOS setup script.
# Checks for all required dependencies, installs any that are missing via
# Homebrew, then launches the app.
#
# Usage (from the project root in Terminal):
#   chmod +x install_mac.sh && ./install_mac.sh

set -euo pipefail
cd "$(dirname "$0")"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}!${NC} $1"; }
err()  { echo -e "  ${RED}✗${NC} $1"; }

echo ""
echo "  LeGaJ — macOS Setup"
echo "  ────────────────────"
echo ""

# ── 1. Xcode Command Line Tools ───────────────────────────────────────────────
echo "Checking Xcode Command Line Tools..."
if ! xcode-select -p &>/dev/null; then
    warn "Not found. A dialog will appear — click Install and wait for it to finish."
    warn "Once the install completes, re-run this script."
    xcode-select --install
    exit 0
fi
ok "Xcode CLT installed."

# ── 2. Homebrew ───────────────────────────────────────────────────────────────
echo "Checking Homebrew..."
if ! command -v brew &>/dev/null; then
    warn "Not found. Installing Homebrew (you may be prompted for your password)..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    # Apple Silicon installs Homebrew to /opt/homebrew; add it to PATH for this session.
    if [[ -f /opt/homebrew/bin/brew ]]; then
        eval "$(/opt/homebrew/bin/brew shellenv)"
    fi
fi
ok "Homebrew ready."

# ── 3. Go 1.25+ ──────────────────────────────────────────────────────────────
echo "Checking Go 1.25+..."
need_go=true
if command -v go &>/dev/null; then
    go_ver=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | head -1 | sed 's/go//')
    go_major=$(echo "$go_ver" | cut -d. -f1)
    go_minor=$(echo "$go_ver" | cut -d. -f2)
    if [[ "$go_major" -gt 1 ]] || [[ "$go_major" -eq 1 && "$go_minor" -ge 25 ]]; then
        ok "Go $go_ver found."
        need_go=false
    else
        warn "Go $go_ver is below the required 1.25. Upgrading via Homebrew..."
        brew upgrade go || brew install go
    fi
fi
if $need_go; then
    warn "Go not found. Installing via Homebrew..."
    brew install go
fi
ok "Go requirement satisfied."

# ── 4. Python 3.12+ ───────────────────────────────────────────────────────────
echo "Checking Python 3.12+..."
PYTHON_BIN=""
for candidate in python3.14 python3.13 python3.12 python3; do
    if command -v "$candidate" &>/dev/null; then
        py_ver=$("$candidate" --version 2>&1 | grep -oE '[0-9]+\.[0-9]+' | head -1)
        py_major=$(echo "$py_ver" | cut -d. -f1)
        py_minor=$(echo "$py_ver" | cut -d. -f2)
        if [[ "$py_major" -ge 3 && "$py_minor" -ge 12 ]]; then
            PYTHON_BIN="$candidate"
            ok "Python $py_ver found ($candidate)."
            break
        fi
    fi
done
if [[ -z "$PYTHON_BIN" ]]; then
    warn "Python 3.12+ not found. Installing via Homebrew..."
    brew install python@3.12
    PYTHON_BIN="python3.12"
    ok "Python 3.12 installed."
fi

# ── 5. Python dependencies ────────────────────────────────────────────────────
echo "Installing Python dependencies..."
"$PYTHON_BIN" -m pip install --upgrade pip --quiet
"$PYTHON_BIN" -m pip install -r requirements.txt --quiet
ok "Python dependencies installed."

# ── 6. Launch ─────────────────────────────────────────────────────────────────
echo ""
echo "  ────────────────────────────────────────"
echo "  Setup complete. Launching LeGaJ..."
echo "  Your data will be saved to:"
echo "  ~/Library/Application Support/LeGaJ/"
echo "  ────────────────────────────────────────"
echo ""
go run .
