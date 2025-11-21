# Troubleshooting Guide

Common issues and their solutions for P2P Folder Sync.

## Quick Diagnostics

Run these commands to gather diagnostic information:

```bash
# Check service status
systemctl status p2p-sync

# View recent logs
journalctl -u p2p-sync --since "10 minutes ago" -n 100

# Check metrics
curl http://localhost:9090/metrics

# Test connectivity to peer
nc -zv <peer-ip> 8080
nc -zuv <peer-ip> 8081

# Check database integrity
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA integrity_check;"

# Check disk space
df -h /var/lib/p2p-sync

# Check process resources
ps aux | grep p2p-sync
top -p $(pgrep p2p-sync)
```

## Common Issues

### 1. Peers Not Discovering Each Other

**Symptoms**:
- Peers don't see each other
- `network_connections_active` metric shows 0
- Logs show "No peers connected"

**Causes & Solutions**:

#### A. Different Subnets
```bash
# Check if peers are on same subnet
ip addr show

# Solution: Use manual peer list
```

Configuration:
```yaml
network:
  peers:
    - "192.168.1.10:8080"
    - "192.168.1.11:8080"
```

#### B. Firewall Blocking Discovery

```bash
# Check firewall status
sudo ufw status
sudo firewall-cmd --list-all

# Solution: Allow UDP port 8081
sudo ufw allow 8081/udp
sudo firewall-cmd --permanent --add-port=8081/udp
sudo firewall-cmd --reload
```

#### C. mDNS Not Working

```bash
# Check if mDNS daemon is running
systemctl status avahi-daemon  # Linux
systemctl status mDNSResponder  # macOS

# Install if missing
sudo apt-get install avahi-daemon  # Ubuntu/Debian
sudo yum install avahi  # CentOS/RHEL
```

#### D. Port Already in Use

```bash
# Check what's using the ports
sudo lsof -i :8080
sudo lsof -i :8081

# Solution: Use different ports
P2P_PORT=9090 P2P_DISCOVERY_PORT=9091 p2p-sync -config config.yaml
```

### 2. Files Not Synchronizing

**Symptoms**:
- Files created but not syncing
- `sync_operations_total` metric not increasing
- No sync operation logs

**Causes & Solutions**:

#### A. File Watcher Not Working

```bash
# Check file watcher status in logs
journalctl -u p2p-sync | grep -i "watcher"

# Check inotify limits (Linux)
cat /proc/sys/fs/inotify/max_user_watches

# Increase if needed
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

#### B. Permissions Issue

```bash
# Check sync folder permissions
ls -la /var/lib/p2p-sync/sync

# Fix permissions
sudo chown -R p2psync:p2psync /var/lib/p2p-sync/sync
sudo chmod 755 /var/lib/p2p-sync/sync
```

#### C. Files in .gitignore or Hidden

By default, hidden files (starting with `.`) may be ignored. Check configuration:

```yaml
sync:
  folder_path: "/var/lib/p2p-sync/sync"
  # Add ignore patterns if needed
```

#### D. Sync Loop (Files Bouncing)

Check logs for rapid sync operations:

```bash
# Check for sync loops
journalctl -u p2p-sync | grep -A 2 "remote" | tail -n 50
```

If files are syncing repeatedly, this indicates a sync loop bug. Report with logs.

### 3. High CPU Usage

**Symptoms**:
- CPU >80% constantly
- System slow
- Fans running high

**Causes & Solutions**:

#### A. Too Much Compression

```yaml
# Reduce compression level
compression:
  algorithm: "lz4"  # Faster than zstd
  level: 1
```

#### B. Too Many Concurrent Operations

```yaml
# Reduce concurrency
sync:
  max_concurrent_transfers: 3
```

#### C. Large Files Being Hashed

Hashing is CPU-intensive for large files. This is normal during initial sync.

```bash
# Monitor hash operations
curl localhost:9090/metrics | grep hashing
```

Wait for initial sync to complete, CPU should normalize.

### 4. High Memory Usage

**Symptoms**:
- Memory >2GB
- Out of memory errors
- System swapping

**Causes & Solutions**:

#### A. Large Chunk Buffers

```yaml
# Reduce chunk size
sync:
  chunk_size_default: 262144  # 256KB instead of 512KB
```

#### B. Too Many Concurrent Transfers

```yaml
sync:
  max_concurrent_transfers: 3
```

#### C. Memory Leak

If memory grows continuously without bound:

```bash
# Monitor memory over time
watch -n 5 'ps aux | grep p2p-sync | grep -v grep'

# Restart service as workaround
sudo systemctl restart p2p-sync
```

Report issue with logs and metrics.

### 5. Database Errors

**Symptoms**:
- "database locked" errors
- "database disk image is malformed"
- Cannot start service

**Solutions**:

#### A. Database Locked

```bash
# Check for other processes
lsof /var/lib/p2p-sync/data/p2p_sync.db

