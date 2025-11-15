package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// Metrics holds all application metrics
type Metrics struct {
	// Core sync metrics
	SyncOperationsTotal    metric.Int64Counter
	SyncOperationDuration   metric.Float64Histogram
	SyncFileTransferBytes   metric.Int64Counter
	SyncActiveTransfers     metric.Int64UpDownCounter

	// Compression metrics
	CompressionFilesCompressed    metric.Int64Counter
	CompressionBytesSaved         metric.Int64Counter
	CompressionRatio              metric.Float64Histogram
	CompressionOperationDuration  metric.Float64Histogram

	// Load balancing metrics
	SyncFileRequestsTotal      metric.Int64Counter
	SyncFilesPerPeer           metric.Float64Histogram
	SyncLoadBalanceEfficiency  metric.Float64Gauge

	// Network metrics
	NetworkConnectionsActive    metric.Int64UpDownCounter
	NetworkMessageLatency       metric.Float64Histogram
	NetworkChunkRetransmissions metric.Int64Counter

	// Resource metrics
	ResourceMemoryUsage    metric.Int64Gauge
	ResourceCPUUsage       metric.Float64Gauge
	ResourceDiskUsage      metric.Int64Gauge
	ResourceBandwidthUsage metric.Int64Gauge

	// Error metrics
	ErrorOperationFailures metric.Int64Counter
	ErrorNetworkTimeouts   metric.Int64Counter
	ErrorCorruptionDetected metric.Int64Counter
}

// NewMetrics creates and initializes all metrics
func NewMetrics(meterProvider metric.MeterProvider, serviceName string) (*Metrics, error) {
	meter := meterProvider.Meter(serviceName)

	// Core sync metrics
	syncOperationsTotal, err := meter.Int64Counter(
		"sync_operations_total",
		metric.WithDescription("Total operations processed"),
	)
	if err != nil {
		return nil, err
	}

	syncOperationDuration, err := meter.Float64Histogram(
		"sync_operation_duration",
		metric.WithDescription("Operation processing time in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	syncFileTransferBytes, err := meter.Int64Counter(
		"sync_file_transfer_bytes",
		metric.WithDescription("Bytes transferred per file"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	syncActiveTransfers, err := meter.Int64UpDownCounter(
		"sync_active_transfers",
		metric.WithDescription("Currently active file transfers"),
	)
	if err != nil {
		return nil, err
	}

	// Compression metrics
	compressionFilesCompressed, err := meter.Int64Counter(
		"compression_files_compressed",
		metric.WithDescription("Total files compressed"),
	)
	if err != nil {
		return nil, err
	}

	compressionBytesSaved, err := meter.Int64Counter(
		"compression_bytes_saved",
		metric.WithDescription("Total bytes saved through compression"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	compressionRatio, err := meter.Float64Histogram(
		"compression_ratio",
		metric.WithDescription("Compression ratio (compressed/original)"),
	)
	if err != nil {
		return nil, err
	}

	compressionOperationDuration, err := meter.Float64Histogram(
		"compression_operation_duration",
		metric.WithDescription("Time spent compressing/decompressing in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Load balancing metrics
	syncFileRequestsTotal, err := meter.Int64Counter(
		"sync_file_requests_total",
		metric.WithDescription("Total file requests made by new peers"),
	)
	if err != nil {
		return nil, err
	}

	syncFilesPerPeer, err := meter.Float64Histogram(
		"sync_files_per_peer",
		metric.WithDescription("Number of files requested from each peer"),
	)
	if err != nil {
		return nil, err
	}

	syncLoadBalanceEfficiency, err := meter.Float64Gauge(
		"sync_load_balance_efficiency",
		metric.WithDescription("Load distribution efficiency (0-1)"),
	)
	if err != nil {
		return nil, err
	}

	// Network metrics
	networkConnectionsActive, err := meter.Int64UpDownCounter(
		"network_connections_active",
		metric.WithDescription("Active peer connections"),
	)
	if err != nil {
		return nil, err
	}

	networkMessageLatency, err := meter.Float64Histogram(
		"network_message_latency",
		metric.WithDescription("End-to-end message latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	networkChunkRetransmissions, err := meter.Int64Counter(
		"network_chunk_retransmissions",
		metric.WithDescription("Chunk retransmission count"),
	)
	if err != nil {
		return nil, err
	}

	// Resource metrics
	resourceMemoryUsage, err := meter.Int64Gauge(
		"resource_memory_usage",
		metric.WithDescription("Memory usage in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	resourceCPUUsage, err := meter.Float64Gauge(
		"resource_cpu_usage",
		metric.WithDescription("CPU usage percentage"),
		metric.WithUnit("%"),
	)
	if err != nil {
		return nil, err
	}

	resourceDiskUsage, err := meter.Int64Gauge(
		"resource_disk_usage",
		metric.WithDescription("Disk space usage in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	resourceBandwidthUsage, err := meter.Int64Gauge(
		"resource_bandwidth_usage",
		metric.WithDescription("Network bandwidth usage in bytes per second"),
		metric.WithUnit("By/s"),
	)
	if err != nil {
		return nil, err
	}

	// Error metrics
	errorOperationFailures, err := meter.Int64Counter(
		"error_operation_failures",
		metric.WithDescription("Failed operations by type"),
	)
	if err != nil {
		return nil, err
	}

	errorNetworkTimeouts, err := meter.Int64Counter(
		"error_network_timeouts",
		metric.WithDescription("Network timeout errors"),
	)
	if err != nil {
		return nil, err
	}

	errorCorruptionDetected, err := meter.Int64Counter(
		"error_corruption_detected",
		metric.WithDescription("Data corruption incidents"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		SyncOperationsTotal:          syncOperationsTotal,
		SyncOperationDuration:        syncOperationDuration,
		SyncFileTransferBytes:        syncFileTransferBytes,
		SyncActiveTransfers:          syncActiveTransfers,
		CompressionFilesCompressed:   compressionFilesCompressed,
		CompressionBytesSaved:        compressionBytesSaved,
		CompressionRatio:             compressionRatio,
		CompressionOperationDuration: compressionOperationDuration,
		SyncFileRequestsTotal:        syncFileRequestsTotal,
		SyncFilesPerPeer:             syncFilesPerPeer,
		SyncLoadBalanceEfficiency:    syncLoadBalanceEfficiency,
		NetworkConnectionsActive:      networkConnectionsActive,
		NetworkMessageLatency:        networkMessageLatency,
		NetworkChunkRetransmissions:  networkChunkRetransmissions,
		ResourceMemoryUsage:           resourceMemoryUsage,
		ResourceCPUUsage:              resourceCPUUsage,
		ResourceDiskUsage:             resourceDiskUsage,
		ResourceBandwidthUsage:        resourceBandwidthUsage,
		ErrorOperationFailures:       errorOperationFailures,
		ErrorNetworkTimeouts:          errorNetworkTimeouts,
		ErrorCorruptionDetected:       errorCorruptionDetected,
	}, nil
}

// InitMetricsProvider initializes the OpenTelemetry metrics provider
func InitMetricsProvider(ctx context.Context, endpoint string, serviceName string) (metric.MeterProvider, func() error, error) {
	if endpoint == "" {
		// Return a no-op provider if no endpoint is configured
		return sdkmetric.NewMeterProvider(), func() error { return nil }, nil
	}

	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithInsecure(), // Use WithTLSClientConfig for production
	)
	if err != nil {
		return nil, nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	)

	otel.SetMeterProvider(mp)

	return mp, func() error {
		return mp.Shutdown(ctx)
	}, nil
}

