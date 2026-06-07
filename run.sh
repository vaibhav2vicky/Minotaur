#!/bin/bash
# Minotaur C2 Framework - Startup Script

set -e

cd "$(dirname "$0")"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}Minotaur C2 Framework - Starting Server${NC}"

# Check Python version
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}Python 3 is required but not installed.${NC}"
    exit 1
fi

PYTHON_VERSION=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
echo -e "Python version: ${YELLOW}$PYTHON_VERSION${NC}"

# Check if virtual environment exists
if [ ! -d ".venv" ]; then
    echo -e "${YELLOW}Creating virtual environment...${NC}"
    python3 -m venv .venv
fi

# Activate virtual environment
source .venv/bin/activate

# Install/upgrade pip
pip install --upgrade pip -q

# Check if requirements are installed
if ! pip freeze | grep -q "Flask"; then
    echo -e "${YELLOW}Installing dependencies...${NC}"
    pip install flask flask-socketio
fi

# Run the server
echo -e "${GREEN}Starting server...${NC}"
python server/app.py