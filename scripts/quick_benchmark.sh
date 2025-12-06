#!/bin/bash

# Quick Benchmark - Measures current performance on running containers

echo "=========================================="
echo "P2P Folder Sync - Quick Benchmark"
echo "=========================================="
echo "Timestamp: $(date)"
echo ""

# Test with a single large file
echo "Test 1: Single 5MB file transfer"
echo "-----------------------------------"

# Clean test file
docker exec p2p-folder-sync-peer-alpha-1 rm -f /app/sync/test_5mb.dat 2>/dev/null
docker exec p2p-folder-sync-peer-beta-1 rm -f /app/sync/test_5mb.dat 2>/dev/null
sleep 2

# Create 5MB file and measure sync time
start=$(date +%s.%N)
docker exec p2p-folder-sync-peer-alpha-1 dd if=/dev/zero bs=1048576 count=5 2>/dev/null | docker exec -i p2p-folder-sync-peer-alpha-1 sh -c 'cat > /app/sync/test_5mb.dat'

# Wait for sync
max_wait=60
waited=0
while [ $waited -lt $max_wait ]; do
    size=$(docker exec p2p-folder-sync-peer-beta-1 stat -c %s /app/sync/test_5mb.dat 2>/dev/null || echo 0)
    if [ "$size" -eq 5242880 ]; then
        break
    fi
    sleep 0.5
    waited=$((waited + 1))
done

end=$(date +%s.%N)
duration=$(echo "$end - $start" | bc)

if [ "$size" -eq 5242880 ]; then
    mb_per_sec=$(echo "scale=2; 5 / $duration" | bc)
    chunk_size=524288  # 512KB
    total_chunks=$(( 5242880 / chunk_size ))
    chunks_per_sec=$(echo "scale=2; $total_chunks / $duration" | bc)

    echo "✓ Success!"
    echo "  File size: 5 MB"
    echo "  Sync time: ${duration}s"
    echo "  Throughput: ${mb_per_sec} MB/s"
    echo "  Chunks: ${total_chunks}"
    echo "  Chunks/s: ${chunks_per_sec}"
else
    echo "✗ Failed to sync within ${max_wait}s"
fi

echo ""

# Test with 10 small files
echo "Test 2: 10 x 100KB files"
echo "-----------------------------------"

# Clean test files
docker exec p2p-folder-sync-peer-alpha-1 sh -c 'rm -f /app/sync/quick_*.dat' 2>/dev/null
docker exec p2p-folder-sync-peer-beta-1 sh -c 'rm -f /app/sync/quick_*.dat' 2>/dev/null
sleep 2

start=$(date +%s.%N)

# Create files
for i in $(seq 1 10); do
    docker exec p2p-folder-sync-peer-alpha-1 dd if=/dev/zero bs=102400 count=1 2>/dev/null | docker exec -i p2p-folder-sync-peer-alpha-1 sh -c "cat > /app/sync/quick_$(printf '%02d' $i).dat" &
done
wait

# Wait for all to sync
max_wait=60
waited=0
while [ $waited -lt $max_wait ]; do
    synced=0
    for i in $(seq 1 10); do
        size=$(docker exec p2p-folder-sync-peer-beta-1 stat -c %s /app/sync/quick_$(printf '%02d' $i).dat 2>/dev/null || echo 0)
        if [ "$size" -eq 102400 ]; then
            synced=$((synced + 1))
        fi
    done

    if [ "$synced" -eq 10 ]; then
        break
    fi
    sleep 0.5
    waited=$((waited + 1))
done

end=$(date +%s.%N)
duration=$(echo "$end - $start" | bc)

if [ "$synced" -eq 10 ]; then
    total_mb=$(echo "scale=2; (10 * 102400) / 1048576" | bc)
    mb_per_sec=$(echo "scale=2; $total_mb / $duration" | bc)
    files_per_sec=$(echo "scale=2; 10 / $duration" | bc)
    chunk_size=524288
    chunks_per_file=$(( (102400 + chunk_size - 1) / chunk_size ))
    total_chunks=$((chunks_per_file * 10))
    chunks_per_sec=$(echo "scale=2; $total_chunks / $duration" | bc)

    echo "✓ Success!"
    echo "  Files synced: ${synced}/10"
    echo "  Total size: ${total_mb} MB"
    echo "  Sync time: ${duration}s"
    echo "  Throughput: ${mb_per_sec} MB/s"
    echo "  Files/s: ${files_per_sec}"
    echo "  Total chunks: ${total_chunks}"
    echo "  Chunks/s: ${chunks_per_sec}"
else
    echo "✗ Partial sync: ${synced}/10 files synced in ${max_wait}s"
fi

echo ""
echo "=========================================="
echo "Benchmark Complete"
echo "=========================================="