# Kill stale connections
sudo systemctl restart p2p-sync
```

#### B. Database Corruption

```bash
# Stop service
sudo systemctl stop p2p-sync

# Check integrity
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA integrity_check;"

# If corrupted, restore from backup
sudo cp /var/backups/p2p_sync_latest.db /var/lib/p2p-sync/data/p2p_sync.db
sudo chown p2psync:p2psync /var/lib/p2p-sync/data/p2p_sync.db

# If no backup, recover what's possible
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db ".recover" | sqlite3 recovered.db
sudo mv recovered.db /var/lib/p2p-sync/data/p2p_sync.db

# Start service
sudo systemctl start p2p-sync
```

#### C. WAL File Growing

```bash
# Check WAL size
ls -lh /var/lib/p2p-sync/data/p2p_sync.db-wal

# Force checkpoint
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

### 6. Network Issues

**Symptoms**:
- Slow synchronization
- Timeout errors
- Retransmission messages in logs

**Diagnosis**:

```bash
# Check network latency
ping -c 10 <peer-ip>

# Check bandwidth
iperf3 -s  # On peer 1
iperf3 -c <peer-ip>  # On peer 2

# Check packet loss
mtr <peer-ip>

# Check connection state
netstat -anp | grep 8080
```

**Solutions**:

#### A. High Latency (>100ms)

```yaml
network:
  connection_timeout: 120
  heartbeat_interval: 60

sync:
  chunk_size_default: 1048576  # Larger chunks
```

#### B. Packet Loss (>1%)

```bash
# Check for network issues
sudo ethtool eth0 | grep -i error

# Check MTU
ip link show | grep mtu

# Adjust MTU if needed
sudo ip link set dev eth0 mtu 1500
```

#### C. Firewall Interference

```bash
# Temporarily disable to test (DON'T DO IN PRODUCTION)
sudo ufw disable
# Test sync
sudo ufw enable

# If that fixes it, add proper rules
sudo ufw allow from <peer-subnet> to any port 8080 proto tcp
sudo ufw allow from <peer-subnet> to any port 8081 proto udp
```

### 7. File Conflicts

**Symptoms**:
- Files with merge conflict markers
- Different content on different peers
- `conflict_detected` in logs

**Understanding Conflicts**:

P2P Folder Sync detects conflicts when:
1. Two peers edit same file concurrently
2. Vector clocks show concurrent operations
3. File checksums differ

**Resolution**:

#### For Text Files
Conflicts are merged using 3-way merge:

```
<<<<<<< peer-alpha
Content from peer alpha
=======
Content from peer beta
>>>>>>> peer-beta
```

Manually resolve conflicts, then save file.

#### For Binary Files
Last-write-wins (LWW) is applied automatically based on timestamp.

To override:
```bash
# Check which version is kept
grep "conflict.*resolved" /var/log/p2p-sync/p2p-sync.log

# Manually copy preferred version
cp /path/to/preferred/version /var/lib/p2p-sync/sync/file.bin
```

### 8. Disk Space Issues

**Symptoms**:
- "no space left on device"
- Sync paused
- Cannot write files

**Solutions**:

```bash
# Check disk space
df -h /var/lib/p2p-sync

# Find large files
du -h /var/lib/p2p-sync/sync | sort -rh | head -20

# Clean up old logs (if enabled)
sudo journalctl --vacuum-time=7d

# Compact database
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "VACUUM;"

# Add more space or move sync folder
# Edit config.yaml to point to larger volume
```

### 9. Service Won't Start

**Symptoms**:
- `systemctl start p2p-sync` fails
- Immediate exit
- Error in logs

**Diagnosis**:

```bash
# Check service status
systemctl status p2p-sync -l

# Check logs
journalctl -u p2p-sync -n 50

# Try running manually
sudo -u p2psync p2p-sync -config /etc/p2p-sync/config.yaml
```

**Common Causes**:

#### A. Configuration Error

```bash
# Validate YAML syntax
python3 -c "import yaml; yaml.safe_load(open('/etc/p2p-sync/config.yaml'))"

# Check for required fields
grep "folder_path" /etc/p2p-sync/config.yaml
```

#### B. Port Already in Use

```bash
# Find conflicting process
sudo lsof -i :8080

# Kill or use different port
```

#### C. Permission Denied

```bash
# Check file ownership
ls -la /var/lib/p2p-sync/
ls -la /etc/p2p-sync/config.yaml

# Fix permissions
sudo chown -R p2psync:p2psync /var/lib/p2p-sync
sudo chmod 640 /etc/p2p-sync/config.yaml
```

### 10. Slow Initial Sync

**Symptoms**:
- New peer takes hours to sync
- Low throughput during initial sync

