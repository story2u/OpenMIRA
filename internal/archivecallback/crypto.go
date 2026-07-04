package archivecallback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CryptoDecryptor implements the WeCom callback AES-CBC envelope.
type CryptoDecryptor struct{}

// Decrypt verifies the SHA1 signature and decrypts one encrypted callback body.
func (CryptoDecryptor) Decrypt(token string, aesKey string, signature string, timestamp string, nonce string, encrypt string) (string, string, error) {
	token = strings.TrimSpace(token)
	aesKey = strings.TrimSpace(aesKey)
	signature = strings.TrimSpace(signature)
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)
	encrypt = strings.TrimSpace(encrypt)
	if token == "" || aesKey == "" {
		return "", "", fmt.Errorf("callback token/aes key is empty")
	}
	if signature == "" || timestamp == "" || nonce == "" || encrypt == "" {
		return "", "", fmt.Errorf("callback signature inputs are incomplete")
	}
	expected := CallbackSignature(token, timestamp, nonce, encrypt)
	if expected != signature {
		return "", "", fmt.Errorf("signature mismatch")
	}
	key, err := DecodeEncodingAESKey(aesKey)
	if err != nil {
		return "", "", err
	}
	cipherText, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return "", "", fmt.Errorf("encrypt base64 decode failed: %w", err)
	}
	if len(cipherText) == 0 || len(cipherText)%aes.BlockSize != 0 {
		return "", "", fmt.Errorf("encrypted payload is not AES block aligned")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	plainPadded := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plainPadded, cipherText)
	plain, err := pkcs7Unpad(plainPadded, aes.BlockSize)
	if err != nil {
		return "", "", err
	}
	if len(plain) < 20 {
		return "", "", fmt.Errorf("decrypted payload is too short")
	}
	messageLength := int(binary.BigEndian.Uint32(plain[16:20]))
	messageStart := 20
	messageEnd := messageStart + messageLength
	if messageLength < 0 || messageEnd > len(plain) {
		return "", "", fmt.Errorf("decrypted message length is invalid")
	}
	message := string(plain[messageStart:messageEnd])
	receiveID := string(plain[messageEnd:])
	return message, receiveID, nil
}

// CallbackSignature mirrors WeCom callback SHA1 signature calculation.
func CallbackSignature(token string, timestamp string, nonce string, encrypt string) string {
	values := []string{
		strings.TrimSpace(token),
		strings.TrimSpace(timestamp),
		strings.TrimSpace(nonce),
		strings.TrimSpace(encrypt),
	}
	sort.Strings(values)
	sum := sha1.Sum([]byte(strings.Join(values, "")))
	return hex.EncodeToString(sum[:])
}

// DecodeEncodingAESKey decodes WeCom's 43-character EncodingAESKey.
func DecodeEncodingAESKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("encoding aes key is empty")
	}
	for len(value)%4 != 0 {
		value += "="
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("encoding aes key decode failed: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encoding aes key length = %d, want 32", len(key))
	}
	return key, nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || blockSize <= 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid pkcs7 padded payload")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	if !bytes.Equal(data[len(data)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, fmt.Errorf("invalid pkcs7 padding bytes")
	}
	return data[:len(data)-padding], nil
}
