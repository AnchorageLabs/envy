package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "fmt"
    "io"
    "os"
    "sync"
)

const (
    // EnvMasterKeyB64 is the environment variable containing the base64-encoded
    // 32-byte AES-256 master key used to wrap per-row data keys.
    EnvMasterKeyB64 = "ENVY_MASTER_KEY_B64"

    envelopeVersion byte = 1

    masterKeyLen = 32
    dataKeyLen   = 32
    nonceLen     = 12
    gcmTagLen    = 16

    wrappedDataKeyLen = dataKeyLen + gcmTagLen

    frameVersionLen = 1
    frameMinLen     = frameVersionLen + nonceLen + wrappedDataKeyLen + nonceLen + gcmTagLen
)

var (
    // ErrNotInitialized is returned when Encrypt or Decrypt is called before a
    // valid master key has been configured.
    ErrNotInitialized = errors.New("envelope crypto master key is not initialized")

    masterKeyMu sync.RWMutex
    masterKey   []byte
)

// InitFromEnv loads and validates ENVY_MASTER_KEY_B64.
//
// The decoded key must be exactly 32 bytes. Call this during API startup and
// fail boot if it returns an error.
func InitFromEnv() error {
    return InitFromBase64(os.Getenv(EnvMasterKeyB64))
}

// MustInitFromEnv loads ENVY_MASTER_KEY_B64 and panics if it is missing or
// invalid. Prefer InitFromEnv where the API startup path can return/log the
// error cleanly.
func MustInitFromEnv() {
    if err := InitFromEnv(); err != nil {
        panic(err)
    }
}

// InitFromBase64 decodes and installs a base64-encoded 32-byte master key.
func InitFromBase64(encoded string) error {
    if encoded == "" {
        return fmt.Errorf("%s is required and must decode to exactly 32 bytes", EnvMasterKeyB64)
    }

    decoded, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return fmt.Errorf("%s must be valid base64: %w", EnvMasterKeyB64, err)
    }

    return SetMasterKey(decoded)
}

// SetMasterKey installs a raw 32-byte master key. The key is copied before it
// is stored so callers may safely reuse or zero their input buffer.
func SetMasterKey(key []byte) error {
    if len(key) != masterKeyLen {
        return fmt.Errorf("%s must decode to exactly 32 bytes, got %d bytes", EnvMasterKeyB64, len(key))
    }

    keyCopy := make([]byte, masterKeyLen)
    copy(keyCopy, key)

    masterKeyMu.Lock()
    defer masterKeyMu.Unlock()

    if masterKey != nil {
        zero(masterKey)
    }
    masterKey = keyCopy

    return nil
}

// Encrypt encrypts plaintext with a random per-value data key, then wraps that
// data key with the configured master key.
//
// Frame format:
//   version byte:                  1 byte, currently 1
//   wrapped data-key nonce:        12 bytes
//   wrapped data-key ciphertext:   48 bytes (32-byte data key + 16-byte GCM tag)
//   value nonce:                   12 bytes
//   value ciphertext:              len(plaintext) + 16-byte GCM tag
func Encrypt(plaintext []byte) ([]byte, error) {
    mk, err := masterKeySnapshot()
    if err != nil {
        return nil, err
    }
    defer zero(mk)

    masterGCM, err := gcmForKey(mk)
    if err != nil {
        return nil, err
    }

    dataKey := make([]byte, dataKeyLen)
    if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
        return nil, fmt.Errorf("generate data key: %w", err)
    }
    defer zero(dataKey)

    keyNonce := make([]byte, nonceLen)
    if _, err := io.ReadFull(rand.Reader, keyNonce); err != nil {
        return nil, fmt.Errorf("generate data-key nonce: %w", err)
    }

    wrappedDataKey := masterGCM.Seal(nil, keyNonce, dataKey, nil)

    dataGCM, err := gcmForKey(dataKey)
    if err != nil {
        return nil, err
    }

    valueNonce := make([]byte, nonceLen)
    if _, err := io.ReadFull(rand.Reader, valueNonce); err != nil {
        return nil, fmt.Errorf("generate value nonce: %w", err)
    }

    valueCiphertext := dataGCM.Seal(nil, valueNonce, plaintext, nil)

    frame := make([]byte, 0, frameVersionLen+len(keyNonce)+len(wrappedDataKey)+len(valueNonce)+len(valueCiphertext))
    frame = append(frame, envelopeVersion)
    frame = append(frame, keyNonce...)
    frame = append(frame, wrappedDataKey...)
    frame = append(frame, valueNonce...)
    frame = append(frame, valueCiphertext...)

    return frame, nil
}

