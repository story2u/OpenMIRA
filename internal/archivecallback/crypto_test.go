package archivecallback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

func TestCryptoDecryptorDecryptsWeComEnvelope(t *testing.T) {
	token := "token"
	aesKey := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "=")
	plainXML := `<xml><Event>change_external_contact</Event></xml>`
	encrypt := encryptCallbackForTest(t, aesKey, plainXML, "corp-1")
	signature := CallbackSignature(token, "123", "nonce", encrypt)

	plain, receiveID, err := (CryptoDecryptor{}).Decrypt(token, aesKey, signature, "123", "nonce", encrypt)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if plain != plainXML || receiveID != "corp-1" {
		t.Fatalf("plain=%q receive_id=%q", plain, receiveID)
	}
}

func TestCryptoDecryptorRejectsSignatureMismatch(t *testing.T) {
	aesKey := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), "=")
	encrypt := encryptCallbackForTest(t, aesKey, `<xml/>`, "corp-1")
	_, _, err := (CryptoDecryptor{}).Decrypt("token", aesKey, "bad-signature", "123", "nonce", encrypt)
	if err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("error = %v, want signature mismatch", err)
	}
}

func TestDecodeEncodingAESKeyRejectsWrongLength(t *testing.T) {
	_, err := DecodeEncodingAESKey(base64.StdEncoding.EncodeToString([]byte("too-short")))
	if err == nil || !strings.Contains(err.Error(), "length") {
		t.Fatalf("error = %v, want length error", err)
	}
}

func encryptCallbackForTest(t *testing.T, aesKey string, plainXML string, receiveID string) string {
	t.Helper()
	key, err := DecodeEncodingAESKey(aesKey)
	if err != nil {
		t.Fatalf("DecodeEncodingAESKey returned error: %v", err)
	}
	message := []byte(plainXML)
	envelope := bytes.NewBuffer(nil)
	envelope.WriteString("0123456789abcdef")
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(message)))
	envelope.Write(length)
	envelope.Write(message)
	envelope.WriteString(receiveID)
	padded := pkcs7PadForTest(envelope.Bytes(), aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}
	cipherText := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(cipherText, padded)
	return base64.StdEncoding.EncodeToString(cipherText)
}

func pkcs7PadForTest(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	if padding == 0 {
		padding = blockSize
	}
	return append(append([]byte(nil), data...), bytes.Repeat([]byte{byte(padding)}, padding)...)
}
