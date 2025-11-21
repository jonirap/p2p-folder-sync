# Installation Guide

Complete instructions for installing P2P Folder Sync on various platforms.

## Prerequisites

- **Go**: 1.21 or higher (for building from source)
- **Operating System**: Linux, macOS, or Windows
- **Disk Space**: 10 GB minimum + your sync folder size
- **Network**: Ports 8080 (TCP) and 8081 (UDP) available

## Installation Methods

### Method 1: Pre-built Binaries (Recommended)

Download the latest release for your platform:

#### Linux (AMD64)
```bash
# Download binary
wget https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-linux-amd64

# Make executable
chmod +x p2p-sync-linux-amd64

# Move to system path
sudo mv p2p-sync-linux-amd64 /usr/local/bin/p2p-sync

# Verify installation
p2p-sync -version
```

#### Linux (ARM64)
```bash
wget https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-linux-arm64
chmod +x p2p-sync-linux-arm64
sudo mv p2p-sync-linux-arm64 /usr/local/bin/p2p-sync
```

#### macOS (Intel)
```bash
wget https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-darwin-amd64
chmod +x p2p-sync-darwin-amd64
sudo mv p2p-sync-darwin-amd64 /usr/local/bin/p2p-sync
```

#### macOS (Apple Silicon)
```bash
wget https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-darwin-arm64
chmod +x p2p-sync-darwin-arm64
sudo mv p2p-sync-darwin-arm64 /usr/local/bin/p2p-sync
```

#### Windows (AMD64)
```powershell
# Download from releases page
# Or use PowerShell
Invoke-WebRequest -Uri "https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-windows-amd64.exe" -OutFile "p2p-sync.exe"

# Add to PATH or run from current directory
./p2p-sync.exe -version
```

### Method 2: Build from Source

```bash
# Clone repository
git clone https://github.com/yourorg/p2p-sync.git
cd p2p-sync

# Install dependencies
go mod download

# Build
make build

# Binary will be at ./bin/p2p-sync
./bin/p2p-sync -version

# Optionally install system-wide
sudo cp ./bin/p2p-sync /usr/local/bin/
```

### Method 3: Docker

```bash
# Pull image
docker pull ghcr.io/yourorg/p2p-sync:latest

# Or build locally
docker build -t p2p-sync:latest .

# Run container
docker run -d \
  --name p2p-sync \
  -v /host/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  p2p-sync:latest
```

### Method 4: Package Managers (Future)

```bash
# Ubuntu/Debian (planned)
sudo apt-get install p2p-sync

# macOS Homebrew (planned)
brew install p2p-sync

# Arch Linux AUR (planned)
yay -S p2p-sync
```

## Post-Installation Setup

### 1. Create Configuration Directory

```bash
sudo mkdir -p /etc/p2p-sync
sudo mkdir -p /var/lib/p2p-sync
```

### 2. Create Configuration File

```bash
# Copy example config
sudo cp config/config.yaml /etc/p2p-sync/

# Edit for your environment
sudo nano /etc/p2p-sync/config.yaml
```

Minimum configuration:
```yaml
sync:
  folder_path: "/var/lib/p2p-sync/sync"

network:
  port: 8080
  discovery_port: 8081
```

### 3. Set Permissions

```bash
# Create sync folder
sudo mkdir -p /var/lib/p2p-sync/sync

# Set ownership (if running as dedicated user)
sudo useradd -r -s /bin/false p2psync
sudo chown -R p2psync:p2psync /var/lib/p2p-sync
```

### 4. Test Installation

```bash
# Run in foreground to test
p2p-sync -config /etc/p2p-sync/config.yaml

# Should see output like:
# INFO: Starting P2P Sync v1.0.0
# INFO: Sync folder: /var/lib/p2p-sync/sync
# INFO: Listening on port 8080
# INFO: Discovery on port 8081
```

## Verification

### Check Binary
```bash
p2p-sync -version
# Output: P2P Folder Sync v1.0.0
```