// Decrypt decrypts a frame produced by Encrypt.
func Decrypt(frame []byte) ([]byte, error) {
    if len(frame) < frameMinLen {
        return nil, fmt.Errorf("invalid envelope ciphertext: frame too short")
    }
    if frame[0] != envelopeVersion {
        return nil, fmt.Errorf("invalid envelope ciphertext: unsupported version %d", frame[0])
    }

    mk, err := masterKeySnapshot()
    if err != nil {
        return nil, err
    }
    defer zero(mk)

    masterGCM, err := gcmForKey(mk)
    if err != nil {
        return nil, err
    }

    offset := frameVersionLen

    keyNonceEnd := offset + nonceLen
    keyNonce := frame[offset:keyNonceEnd]
    offset = keyNonceEnd

    wrappedDataKeyEnd := offset + wrappedDataKeyLen
    if wrappedDataKeyEnd > len(frame) {
        return nil, fmt.Errorf("invalid envelope ciphertext: wrapped data key truncated")
    }
    wrappedDataKey := frame[offset:wrappedDataKeyEnd]
    offset = wrappedDataKeyEnd

    valueNonceEnd := offset + nonceLen
    if valueNonceEnd > len(frame) {
        return nil, fmt.Errorf("invalid envelope ciphertext: value nonce truncated")
    }
    valueNonce := frame[offset:valueNonceEnd]
    offset = valueNonceEnd

    valueCiphertext := frame[offset:]
    if len(valueCiphertext) < gcmTagLen {
        return nil, fmt.Errorf("invalid envelope ciphertext: value ciphertext truncated")
    }

    dataKey, err := masterGCM.Open(nil, keyNonce, wrappedDataKey, nil)
    if err != nil {
        return nil, fmt.Errorf("decrypt envelope data key: %w", err)
    }
    defer zero(dataKey)

    if len(dataKey) != dataKeyLen {
        return nil, fmt.Errorf("invalid envelope ciphertext: decrypted data key has invalid length")
    }

    dataGCM, err := gcmForKey(dataKey)
    if err != nil {
        return nil, err
    }

    plaintext, err := dataGCM.Open(nil, valueNonce, valueCiphertext, nil)
    if err != nil {
        return nil, fmt.Errorf("decrypt envelope value: %w", err)
    }

    return plaintext, nil
}

func masterKeySnapshot() ([]byte, error) {
    masterKeyMu.RLock()
    defer masterKeyMu.RUnlock()

    if len(masterKey) == 0 {
        return nil, ErrNotInitialized
    }
    if len(masterKey) != masterKeyLen {
        return nil, fmt.Errorf("configured envelope crypto master key has invalid length")
    }

    keyCopy := make([]byte, masterKeyLen)
    copy(keyCopy, masterKey)

    return keyCopy, nil
}

func gcmForKey(key []byte) (cipher.AEAD, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("create AES cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
    }
    if gcm.NonceSize() != nonceLen {
        return nil, fmt.Errorf("unexpected AES-GCM nonce size: got %d", gcm.NonceSize())
    }

    return gcm, nil
}

func zero(b []byte) {
    for i := range b {
        b[i] = 0
    }
}