**This is Expected**:
Initial sync of large datasets takes time. Monitor progress:

```bash
# Watch sync progress
watch -n 5 'curl -s localhost:9090/metrics | grep sync_file_transfer_bytes_total'

# Check active transfers
curl localhost:9090/metrics | grep sync_active_transfers
```

**Optimization**:

```yaml
sync:
  max_concurrent_transfers: 10  # Increase for faster initial sync

compression:
  algorithm: "lz4"  # Faster compression
  level: 1
```

## Debugging Tools

### Enable Debug Logging

```bash
# Temporarily
LOG_LEVEL=debug p2p-sync -config config.yaml

# Permanently
```

```yaml
observability:
  log_level: "debug"
```

### Capture Network Traffic

```bash
# Capture sync traffic
sudo tcpdump -i eth0 -w p2p-sync.pcap port 8080 or port 8081

# Analyze with wireshark
wireshark p2p-sync.pcap
```

### Profile Performance

```bash
# CPU profiling (requires pprof enabled)
go tool pprof http://localhost:6060/debug/pprof/profile

# Memory profiling
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Generate Diagnostic Bundle

```bash
#!/bin/bash
# collect-diagnostics.sh

OUTDIR="p2p-sync-diagnostics-$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUTDIR"

# System info
uname -a > "$OUTDIR/system.txt"
df -h > "$OUTDIR/disk.txt"
free -h > "$OUTDIR/memory.txt"

# Service status
systemctl status p2p-sync > "$OUTDIR/service-status.txt"
journalctl -u p2p-sync -n 1000 > "$OUTDIR/logs.txt"

# Configuration
cp /etc/p2p-sync/config.yaml "$OUTDIR/config.yaml"

# Metrics
curl -s localhost:9090/metrics > "$OUTDIR/metrics.txt"

# Network
netstat -tunlp > "$OUTDIR/network.txt"
ss -s > "$OUTDIR/sockets.txt"

# Database stats
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db ".tables" > "$OUTDIR/db-tables.txt"
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db ".schema" > "$OUTDIR/db-schema.txt"

# Create tarball
tar czf "$OUTDIR.tar.gz" "$OUTDIR"
echo "Diagnostics saved to $OUTDIR.tar.gz"
```

## Getting Help

### Before Reporting Issues

1. **Search existing issues**: Check GitHub issues for similar problems
2. **Collect diagnostics**: Run diagnostic bundle script
3. **Reproduce**: Try to reproduce on clean environment
4. **Minimal config**: Test with minimal configuration

### Reporting Bugs

Include:
- P2P Sync version (`p2p-sync -version`)
- Operating system and version
- Configuration file (redact secrets)
- Relevant logs (last 100 lines)
- Steps to reproduce
- Expected vs actual behavior

### Emergency Contacts

- **Production Issues**: Open high-priority GitHub issue
- **Security Issues**: Email security@example.com
- **Questions**: GitHub Discussions

## Preventive Measures

### Regular Maintenance

```bash
# Weekly: Check disk space
df -h /var/lib/p2p-sync

# Weekly: Check database health
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "PRAGMA integrity_check;"

# Monthly: Vacuum database
sqlite3 /var/lib/p2p-sync/data/p2p_sync.db "VACUUM;"

# Monthly: Review logs for errors
journalctl -u p2p-sync --since "30 days ago" | grep -i error

# Quarterly: Update to latest version
# Check release notes first!
```

### Monitoring Alerts

Set up alerts for:
- Service down for >5 minutes
- No peer connections for >10 minutes
- Disk space <10% free
- Error rate >5% of operations
- Memory usage >90%

See [Monitoring Guide](docs/guides/monitoring.md) for Prometheus alert rules.

### Backup Strategy

```bash
# Daily database backup
0 2 * * * /usr/local/bin/backup-p2p-sync-db.sh

# Weekly configuration backup
0 3 * * 0 tar czf /var/backups/p2p-sync-config-$(date +\%Y\%m\%d).tar.gz /etc/p2p-sync
```

## Known Issues

### Issue #1: Docker on WSL2

**Symptom**: Docker tests fail on Windows WSL2
**Cause**: Docker credential helper issues
**Workaround**: Run tests with `--fast` flag to skip Docker tests
**Status**: Known limitation, not a code issue

### Issue #2: Large File Memory Usage

**Symptom**: High memory usage with files >1GB
**Cause**: Chunk buffering in memory
**Workaround**: Reduce chunk size or add more RAM
**Status**: Optimization planned for future release

## Additional Resources

- [Architecture Documentation](ARCHITECTURE.md)
- [API Reference](API_REFERENCE.md)
- [Performance Tuning](docs/guides/performance.md)
- [Deployment Guide](DEPLOYMENT.md)

---

**Last Updated**: January 2025

**Have a question not covered here?** Open a GitHub Discussion.
