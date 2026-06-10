#!/bin/bash
# Minotaur C2 Framework - One‑command startup script (Linux)
# Checks for Python3, Go, sets up venv, and runs the server.

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Minotaur C2 Framework - Starting Server${NC}"

# ---------- Helper: detect package manager ----------
detect_package_manager() {
    if command -v apt-get &> /dev/null; then
        echo "apt"
    elif command -v dnf &> /dev/null; then
        echo "dnf"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v pacman &> /dev/null; then
        echo "pacman"
    elif command -v zypper &> /dev/null; then
        echo "zypper"
    else
        echo "unknown"
    fi
}

# ---------- Check Python 3 ----------
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}Python 3 is required but not installed.${NC}"
    PM=$(detect_package_manager)
    case $PM in
        apt)
            echo -e "${YELLOW}Install with: sudo apt install python3 python3-venv${NC}"
            ;;
        dnf|yum)
            echo -e "${YELLOW}Install with: sudo dnf install python3 python3-virtualenv${NC}"
            ;;
        pacman)
            echo -e "${YELLOW}Install with: sudo pacman -S python python-virtualenv${NC}"
            ;;
        zypper)
            echo -e "${YELLOW}Install with: sudo zypper install python3 python3-virtualenv${NC}"
            ;;
        *)
            echo -e "${YELLOW}Please install Python 3.8+ from your package manager.${NC}"
            ;;
    esac
    exit 1
fi

PYTHON_VERSION=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
echo -e "Python version: ${YELLOW}$PYTHON_VERSION${NC}"

# ---------- Check Go ----------
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is required but not installed.${NC}"
    PM=$(detect_package_manager)
    case $PM in
        apt)
            echo -e "${YELLOW}Install with: sudo apt install golang-go${NC}"
            echo -e "   or for latest: https://go.dev/doc/install"
            ;;
        dnf|yum)
            echo -e "${YELLOW}Install with: sudo dnf install golang${NC}"
            ;;
        pacman)
            echo -e "${YELLOW}Install with: sudo pacman -S go${NC}"
            ;;
        zypper)
            echo -e "${YELLOW}Install with: sudo zypper install go${NC}"
            ;;
        *)
            echo -e "${YELLOW}Please install Go 1.21+ from https://go.dev/dl/${NC}"
            ;;
    esac
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo -e "Go version: ${YELLOW}$GO_VERSION${NC}"

# ---------- Create virtual environment if missing ----------
VENV_DIR=".venv"
if [ ! -d "$VENV_DIR" ]; then
    echo -e "${YELLOW}Creating virtual environment...${NC}"
    python3 -m venv "$VENV_DIR"
fi

# Activate virtual environment
source "$VENV_DIR/bin/activate"

# Upgrade pip
pip install --upgrade pip -q

# Install required packages if missing
if ! pip freeze | grep -q "Flask"; then
    echo -e "${YELLOW}Installing Python dependencies...${NC}"
    pip install flask flask-socketio cryptography
fi

# ---------- Run the C2 server ----------
echo -e "${GREEN}Starting server...${NC}"
python server/app.py