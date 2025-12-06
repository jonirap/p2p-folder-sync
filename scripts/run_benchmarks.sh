#!/bin/bash

# P2P Folder Sync - Benchmark Script
# This script runs comprehensive performance benchmarks with Docker

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
REPORT_DIR="$PROJECT_ROOT/benchmark_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=================================================="
echo "P2P Folder Sync - Performance Benchmark Suite"
echo "=================================================="
echo "Timestamp: $(date)"
echo ""

# Create report directory
mkdir -p "$REPORT_DIR"

# Function to cleanup
cleanup() {
    echo ""
    echo "Cleaning up Docker resources..."
    docker compose -f "$PROJECT_ROOT/docker-compose.benchmark.yml" down -v 2>/dev/null || true
}

trap cleanup EXIT

# Function to wait for services
wait_for_services() {
    echo "Waiting for services to be ready..."
    sleep 10

    # Check if containers are running
    if ! docker compose -f "$PROJECT_ROOT/docker-compose.benchmark.yml" ps | grep -q "Up"; then
        echo "ERROR: Services failed to start"
        docker compose -f "$PROJECT_ROOT/docker-compose.benchmark.yml" logs
        exit 1
    fi

    echo "Services ready!"
}

# Function to create test file
create_test_file() {
    local size=$1
    local filename=$2
    local content_char=${3:-"X"}

    if ! is_container_running "p2p-folder-sync-peer-alpha-1"; then
        echo "ERROR: peer-alpha-1 container is not running"
        return 1
    fi

    docker exec p2p-folder-sync-peer-alpha-1 sh -c "dd if=/dev/zero bs=$size count=1 2>/dev/null | tr '\0' '$content_char' > /app/sync/$filename" 2>/dev/null
}

# Function to check if container is running
is_container_running() {
    local container_name=$1
    if docker inspect "$container_name" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
        return 0
    else
        return 1
    fi
}

# Function to check file sync
check_file_synced() {
    local filename=$1
    local expected_size=$2

    # Check if container is running first
    if ! is_container_running "p2p-folder-sync-peer-beta-1"; then
        echo "ERROR: peer-beta-1 container is not running"
        return 2
    fi

    actual_size=$(docker exec p2p-folder-sync-peer-beta-1 sh -c "stat -c %s /app/sync/$filename 2>/dev/null || echo 0" 2>/dev/null)

    # Handle empty response
    if [ -z "$actual_size" ]; then
        actual_size=0
    fi

    if [ "$actual_size" -eq "$expected_size" 2>/dev/null ]; then
        return 0
    else
        return 1
    fi
}

# Function to wait for file sync
wait_for_file_sync() {
    local filename=$1
    local expected_size=$2
    local max_wait=${3:-60}

    local waited=0
    while [ $waited -lt $max_wait ]; do
        check_file_synced "$filename" "$expected_size"
        local result=$?

        if [ $result -eq 0 ]; then
            return 0
        elif [ $result -eq 2 ]; then
            echo "FATAL: Container crashed, aborting benchmark"
            # Show container logs for debugging
            echo "=== Container Logs ==="
            docker logs p2p-folder-sync-peer-beta-1 2>&1 | tail -20 || true
            docker logs p2p-folder-sync-peer-alpha-1 2>&1 | tail -20 || true
            return 2
        fi

        sleep 0.5
        waited=$((waited + 1))
    done

    echo "WARNING: File $filename did not sync within ${max_wait}s"
    return 1
}

# Function to run benchmark test
run_benchmark() {
    local test_name=$1
    local num_files=$2
    local file_size=$3
    local file_prefix=$4

    echo ""
    echo "Running: $test_name"
    echo "  Files: $num_files, Size: $file_size bytes each"

    # Clear sync directories
    docker exec p2p-folder-sync-peer-alpha-1 sh -c "rm -f /app/sync/${file_prefix}*" 2>/dev/null || true
    docker exec p2p-folder-sync-peer-beta-1 sh -c "rm -f /app/sync/${file_prefix}*" 2>/dev/null || true
    sleep 2

    # Start benchmark
    start_time=$(date +%s.%N)

    # Create files
    for i in $(seq 1 $num_files); do
        filename="${file_prefix}_$(printf "%04d" $i).dat"
        create_test_file $file_size "$filename" &
    done
    wait

    creation_time=$(date +%s.%N)

    # Wait for all files to sync
    synced=0
    for i in $(seq 1 $num_files); do
        filename="${file_prefix}_$(printf "%04d" $i).dat"
        wait_for_file_sync "$filename" "$file_size" 120
        sync_result=$?

        if [ $sync_result -eq 0 ]; then
            synced=$((synced + 1))
        elif [ $sync_result -eq 2 ]; then
            echo "ERROR: Container failure detected. Benchmark cannot continue."
            exit 1
        fi
    done

    end_time=$(date +%s.%N)

    # Calculate metrics
    creation_duration=$(echo "$creation_time - $start_time" | bc)
    sync_duration=$(echo "$end_time - $creation_time" | bc)
    total_duration=$(echo "$end_time - $start_time" | bc)

    total_bytes=$((num_files * file_size))
    total_mb=$(echo "scale=2; $total_bytes / 1024 / 1024" | bc)

    # Calculate rates
    if [ $(echo "$sync_duration > 0" | bc) -eq 1 ]; then
        mb_per_sec=$(echo "scale=2; $total_mb / $sync_duration" | bc)
        files_per_sec=$(echo "scale=2; $synced / $sync_duration" | bc)

        # Estimate chunks (assuming 512KB chunk size)
        chunk_size=524288
        total_chunks=$(( (total_bytes + chunk_size - 1) / chunk_size ))
        chunks_per_sec=$(echo "scale=2; $total_chunks / $sync_duration" | bc)
    else
        mb_per_sec="N/A"
        files_per_sec="N/A"
        chunks_per_sec="N/A"
        total_chunks=0
    fi

    # Print results
    echo "  Results:"
    echo "    Files synced: $synced/$num_files"
    echo "    Total data: ${total_mb} MB"
    echo "    Creation time: ${creation_duration}s"
    echo "    Sync time: ${sync_duration}s"
    echo "    Total time: ${total_duration}s"
    echo "    Throughput: ${mb_per_sec} MB/s"
    echo "    Files/s: ${files_per_sec}"
    echo "    Chunks/s: ${chunks_per_sec}"
    echo "    Total chunks: ${total_chunks}"

    # Return results as CSV
    echo "$test_name,$num_files,$file_size,$synced,$total_bytes,$sync_duration,$mb_per_sec,$files_per_sec,$chunks_per_sec,$total_chunks"
}

