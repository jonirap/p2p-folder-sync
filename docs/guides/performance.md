# Performance Tuning Guide

Optimize P2P Folder Sync for your workload and environment.

## Performance Metrics

Monitor these key indicators:

| Metric | Target | Command |
|--------|--------|---------|
| Sync Latency | <1s for small files | `curl localhost:9090/metrics | grep sync_operation_duration` |
| Throughput | >100 MB/s on gigabit LAN | `curl localhost:9090/metrics | grep sync_file_transfer_bytes` |
| CPU Usage | <50% average | `top -p $(pgrep p2p-sync)` |
| Memory Usage | <1 GB | `ps aux | grep p2p-sync` |
| Active Connections | All peers connected | `curl localhost:9090/metrics | grep network_connections_active` |

## Quick Wins

### 1. Increase Concurrent Transfers

```yaml
sync:
  max_concurrent_transfers: 10  # Default: 5
```

**Impact**: 2x throughput for multiple files
**Trade-off**: Higher CPU and memory usage

### 2. Use Faster Compression

```yaml
compression:
  algorithm: "lz4"  # Instead of zstd
  level: 1
```

**Impact**: 30-50% faster compression
**Trade-off**: 10-20% less compression ratio

### 3. Larger Chunk Sizes

```yaml
sync:
  chunk_size_default: 1048576  # 1MB instead of 512KB
```

**Impact**: Fewer network round trips
**Trade-off**: More memory per transfer

### 4. Optimize System

```bash
# Increase file descriptors
ulimit -n 65536

# Increase network buffer sizes
sudo sysctl -w net.core.rmem_max=16777216
sudo sysctl -w net.core.wmem_max=16777216
```

## Workload-Specific Tuning

### Large Files (>100MB)

**Configuration**:
```yaml
sync:
  chunk_size_default: 2097152  # 2MB chunks
  max_concurrent_transfers: 3   # Fewer, larger transfers

compression:
  enabled: true
  algorithm: "lz4"
  level: 1
  file_size_threshold: 10485760  # 10MB
```

**Rationale**:
- Larger chunks reduce overhead
- LZ4 minimizes CPU time
- Fewer concurrent transfers prevent memory exhaustion

### Many Small Files (<1MB)

**Configuration**:
```yaml
sync:
  chunk_size_default: 524288  # 512KB (no chunking for small files)
  max_concurrent_transfers: 15

compression:
  enabled: true
  algorithm: "zstd"
  level: 1
  file_size_threshold: 102400  # 100KB
```

**Rationale**:
- Small chunks unnecessary
- Many concurrent transfers maximize throughput
- Lower compression level for speed

### Mixed Workload

**Configuration**:
```yaml
sync:
  chunk_size_default: 524288
  max_concurrent_transfers: 8

compression:
  enabled: true
  algorithm: "zstd"
  level: 3
  file_size_threshold: 1048576
```

**Rationale**: Balanced settings for variety of file sizes

## Network Optimization

### High-Latency Networks (>50ms RTT)

```yaml
network:
  heartbeat_interval: 60       # Longer interval
  connection_timeout: 120      # Higher timeout

sync:
  chunk_size_default: 1048576  # Larger chunks
```

### Low-Bandwidth Networks (<10 Mbps)

```yaml
sync:
  max_concurrent_transfers: 2

compression:
  enabled: true
  algorithm: "zstd"
  level: 9  # Maximum compression
  file_size_threshold: 51200  # 50KB
```

### High-Bandwidth LAN (>1 Gbps)

```yaml
sync:
  chunk_size_default: 2097152
  max_concurrent_transfers: 20

compression:
  enabled: false  # Network faster than compression
```

## System-Level Tuning

### Linux

**TCP Tuning**:
```bash
# /etc/sysctl.conf
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 67108864
net.ipv4.tcp_wmem = 4096 65536 67108864
net.core.netdev_max_backlog = 5000
net.ipv4.tcp_congestion_control = bbr

# Apply
sudo sysctl -p
```

**File Descriptors**:
```bash
# /etc/security/limits.conf
p2psync soft nofile 65536
p2psync hard nofile 65536

# /etc/systemd/system/p2p-sync.service
[Service]
LimitNOFILE=65536
```

**I/O Scheduler**:
```bash
# For SSD
echo none > /sys/block/sda/queue/scheduler

# For HDD
echo deadline > /sys/block/sda/queue/scheduler
```

### macOS

**Network Buffers**:
```bash
sudo sysctl -w kern.ipc.maxsockbuf=16777216
sudo sysctl -w net.inet.tcp.sendspace=1048576
sudo sysctl -w net.inet.tcp.recvspace=1048576
```

**File Descriptors**:
```bash
sudo launchctl limit maxfiles 65536 200000
ulimit -n 65536
```

## Database Optimization

### SQLite Tuning

```bash
# Check current settings
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA compile_options;"

# Optimize
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db <<EOF
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=-64000;  -- 64MB cache
PRAGMA temp_store=MEMORY;
PRAGMA mmap_size=268435456;  -- 256MB mmap
ANALYZE;
EOF
```

### Regular Maintenance

```bash
#!/bin/bash
# optimize-db.sh
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db <<EOF
-- Reclaim space
VACUUM;

-- Update statistics
ANALYZE;

-- Check integrity
PRAGMA integrity_check;
EOF
```

