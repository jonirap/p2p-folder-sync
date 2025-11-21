# Configuration Guide

Detailed guide for configuring P2P Folder Sync for your environment.

## Configuration Methods

Configuration can be provided through:
1. **Configuration file** (YAML)
2. **Environment variables**
3. **Command-line flags**

**Precedence** (highest to lowest): Environment Variables → Config File → Defaults

## Basic Configuration

### Minimal Configuration

```yaml
sync:
  folder_path: "/path/to/sync"

network:
  port: 8080
  discovery_port: 8081
```

This is sufficient to get started with automatic peer discovery on the local network.

## Complete Configuration Reference

See [API_REFERENCE.md](../../API_REFERENCE.md#configuration-reference) for complete options.

## Common Configuration Scenarios

### Scenario 1: Single Office Network

**Use Case**: Small office with 5-10 computers on same LAN

```yaml
sync:
  folder_path: "/shared/documents"
  max_concurrent_transfers: 5

network:
  port: 8080
  discovery_port: 8081
  # Auto-discovery via mDNS

compression:
  enabled: true
  algorithm: "zstd"
  level: 3

observability:
  log_level: "info"
  metrics_enabled: true
```

### Scenario 2: Multi-Office with VPN

**Use Case**: Offices connected via VPN, need explicit peer list

```yaml
sync:
  folder_path: "/shared/files"
  max_concurrent_transfers: 10

network:
  port: 8080
  discovery_port: 8081
  peers:
    - "office-nyc.vpn:8080"
    - "office-sf.vpn:8080"
    - "office-london.vpn:8080"

compression:
  enabled: true
  algorithm: "zstd"
  level: 5  # Higher compression for WAN

observability:
  otel_endpoint: "http://monitoring.company.com:4317"
  log_level: "info"
  metrics_enabled: true
  tracing_enabled: true
```

### Scenario 3: High-Performance LAN

**Use Case**: Gigabit LAN, large files, need maximum throughput

```yaml
sync:
  folder_path: "/data/media"
  chunk_size_default: 1048576  # 1MB chunks
  max_concurrent_transfers: 10

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30

compression:
  enabled: true
  algorithm: "lz4"  # Fastest compression
  level: 1
  file_size_threshold: 10485760  # 10MB threshold

observability:
  log_level: "warn"  # Less verbose for performance
```

### Scenario 4: Low-Bandwidth Remote

**Use Case**: Slow connection, minimize bandwidth usage

```yaml
sync:
  folder_path: "/sync"
  chunk_size_default: 262144  # 256KB chunks
  max_concurrent_transfers: 2

network:
  port: 8080
  discovery_port: 8081
  connection_timeout: 120  # Longer timeout

compression:
  enabled: true
  algorithm: "zstd"
  level: 9  # Maximum compression
  file_size_threshold: 102400  # 100KB threshold

observability:
  log_level: "error"  # Minimize logging overhead
```

### Scenario 5: Docker Swarm/Kubernetes

**Use Case**: Container orchestration, dynamic peer discovery

```yaml
sync:
  folder_path: "/app/sync"
  max_concurrent_transfers: 5

network:
  port: 8080
  discovery_port: 8081
  peers:
    - "p2p-sync-0.p2p-sync:8080"
    - "p2p-sync-1.p2p-sync:8080"
    - "p2p-sync-2.p2p-sync:8080"

compression:
  enabled: true
  algorithm: "zstd"
  level: 3

observability:
  otel_endpoint: "${OTEL_ENDPOINT}"
  log_level: "${LOG_LEVEL:-info}"
  metrics_enabled: true
  tracing_enabled: true
```

## Environment Variables

Override any configuration value:

```bash
# Basic settings
export P2P_SYNC_FOLDER="/data/sync"
export P2P_PORT=8080
export P2P_DISCOVERY_PORT=8081

# Peer list (comma-separated)
export PEERS="peer1:8080,peer2:8080,peer3:8080"

# Logging
export LOG_LEVEL=debug

# Observability
export OTEL_ENDPOINT="http://otel-collector:4317"

# Run
p2p-sync -config /etc/p2p-sync/config.yaml
```

## Configuration Validation

Validate configuration before running:

```bash
# Test configuration
p2p-sync -config config.yaml 2>&1 | grep -i error

# Check syntax
yamllint config.yaml
```

## Security Best Practices

### File Permissions

```bash
# Config file should be readable only by owner/group
chmod 640 /etc/p2p-sync/config.yaml
chown root:p2psync /etc/p2p-sync/config.yaml

# Data directory should be private
chmod 700 /var/lib/p2p-sync/data
chown p2psync:p2psync /var/lib/p2p-sync/data
```

### Secrets Management

Never commit secrets to version control. Use:

1. **Environment variables**:
   ```yaml
   observability:
     otel_endpoint: "${OTEL_ENDPOINT}"
   ```

2. **External secrets**:
   ```bash
   # Source secrets before running
   source /etc/p2p-sync/secrets.env
   p2p-sync -config /etc/p2p-sync/config.yaml
   ```

3. **Kubernetes secrets**:
   ```yaml
   env:
     - name: OTEL_ENDPOINT
       valueFrom:
         secretKeyRef:
           name: p2p-sync-secrets
           key: otel-endpoint
   ```

## Dynamic Configuration

Some settings can be changed without restart:

### Log Level
```bash
# Send SIGUSR1 to increase log level
kill -USR1 $(pgrep p2p-sync)

# Send SIGUSR2 to decrease log level
kill -USR2 $(pgrep p2p-sync)
```

### Reload Configuration
```bash
# Send SIGHUP to reload config
kill -HUP $(pgrep p2p-sync)

# Or via systemd
sudo systemctl reload p2p-sync
```

## Configuration Templates

### Production Template

```yaml
sync:
  folder_path: "/var/lib/p2p-sync/sync"
  chunk_size_default: 524288
  max_concurrent_transfers: 5
  operation_log_size: 10000

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30
  connection_timeout: 60
  peers: []  # Set via environment

security:
  key_rotation_interval: 86400

compression:
  enabled: true
  file_size_threshold: 1048576
  algorithm: "zstd"
  level: 3
  chunk_compression: true

observability:
  otel_endpoint: "${OTEL_ENDPOINT}"
  log_level: "warn"
  metrics_enabled: true
  tracing_enabled: true
```

### Development Template

```yaml
sync:
  folder_path: "./sync-data"
  chunk_size_default: 524288
  max_concurrent_transfers: 3

network:
  port: 8080
  discovery_port: 8081

compression:
  enabled: false  # Disable for faster testing

observability:
  log_level: "debug"
  metrics_enabled: false
  tracing_enabled: false
```

## Troubleshooting Configuration

### Common Issues

**1. Port conflicts**:
```bash
# Check if ports are in use
sudo lsof -i :8080
sudo lsof -i :8081

# Use different ports
P2P_PORT=9090 P2P_DISCOVERY_PORT=9091 p2p-sync -config config.yaml
```

**2. Invalid YAML**:
```bash
# Validate YAML syntax
python3 -c "import yaml; yaml.safe_load(open('config.yaml'))"
```

**3. Path doesn't exist**:
```bash
# Create sync folder
mkdir -p /path/to/sync
chown p2psync:p2psync /path/to/sync
```

**4. Permission denied**:
```bash
# Check file permissions
ls -la /etc/p2p-sync/config.yaml

# Fix permissions
sudo chmod 640 /etc/p2p-sync/config.yaml
```

## Configuration Best Practices

1. **Start with defaults**: Only override what you need
2. **Use environment variables for secrets**: Never hardcode
3. **Version control your config**: Track changes over time
4. **Test before deploying**: Validate in staging first
5. **Document custom settings**: Add comments explaining why
6. **Monitor configuration changes**: Alert on unexpected modifications

## Next Steps

- [Performance Tuning](performance.md) - Optimize for your workload
- [Security Guide](security.md) - Harden your deployment
- [Monitoring](monitoring.md) - Set up observability

---

**Last Updated**: January 2025
