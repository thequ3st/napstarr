package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Instance represents this node's identity on the Napstarr network.
type Instance struct {
	PrivateKey ed25519.PrivateKey `json:"-"`
	PublicKey  ed25519.PublicKey  `json:"publicKey"`
	ID         string            `json:"id"`       // base58(SHA-256(pubkey))
	Name       string            `json:"name"`
	CreatedAt  string            `json:"createdAt"`
}

// InstanceInfo is the public-facing identity shared with other nodes.
type InstanceInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"` // hex-encoded
	CreatedAt string `json:"createdAt"`
}

// SignedMessage is any message sent between instances, signed by the sender.
type SignedMessage struct {
	InstanceID string          `json:"instanceId"`
	Timestamp  int64           `json:"timestamp"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	PublicKey  string          `json:"publicKey"` // hex-encoded, for self-verification
	Signature  string          `json:"signature"` // hex-encoded Ed25519 signature
}

// keyFile is the on-disk format for persisting the keypair.
type keyFile struct {
	PrivateKey string `json:"privateKey"` // hex-encoded 64-byte Ed25519 private key
	PublicKey  string `json:"publicKey"`  // hex-encoded 32-byte Ed25519 public key
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
}

// LoadOrCreate loads the instance identity from disk, or generates a new one.
func LoadOrCreate(dataDir string, name string) (*Instance, error) {
	keyPath := filepath.Join(dataDir, "identity.json")

	if data, err := os.ReadFile(keyPath); err == nil {
		return loadFromFile(data, name)
	}

	return generate(keyPath, name)
}

func loadFromFile(data []byte, name string) (*Instance, error) {
	var kf keyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}

	privBytes, err := hex.DecodeString(kf.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	pubBytes, err := hex.DecodeString(kf.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := ed25519.PublicKey(pubBytes)

	// Allow name update
	if name != "" {
		kf.Name = name
	}

	return &Instance{
		PrivateKey: priv,
		PublicKey:  pub,
		ID:         computeID(pub),
		Name:       kf.Name,
		CreatedAt:  kf.CreatedAt,
	}, nil
}

func generate(keyPath string, name string) (*Instance, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if name == "" {
		name = "Napstarr Node"
	}

	inst := &Instance{
		PrivateKey: priv,
		PublicKey:  pub,
		ID:         computeID(pub),
		Name:       name,
		CreatedAt:  now,
	}

	kf := keyFile{
		PrivateKey: hex.EncodeToString(priv),
		PublicKey:  hex.EncodeToString(pub),
		Name:       name,
		CreatedAt:  now,
	}

	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal identity: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if err := os.WriteFile(keyPath, data, 0600); err != nil {
		return nil, fmt.Errorf("write identity: %w", err)
	}

	return inst, nil
}

// computeID returns base58(SHA-256(publicKey)) truncated to 16 chars for readability.
func computeID(pub ed25519.PublicKey) string {
	hash := sha256.Sum256(pub)
	return base58Encode(hash[:])[:16]
}

// Info returns the public-facing instance info.
func (inst *Instance) Info() InstanceInfo {
	return InstanceInfo{
		ID:        inst.ID,
		Name:      inst.Name,
		PublicKey: hex.EncodeToString(inst.PublicKey),
		CreatedAt: inst.CreatedAt,
	}
}

// Sign creates a SignedMessage with the given type and payload.
func (inst *Instance) Sign(msgType string, payload any) (*SignedMessage, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	msg := &SignedMessage{
		InstanceID: inst.ID,
		Timestamp:  time.Now().UnixMilli(),
		Type:       msgType,
		Payload:    payloadBytes,
		PublicKey:  hex.EncodeToString(inst.PublicKey),
	}

	// Sign the deterministic representation: instanceId + timestamp + type + payload
	sigData := fmt.Sprintf("%s:%d:%s:%s", msg.InstanceID, msg.Timestamp, msg.Type, msg.Payload)
	sig := ed25519.Sign(inst.PrivateKey, []byte(sigData))
	msg.Signature = hex.EncodeToString(sig)

	return msg, nil
}

// Verify checks that a SignedMessage was signed by the given public key.
func Verify(msg *SignedMessage, pubKeyHex string) bool {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false
	}
	pub := ed25519.PublicKey(pubBytes)

	// Verify instance ID matches public key
	if computeID(pub) != msg.InstanceID {
		return false
	}

	// Verify signature
	sigBytes, err := hex.DecodeString(msg.Signature)
	if err != nil {
		return false
	}

	sigData := fmt.Sprintf("%s:%d:%s:%s", msg.InstanceID, msg.Timestamp, msg.Type, msg.Payload)
	return ed25519.Verify(pub, []byte(sigData), sigBytes)
}

// Reject messages older than 5 minutes to prevent replay attacks.
func (msg *SignedMessage) IsFresh() bool {
	age := time.Since(time.UnixMilli(msg.Timestamp))
	return age < 5*time.Minute && age > -1*time.Minute
}

// base58Encode encodes bytes to base58 (Bitcoin alphabet).
func base58Encode(data []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	// Simple base58 encoding
	// Convert byte slice to a big integer manually
	result := make([]byte, 0, len(data)*2)
	input := make([]byte, len(data))
	copy(input, data)

	for len(input) > 0 {
		var carry int
		newInput := make([]byte, 0, len(input))
		for _, b := range input {
			carry = carry*256 + int(b)
			if len(newInput) > 0 || carry/58 > 0 {
				newInput = append(newInput, byte(carry/58))
			}
			carry %= 58
		}
		result = append(result, alphabet[carry])
		input = newInput
	}

	// Add leading '1's for leading zero bytes
	for _, b := range data {
		if b != 0 {
			break
		}
		result = append(result, alphabet[0])
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}
