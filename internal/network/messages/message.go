package messages

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message represents a base message
type Message struct {
	ID            string      `json:"id"`
	Type          string      `json:"type"`
	Timestamp     int64       `json:"timestamp"`
	SenderID      string      `json:"sender_id"`
	Payload       interface{} `json:"payload"`
	CorrelationID *string     `json:"correlation_id,omitempty"`
}

// NewMessage creates a new message
func NewMessage(msgType string, senderID string, payload interface{}) *Message {
	return &Message{
		ID:        GenerateMessageID(),
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		SenderID:  senderID,
		Payload:   payload,
	}
}

// WithCorrelationID sets the correlation ID for request/response matching
func (m *Message) WithCorrelationID(correlationID string) *Message {
	m.CorrelationID = &correlationID
	return m
}

// Encode encodes the message to JSON
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage decodes a JSON message
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}
	return &msg, nil
}

// EncodePayload encodes a message payload to JSON bytes
func EncodePayload(payload interface{}) ([]byte, error) {
	return json.Marshal(payload)
}

// DecodePayload decodes JSON bytes into a message payload based on message type
func DecodePayload(data []byte, msgType string) (interface{}, error) {
	var payload interface{}

	switch msgType {
	case TypeDiscovery:
		payload = &DiscoveryMessage{}
	case TypeDiscoveryResponse:
		payload = &DiscoveryResponseMessage{}
	case TypeHandshake:
		payload = &HandshakeMessage{}
	case TypeHandshakeAck:
		payload = &HandshakeChallengeMessage{}
	case TypeHandshakeComplete:
		payload = &HandshakeCompleteMessage{}
	case TypeStateDeclaration:
		payload = &StateDeclarationMessage{}
	case TypeFileRequest:
		payload = &FileRequestMessage{}
	case TypeSyncOperation:
		payload = &LogEntryPayload{}
	case TypeChunk:
		payload = &ChunkMessage{}
	case TypeChunkRequest:
		payload = &ChunkRequestMessage{}
	case TypeOperationAck:
		payload = &OperationAckMessage{}
	case TypeChunkAck:
		payload = &ChunkAckMessage{}
	case TypeHeartbeat:
		payload = &HeartbeatMessage{}
	default:
		return nil, fmt.Errorf("unknown message type: %s", msgType)
	}

	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	return payload, nil
}

// GenerateMessageID generates a unique message ID
func GenerateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}