# Start Docker Compose
echo "Starting Docker Compose..."
docker compose -f "$PROJECT_ROOT/docker-compose.benchmark.yml" up -d --build

wait_for_services

# Verify containers are actually running
echo ""
echo "Verifying containers are running..."
if ! is_container_running "p2p-folder-sync-peer-alpha-1"; then
    echo "ERROR: peer-alpha-1 container failed to start or crashed"
    echo "=== Container Logs ==="
    docker logs p2p-folder-sync-peer-alpha-1 2>&1 || true
    exit 1
fi

if ! is_container_running "p2p-folder-sync-peer-beta-1"; then
    echo "ERROR: peer-beta-1 container failed to start or crashed"
    echo "=== Container Logs ==="
    docker logs p2p-folder-sync-peer-beta-1 2>&1 || true
    exit 1
fi

echo "All containers verified and running!"

# Initialize results file
RESULTS_CSV="$REPORT_DIR/benchmark_${TIMESTAMP}.csv"
echo "test_name,num_files,file_size_bytes,files_synced,total_bytes,sync_time_sec,mb_per_sec,files_per_sec,chunks_per_sec,total_chunks" > "$RESULTS_CSV"

echo ""
echo "=================================================="
echo "Running Benchmark Tests"
echo "=================================================="

# Test Suite 1: Small files (high file count)
run_benchmark "Small Files (1KB)" 100 1024 "small" >> "$RESULTS_CSV"

# Test Suite 2: Medium files (balanced)
run_benchmark "Medium Files (100KB)" 50 102400 "medium" >> "$RESULTS_CSV"

# Test Suite 3: Large files (throughput test)
run_benchmark "Large Files (1MB)" 20 1048576 "large" >> "$RESULTS_CSV"

# Test Suite 4: Very large files (max throughput)
run_benchmark "XLarge Files (10MB)" 5 10485760 "xlarge" >> "$RESULTS_CSV"

# Test Suite 5: Mixed workload
run_benchmark "Mixed Small" 200 512 "mixed_s" >> "$RESULTS_CSV"
run_benchmark "Mixed Medium" 30 524288 "mixed_m" >> "$RESULTS_CSV"

echo ""
echo "=================================================="
echo "Benchmark Complete!"
echo "=================================================="
echo "Results saved to: $RESULTS_CSV"

# Generate markdown report
REPORT_MD="$REPORT_DIR/benchmark_${TIMESTAMP}.md"

cat > "$REPORT_MD" << 'EOFREPORT'
# P2P Folder Sync - Benchmark Report

**Generated:** $(date)

## Executive Summary

| Test Name | Files | Size/File | MB/s | Files/s | Chunks/s | Total Data | Sync Time |
|-----------|-------|-----------|------|---------|----------|------------|-----------|
EOFREPORT

# Parse CSV and add to markdown (skip header)
tail -n +2 "$RESULTS_CSV" | while IFS=, read -r test_name num_files file_size files_synced total_bytes sync_time mb_s files_s chunks_s total_chunks; do
    # Format file size
    if [ "$file_size" -lt 1024 ]; then
        size_fmt="${file_size}B"
    elif [ "$file_size" -lt 1048576 ]; then
        size_fmt="$(echo "scale=0; $file_size / 1024" | bc)KB"
    else
        size_fmt="$(echo "scale=0; $file_size / 1048576" | bc)MB"
    fi

    # Format total data
    total_mb=$(echo "scale=2; $total_bytes / 1048576" | bc)

    echo "| $test_name | $files_synced | $size_fmt | $mb_s | $files_s | $chunks_s | ${total_mb}MB | ${sync_time}s |" >> "$REPORT_MD"
done

cat >> "$REPORT_MD" << 'EOFREPORT2'

## Test Configuration

- **Docker Network:** Bridge (172.25.0.0/16)
- **Chunk Size:** 512KB
- **Compression:** Zstd level 3 (files > 1MB)
- **Encryption:** AES-256-GCM
- **Max Concurrent Transfers:** 10

## Test Details

### Test Suite Overview

1. **Small Files (1KB):** Tests file sync performance with many small files
2. **Medium Files (100KB):** Balanced test with moderate file sizes
3. **Large Files (1MB):** Throughput test with larger files
4. **XLarge Files (10MB):** Maximum throughput test
5. **Mixed Workload:** Combination of different file sizes

### Metrics Explanation

- **MB/s:** Megabytes per second throughput
- **Files/s:** Number of files synchronized per second
- **Chunks/s:** Number of chunks transferred per second (512KB chunks)

EOFREPORT2

echo ""
echo "Markdown report: $REPORT_MD"
cat "$REPORT_MD"

# Cleanup is handled by trap
