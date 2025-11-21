# Deployment Guide

Complete guide for deploying P2P Folder Sync in production environments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Deployment Options](#deployment-options)
3. [Docker Deployment](#docker-deployment)
4. [Systemd Service](#systemd-service)
5. [Configuration](#configuration)
6. [Security](#security)
7. [Monitoring](#monitoring)
8. [High Availability](#high-availability)
9. [Backup and Recovery](#backup-and-recovery)
10. [Performance Tuning](#performance-tuning)
11. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Hardware Requirements

**Minimum**:
- CPU: 2 cores
- RAM: 2 GB
- Disk: 10 GB + synchronized folder size
- Network: 100 Mbps

**Recommended**:
- CPU: 4+ cores
- RAM: 4 GB
- Disk: SSD with 50 GB + synchronized folder size
- Network: 1 Gbps

### Software Requirements

- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+, Debian 11+)
- **Go**: 1.21+ (for building from source)
- **SQLite**: 3.x (usually pre-installed)
- **Docker**: 20.10+ (optional, for container deployment)

### Network Requirements

- **Firewall Rules**:
  - TCP port 8080 (sync traffic)
  - UDP port 8081 (peer discovery)
  - TCP port 9090 (metrics, internal only)

- **Bandwidth**: Varies based on file sizes and sync frequency
- **Latency**: <100ms for best performance

---

## Deployment Options

### 1. Standalone Binary

Best for: Development, testing, simple deployments

```bash
# Download latest release
wget https://github.com/yourorg/p2p-sync/releases/download/v1.0.0/p2p-sync-linux-amd64

# Make executable
chmod +x p2p-sync-linux-amd64
sudo mv p2p-sync-linux-amd64 /usr/local/bin/p2p-sync

# Create config
sudo mkdir -p /etc/p2p-sync
sudo cp config/config.yaml /etc/p2p-sync/

# Run
p2p-sync -config /etc/p2p-sync/config.yaml
```

### 2. Systemd Service

Best for: Production Linux servers

See [Systemd Service](#systemd-service) section below.

### 3. Docker Container

Best for: Cloud deployments, Kubernetes, Docker Swarm

See [Docker Deployment](#docker-deployment) section below.

### 4. Kubernetes

Best for: Large-scale, multi-region deployments

See [High Availability](#high-availability) section below.

---

## Docker Deployment

### Single Node

```bash
# Build image
docker build -t p2p-sync:latest .

# Run container
docker run -d \
  --name p2p-sync \
  -v /host/sync/folder:/app/sync \
  -v p2p-sync-db:/app/data \
  -p 8080:8080 \
  -p 8081:8081/udp \
  -e P2P_SYNC_FOLDER=/app/sync \
  -e LOG_LEVEL=info \
  --restart unless-stopped \
  p2p-sync:latest
```

### Docker Compose (Multi-Node)

```yaml
version: '3.8'

services:
  peer-alpha:
    build: .
    container_name: p2p-sync-alpha
    volumes:
      - ./sync-alpha:/app/sync
      - p2p-sync-alpha-db:/app/data
    ports:
      - "8080:8080"
      - "8081:8081/udp"
    environment:
      - P2P_SYNC_FOLDER=/app/sync
      - P2P_PORT=8080
      - P2P_DISCOVERY_PORT=8081
      - PEERS=peer-beta:8080,peer-gamma:8080
      - LOG_LEVEL=info
    networks:
      - p2p-network
    restart: unless-stopped

  peer-beta:
    build: .
    container_name: p2p-sync-beta
    volumes:
      - ./sync-beta:/app/sync
      - p2p-sync-beta-db:/app/data
    ports:
      - "8082:8080"
      - "8083:8081/udp"
    environment:
      - P2P_SYNC_FOLDER=/app/sync
      - P2P_PORT=8080
      - P2P_DISCOVERY_PORT=8081
      - PEERS=peer-alpha:8080,peer-gamma:8080
    networks:
      - p2p-network
    restart: unless-stopped

  peer-gamma:
    build: .
    container_name: p2p-sync-gamma
    volumes:
      - ./sync-gamma:/app/sync
      - p2p-sync-gamma-db:/app/data
    ports:
      - "8084:8080"
      - "8085:8081/udp"
    environment:
      - P2P_SYNC_FOLDER=/app/sync
      - P2P_PORT=8080
      - P2P_DISCOVERY_PORT=8081
      - PEERS=peer-alpha:8080,peer-beta:8080
    networks:
      - p2p-network
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    ports:
      - "9091:9090"
    networks:
      - p2p-network
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    volumes:
      - grafana-data:/var/lib/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    networks:
      - p2p-network
    restart: unless-stopped

volumes:
  p2p-sync-alpha-db:
  p2p-sync-beta-db:
  p2p-sync-gamma-db:
  prometheus-data:
  grafana-data:

networks:
  p2p-network:
    driver: bridge
```

### Docker Best Practices

1. **Volume Management**:
   - Use named volumes for database persistence
   - Use bind mounts for synchronized folder
   - Set correct permissions: `chown -R 1000:1000 /host/sync/folder`

2. **Resource Limits**:
   ```yaml
   deploy:
     resources:
       limits:
         cpus: '2'
         memory: 2G
       reservations:
         cpus: '1'
         memory: 1G
   ```

3. **Health Checks**:
   ```dockerfile
   HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
     CMD curl -f http://localhost:9090/metrics || exit 1
   ```

---

## Systemd Service

### Service File

Create `/etc/systemd/system/p2p-sync.service`:

```ini
[Unit]
Description=P2P Folder Synchronization Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=p2psync
Group=p2psync
WorkingDirectory=/var/lib/p2p-sync

# Binary and config
ExecStart=/usr/local/bin/p2p-sync -config /etc/p2p-sync/config.yaml

# Environment
Environment="LOG_LEVEL=info"
Environment="OTEL_ENDPOINT=http://localhost:4317"

# Restart policy
Restart=on-failure
RestartSec=10s
StartLimitBurst=3
StartLimitInterval=60s

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096
MemoryLimit=2G
CPUQuota=200%

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/p2p-sync
ReadOnlyPaths=/etc/p2p-sync

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=p2p-sync

[Install]
WantedBy=multi-user.target
```

### Setup Steps

```bash
# Create user and group
sudo useradd -r -s /bin/false -d /var/lib/p2p-sync p2psync

# Create directories
sudo mkdir -p /var/lib/p2p-sync/{sync,data}
sudo mkdir -p /etc/p2p-sync
sudo mkdir -p /var/log/p2p-sync

# Set permissions
sudo chown -R p2psync:p2psync /var/lib/p2p-sync
sudo chown -R p2psync:p2psync /var/log/p2p-sync
sudo chmod 755 /var/lib/p2p-sync
sudo chmod 700 /var/lib/p2p-sync/data

# Copy config
sudo cp config/config.yaml /etc/p2p-sync/
sudo chown root:p2psync /etc/p2p-sync/config.yaml
sudo chmod 640 /etc/p2p-sync/config.yaml

# Edit config for your environment
sudo nano /etc/p2p-sync/config.yaml

# Install and enable service
sudo systemctl daemon-reload
sudo systemctl enable p2p-sync
sudo systemctl start p2p-sync

# Check status
sudo systemctl status p2p-sync

# View logs
sudo journalctl -u p2p-sync -f
```

### Service Management

```bash
# Start service
sudo systemctl start p2p-sync

# Stop service
sudo systemctl stop p2p-sync

# Restart service
sudo systemctl restart p2p-sync

# Reload configuration
sudo systemctl reload p2p-sync

# Check status
sudo systemctl status p2p-sync

# View logs
sudo journalctl -u p2p-sync -f
sudo journalctl -u p2p-sync --since "1 hour ago"
sudo journalctl -u p2p-sync --since today
```

---

## Configuration

### Production Configuration Template

```yaml
sync:
  folder_path: "/var/lib/p2p-sync/sync"
  chunk_size_default: 524288        # 512KB
  max_concurrent_transfers: 5

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30
  connection_timeout: 60
  peers:
    - "peer1.example.com:8080"
    - "peer2.example.com:8080"

security:
  key_rotation_interval: 86400      # 24 hours

compression:
  enabled: true
  file_size_threshold: 1048576      # 1MB
  algorithm: "zstd"
  level: 3

observability:
  otel_endpoint: "http://otel-collector:4317"
  log_level: "info"
  metrics_enabled: true
  tracing_enabled: true
```

### Environment-Specific Settings

#### Development
```yaml
observability:
  log_level: "debug"
  metrics_enabled: false
  tracing_enabled: false
```

#### Staging
```yaml
compression:
  level: 1                          # Faster for testing
observability:
  log_level: "info"
  metrics_enabled: true
  tracing_enabled: true
```

#### Production
```yaml
sync:
  max_concurrent_transfers: 10      # Higher throughput
compression:
  level: 5                          # Better compression
observability:
  log_level: "warn"                 # Less verbose
  metrics_enabled: true
  tracing_enabled: true
```

---

## Security

### Network Security

#### Firewall Configuration (UFW)

```bash
# Allow sync port
sudo ufw allow 8080/tcp comment 'P2P Sync'

# Allow discovery port
sudo ufw allow 8081/udp comment 'P2P Discovery'

# Allow metrics (internal only)
sudo ufw allow from 10.0.0.0/8 to any port 9090 proto tcp comment 'Metrics'

# Enable firewall
sudo ufw enable
```

#### Firewall Configuration (firewalld)

```bash
# Add services
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=8081/udp
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.0.0.0/8" port port="9090" protocol="tcp" accept'

# Reload
sudo firewall-cmd --reload
```

### TLS/SSL Configuration

For external-facing deployments, use a reverse proxy with TLS:

#### Nginx Configuration

```nginx
upstream p2p_sync {
    least_conn;
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name sync.example.com;

    ssl_certificate /etc/letsencrypt/live/sync.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/sync.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    location / {
        proxy_pass http://p2p_sync;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### File System Security

```bash
# Secure configuration directory
sudo chmod 755 /etc/p2p-sync
sudo chmod 640 /etc/p2p-sync/config.yaml

# Secure data directory
sudo chmod 700 /var/lib/p2p-sync/data

# Secure sync folder
sudo chmod 755 /var/lib/p2p-sync/sync

# Set SELinux context (if applicable)
sudo chcon -R -t user_home_t /var/lib/p2p-sync/sync
```

---

## Monitoring

### Prometheus Configuration

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'p2p-sync'
    static_configs:
      - targets:
          - 'peer-alpha:9090'
          - 'peer-beta:9090'
          - 'peer-gamma:9090'
    metrics_path: '/metrics'
```

### Grafana Dashboard

Import the provided dashboard (coming soon) or create custom panels:

#### Key Metrics to Monitor

1. **Sync Operations**:
   - `sync_operations_total`
   - `sync_operation_duration_seconds`
   - `sync_operation_errors_total`

2. **Network**:
   - `network_connections_active`
   - `network_message_latency_seconds`
   - `network_chunk_retransmissions_total`

3. **Resources**:
   - `resource_memory_bytes`
   - `resource_cpu_usage_ratio`
   - `resource_disk_usage_bytes`

4. **Compression**:
   - `compression_files_compressed_total`
   - `compression_bytes_saved_total`
   - `compression_ratio`

### Alerting Rules

Create `alerts.yml`:

```yaml
groups:
  - name: p2p_sync
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: rate(sync_operation_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High sync error rate on {{ $labels.instance }}"

      - alert: PeerDisconnected
        expr: network_connections_active == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "No peer connections on {{ $labels.instance }}"

      - alert: HighMemoryUsage
        expr: resource_memory_bytes > 1.8e9  # 1.8GB
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage on {{ $labels.instance }}"

      - alert: DiskSpaceLow
        expr: resource_disk_free_bytes < 5e9  # 5GB
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Low disk space on {{ $labels.instance }}"
```

### Logging

#### Centralized Logging with Fluentd

```yaml
# fluent.conf
<source>
  @type forward
  port 24224
</source>

<filter p2p.sync.**>
  @type parser
  key_name log
  <parse>
    @type json
  </parse>
</filter>

<match p2p.sync.**>
  @type elasticsearch
  host elasticsearch
  port 9200
  logstash_format true
  logstash_prefix p2p-sync
</match>
```

---

## High Availability

### Multi-Region Deployment

```
Region A (Primary)    Region B (DR)       Region C (Edge)
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│  Peer A1    │──────│  Peer B1    │──────│  Peer C1    │
│  Peer A2    │      │  Peer B2    │      │  Peer C2    │
└─────────────┘      └─────────────┘      └─────────────┘
```

### Load Balancing

Use DNS round-robin or a load balancer for peer discovery:

```yaml
network:
  peers:
    - "lb-region-a.example.com:8080"
    - "lb-region-b.example.com:8080"
    - "lb-region-c.example.com:8080"
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: p2p-sync
spec:
  serviceName: p2p-sync
  replicas: 3
  selector:
    matchLabels:
      app: p2p-sync
  template:
    metadata:
      labels:
        app: p2p-sync
    spec:
      containers:
      - name: p2p-sync
        image: p2p-sync:latest
        ports:
        - containerPort: 8080
          name: sync
        - containerPort: 8081
          name: discovery
          protocol: UDP
        volumeMounts:
        - name: sync-data
          mountPath: /app/sync
        - name: db-data
          mountPath: /app/data
        env:
        - name: P2P_SYNC_FOLDER
          value: "/app/sync"
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        resources:
          limits:
            cpu: "2"
            memory: "2Gi"
          requests:
            cpu: "1"
            memory: "1Gi"
  volumeClaimTemplates:
  - metadata:
      name: sync-data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 50Gi
  - metadata:
      name: db-data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 10Gi
```

---

## Backup and Recovery

### Database Backup

```bash
#!/bin/bash
# backup-db.sh

BACKUP_DIR="/var/backups/p2p-sync"
DB_PATH="/var/lib/p2p-sync/data/p2p_sync.db"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR"

# Backup with SQLite backup command
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/p2p_sync_$TIMESTAMP.db'"

# Compress
gzip "$BACKUP_DIR/p2p_sync_$TIMESTAMP.db"

# Retain last 7 days
find "$BACKUP_DIR" -name "*.db.gz" -mtime +7 -delete

echo "Backup completed: p2p_sync_$TIMESTAMP.db.gz"
```

### Automated Backup with Cron

```bash
# Add to crontab
0 2 * * * /usr/local/bin/backup-db.sh >> /var/log/p2p-sync/backup.log 2>&1
```

### Restore from Backup

```bash
# Stop service
sudo systemctl stop p2p-sync

# Restore database
gunzip -c /var/backups/p2p-sync/p2p_sync_20250119_020000.db.gz > /var/lib/p2p-sync/data/p2p_sync.db

# Fix permissions
sudo chown p2psync:p2psync /var/lib/p2p-sync/data/p2p_sync.db

# Start service
sudo systemctl start p2p-sync
```

---

## Performance Tuning

### System Tuning

```bash
# Increase file descriptors
echo "fs.file-max = 100000" | sudo tee -a /etc/sysctl.conf
echo "p2psync soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "p2psync hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# TCP tuning
echo "net.core.somaxconn = 1024" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_max_syn_backlog = 2048" | sudo tee -a /etc/sysctl.conf

# Apply changes
sudo sysctl -p
```

### Application Tuning

```yaml
# For high-throughput environments
sync:
  chunk_size_default: 1048576       # 1MB chunks
  max_concurrent_transfers: 10

compression:
  level: 1                          # Fast compression

# For low-bandwidth environments
sync:
  chunk_size_default: 262144        # 256KB chunks
  max_concurrent_transfers: 3

compression:
  level: 9                          # Maximum compression
```

---

## Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed troubleshooting steps.

### Quick Diagnostics

```bash
# Check service status
systemctl status p2p-sync

# View recent logs
journalctl -u p2p-sync --since "10 minutes ago"

# Check metrics
curl http://localhost:9090/metrics

# Test connectivity
nc -zv peer.example.com 8080

# Check database
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA integrity_check;"
```

---

## Additional Resources

- [Configuration Reference](API_REFERENCE.md)
- [Architecture Documentation](ARCHITECTURE.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)
- [Performance Tuning](docs/guides/performance.md)

---

**Last Updated**: January 2025
