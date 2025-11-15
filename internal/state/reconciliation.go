package state

import (
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// Reconciler handles state reconciliation between peers
type Reconciler struct {
}

// NewReconciler creates a new state reconciler
func NewReconciler() *Reconciler {
	return &Reconciler{}
}

// PeerState represents the state of a peer
type PeerState struct {
	PeerID       string
	FileManifest []FileManifestEntry
}

// FileManifestEntry represents a file in a peer's manifest
type FileManifestEntry struct {
	FileID string
	Size   int64
}

// PeerCapabilities represents peer capabilities for load balancing
type PeerCapabilities struct {
	SupportsCompression    bool
	MaxConcurrentTransfers int
	BandwidthLimit         int64
}

// AssignFilesToPeers assigns files to peers based on load balancing
func (r *Reconciler) AssignFilesToPeers(peers map[string]*PeerState, files []string) map[string][]string {
	result := make(map[string][]string)

	// Simple round-robin assignment for now
	peerIDs := make([]string, 0, len(peers))
	for peerID := range peers {
		peerIDs = append(peerIDs, peerID)
	}

	// Handle empty peer list
	if len(peerIDs) == 0 {
		return result
	}

	for i, file := range files {
		peerID := peerIDs[i%len(peerIDs)]
		result[peerID] = append(result[peerID], file)
	}

	return result
}

// AssignFilesWithCapacity assigns files considering peer capacity limits
func (r *Reconciler) AssignFilesWithCapacity(peerCapabilities map[string]PeerCapabilities, currentLoad map[string]int, filesToSync []string) map[string][]string {
	result := make(map[string][]string)

	// Simple round-robin assignment ignoring capacity for now
	peerIDs := make([]string, 0, len(peerCapabilities))
	for peerID := range peerCapabilities {
		peerIDs = append(peerIDs, peerID)
	}

	for i, file := range filesToSync {
		peerID := peerIDs[i%len(peerIDs)]
		result[peerID] = append(result[peerID], file)
	}

	return result
}

// ReconciliationResult represents the result of state reconciliation
type ReconciliationResult struct {
	MissingFiles      []messages.FileManifestEntry
	ConflictingFiles  []messages.FileManifestEntry
	PendingOpsToApply []messages.LogEntryPayload
}

// ReconcileStates reconciles two state declarations
func ReconcileStates(local, remote *StateDeclaration) *ReconciliationResult {
	result := &ReconciliationResult{
		MissingFiles:      make([]messages.FileManifestEntry, 0),
		ConflictingFiles:  make([]messages.FileManifestEntry, 0),
		PendingOpsToApply: make([]messages.LogEntryPayload, 0),
	}

	// Build local file map
	localFiles := make(map[string]messages.FileManifestEntry)
	for _, file := range local.FileManifest {
		localFiles[file.FileID] = file
	}

	// Check remote files
	for _, remoteFile := range remote.FileManifest {
		localFile, exists := localFiles[remoteFile.FileID]
		if !exists {
			// File missing locally
			result.MissingFiles = append(result.MissingFiles, remoteFile)
		} else {
			// Check for conflicts (different hash)
			if localFile.Hash != remoteFile.Hash {
				result.ConflictingFiles = append(result.ConflictingFiles, remoteFile)
			}
		}
	}

	// Find pending operations to apply
	// Operations that are in remote but not in local
	localOps := make(map[string]bool)
	for _, op := range local.PendingOperations {
		localOps[op.OperationID] = true
	}

	for _, remoteOp := range remote.PendingOperations {
		if !localOps[remoteOp.OperationID] {
			result.PendingOpsToApply = append(result.PendingOpsToApply, remoteOp)
		}
	}

	return result
}
