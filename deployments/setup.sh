#!/usr/bin/env bash
set -euo pipefail

echo "=================================================================="
echo " YouTube Data Harvester (harvest/) EC2 Bootstrap Script"
echo "=================================================================="

# 1. Update and install base packages
sudo apt-get update -y
sudo apt-get install -y git curl build-essential htop

# 2. Install Go 1.27 if not already installed
if ! command -v go &> /dev/null; then
    echo "Installing Go..."
    ARCH="$(dpkg --print-architecture)"
    GO_VERSION="1.27.0"
    GO_TAR="go${GO_VERSION}.linux-${ARCH}.tar.gz"
    curl -fsSLO "https://go.dev/dl/${GO_TAR}"
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf "${GO_TAR}"
    rm "${GO_TAR}"
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
    export PATH=$PATH:/usr/local/go/bin
fi

echo "Go version: $(go version)"

# 3. Build Harvester Binary
echo "Building harvester binary..."
make build

# 4. Check for .env file
if [ ! -f .env ]; then
    echo "Creating .env template..."
    cat << 'EOF' > .env
YOUTUBE_API_KEY="YOUR_ACTUAL_API_KEY_HERE"
DATABASE_PATH="./data/harvester.db"
EOF
    echo "WARNING: Please edit .env with your real YOUTUBE_API_KEY before starting the service!"
fi

# 5. Install systemd service
echo "Installing systemd service..."
sudo cp deployments/viral-harvester.service /etc/systemd/system/
sudo systemctl daemon-reload

echo "=================================================================="
echo " Setup Complete!"
echo " Next steps:"
echo " 1. Edit .env with your real YOUTUBE_API_KEY"
echo " 2. Enable & start service: sudo systemctl enable --now viral-harvester"
echo " 3. Check logs: journalctl -u viral-harvester -f"
echo "=================================================================="