Run monthly via cron:
```bash
0 3 1 * * /usr/local/bin/optimize-db.sh
```

## Monitoring Performance

### Real-Time Metrics

```bash
# Watch metrics
watch -n 1 'curl -s localhost:9090/metrics | grep -E "sync_operation|network_message|resource"'

# Specific metrics
curl localhost:9090/metrics | grep sync_operations_total
curl localhost:9090/metrics | grep sync_operation_duration_seconds
curl localhost:9090/metrics | grep compression_ratio
```

### Prometheus Queries

```promql
# Average sync duration
rate(sync_operation_duration_seconds_sum[5m]) / rate(sync_operation_duration_seconds_count[5m])

# Throughput (bytes/sec)
rate(sync_file_transfer_bytes_total[1m])

# Compression efficiency
avg(compression_ratio)

# Active transfers
sync_active_transfers
```

### Grafana Dashboards

Key panels to monitor:
1. **Sync Operations Rate** (operations/sec)
2. **File Transfer Throughput** (MB/s)
3. **Operation Latency** (p50, p95, p99)
4. **Active Connections** (gauge)
5. **Compression Ratio** (histogram)
6. **CPU & Memory Usage** (system metrics)

## Benchmarking

### Baseline Test

```bash
#!/bin/bash
# benchmark.sh

SYNC_DIR="/tmp/benchmark-sync"
mkdir -p "$SYNC_DIR"

# Create test files
echo "Creating test files..."
for i in {1..100}; do
    dd if=/dev/urandom of="$SYNC_DIR/file_${i}.dat" bs=1M count=10 2>/dev/null
done

# Start timer
START=$(date +%s)

# Trigger sync (file creation)
echo "Syncing..."

# Wait for completion (check metrics)
while true; do
    ACTIVE=$(curl -s localhost:9090/metrics | grep sync_active_transfers | awk '{print $2}')
    if [ "$ACTIVE" == "0" ]; then
        break
    fi
    sleep 1
done

# Calculate duration
END=$(date +%s)
DURATION=$((END - START))
TOTAL_SIZE=$((100 * 10))  # 100 files * 10MB

echo "Benchmark Results:"
echo "  Files: 100"
echo "  Total Size: ${TOTAL_SIZE}MB"
echo "  Duration: ${DURATION}s"
echo "  Throughput: $((TOTAL_SIZE / DURATION))MB/s"
```

### Stress Test

```bash
# Create many files simultaneously
for i in {1..1000}; do
    echo "File $i" > "/tmp/sync/file_$i.txt" &
done
wait

# Monitor system during sync
dstat -tcmndy 1 60
```

## Troubleshooting Performance Issues

### High CPU Usage

**Symptoms**: CPU >80%, slow sync

**Causes**:
1. Too many concurrent operations
2. Expensive compression
3. Excessive hashing

**Solutions**:
```yaml
sync:
  max_concurrent_transfers: 3

compression:
  algorithm: "lz4"
  level: 1
```

### High Memory Usage

**Symptoms**: Memory >2GB, OOM errors

**Causes**:
1. Large chunk buffers
2. Too many concurrent transfers
3. Memory leaks (report bug)

**Solutions**:
```yaml
sync:
  chunk_size_default: 262144  # 256KB
  max_concurrent_transfers: 3
```

### Slow Synchronization

**Symptoms**: Sync takes >5s for small files

**Causes**:
1. Network latency
2. Slow disk I/O
3. Database contention

**Solutions**:
```bash
# Check disk performance
sudo hdparm -tT /dev/sda

# Check database lock
lsof /var/lib/p2p-sync/data/p2p_sync.db

# Enable profiling
LOG_LEVEL=debug p2p-sync -config config.yaml
```

### Network Bottleneck

**Symptoms**: Low throughput despite fast network

**Solutions**:
```bash
# Check network speed
iperf3 -s  # On one peer
iperf3 -c <peer-ip>  # On another

# Verify MTU
ip link show | grep mtu

# Check for packet loss
ping -c 100 <peer-ip>
```

## Performance Best Practices

1. **Use SSD for database**: 10x faster than HDD
2. **Dedicated network**: Isolate sync traffic
3. **Monitor continuously**: Catch regressions early
4. **Benchmark after changes**: Validate improvements
5. **Scale horizontally**: Add peers for redundancy, not speed
6. **Optimize OS first**: Tune before application
7. **Profile bottlenecks**: Measure, don't guess
8. **Test with real workload**: Synthetic tests miss edge cases

## Advanced Optimization

### CPU Affinity

```bash
# Pin to specific cores
taskset -c 0-3 p2p-sync -config config.yaml
```

### NUMA Awareness

```bash
# Check NUMA topology
numactl --hardware

# Run on specific NUMA node
numactl --cpunodebind=0 --membind=0 p2p-sync -config config.yaml
```

### Huge Pages

```bash
# Enable huge pages
echo 512 > /proc/sys/vm/nr_hugepages

# Verify
grep HugePages /proc/meminfo
```

## Next Steps

- [Monitoring Guide](monitoring.md) - Set up comprehensive monitoring
- [Troubleshooting](../../TROUBLESHOOTING.md) - Resolve common issues
- [Architecture](../../ARCHITECTURE.md) - Understand system internals

---

**Last Updated**: January 2025