### Check Configuration
```bash
p2p-sync -config /etc/p2p-sync/config.yaml 2>&1 | head -n 5
# Should show startup messages without errors
```

### Check Network Ports
```bash
# After starting p2p-sync
sudo netstat -tulnp | grep p2p-sync
# Should show ports 8080 and 8081 listening
```

## Platform-Specific Notes

### Linux

**Firewall (UFW)**:
```bash
sudo ufw allow 8080/tcp
sudo ufw allow 8081/udp
```

**Firewall (firewalld)**:
```bash
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=8081/udp
sudo firewall-cmd --reload
```

**SELinux**:
```bash
# If SELinux is enforcing
sudo setsebool -P allow_user_exec_content on
sudo chcon -R -t user_home_t /var/lib/p2p-sync/sync
```

### macOS

**Security**:
```bash
# First run may require security approval
# System Preferences → Security & Privacy → Allow
```

**Firewall**:
```bash
# Allow incoming connections
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/p2p-sync
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp /usr/local/bin/p2p-sync
```

### Windows

**Firewall**:
```powershell
# Allow through Windows Firewall
New-NetFirewallRule -DisplayName "P2P Sync" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
New-NetFirewallRule -DisplayName "P2P Sync Discovery" -Direction Inbound -Protocol UDP -LocalPort 8081 -Action Allow
```

**Run as Service** (using NSSM):
```powershell
# Download NSSM from nssm.cc
nssm install P2PSync "C:\Program Files\p2p-sync\p2p-sync.exe" "-config C:\ProgramData\p2p-sync\config.yaml"
nssm start P2PSync
```

## Troubleshooting Installation

### Binary Won't Execute

**Linux/macOS**:
```bash
# Ensure it's executable
chmod +x /usr/local/bin/p2p-sync

# Check if correct architecture
file /usr/local/bin/p2p-sync
```

### Port Already in Use

```bash
# Find process using port
sudo lsof -i :8080
sudo lsof -i :8081

# Kill process or choose different ports
P2P_PORT=9090 p2p-sync -config config.yaml
```

### Permission Denied

```bash
# Run as root or change folder permissions
sudo chown -R $USER:$USER /var/lib/p2p-sync/sync

# Or run as dedicated user
sudo -u p2psync p2p-sync -config /etc/p2p-sync/config.yaml
```

## Next Steps

- [Configuration Guide](configuration.md) - Detailed configuration options
- [Deployment Guide](../../DEPLOYMENT.md) - Production deployment
- [Quick Start](../../README.md#quick-start) - Get started syncing

## Upgrading

### From Binary

```bash
# Stop service
sudo systemctl stop p2p-sync

# Backup database
sudo cp /var/lib/p2p-sync/data/p2p_sync.db /var/backups/

# Download new version
wget https://github.com/yourorg/p2p-sync/releases/latest/download/p2p-sync-linux-amd64
sudo mv p2p-sync-linux-amd64 /usr/local/bin/p2p-sync
chmod +x /usr/local/bin/p2p-sync

# Start service
sudo systemctl start p2p-sync
```

### From Docker

```bash
# Pull latest image
docker pull ghcr.io/yourorg/p2p-sync:latest

# Stop and remove old container
docker stop p2p-sync
docker rm p2p-sync

# Start new container (volumes persist)
docker run -d \
  --name p2p-sync \
  -v /host/sync:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  ghcr.io/yourorg/p2p-sync:latest
```

## Uninstallation

```bash
# Stop service
sudo systemctl stop p2p-sync
sudo systemctl disable p2p-sync

# Remove binary
sudo rm /usr/local/bin/p2p-sync

# Remove data (CAUTION: This deletes your sync state)
sudo rm -rf /var/lib/p2p-sync

# Remove config
sudo rm -rf /etc/p2p-sync

# Remove user (if created)
sudo userdel p2psync
```

---

**Last Updated**: January 2025
