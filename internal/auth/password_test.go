package auth

import "testing"

func TestHashPasswordVerifiesPBKDF2Hash(t *testing.T) {
	hash, err := HashPassword(" 1234567890 ")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" || !VerifyPasswordHash(hash, "1234567890") {
		t.Fatalf("hash did not verify: %q", hash)
	}
	if VerifyPasswordHash(hash, "bad-password") {
		t.Fatal("VerifyPasswordHash accepted the wrong password")
	}
}

func TestVerifyPasswordHashRejectsMalformedInput(t *testing.T) {
	if VerifyPasswordHash("", "1234567890") {
		t.Fatal("empty hash verified")
	}
	if VerifyPasswordHash("pbkdf2-sha256$bad$salt$key", "1234567890") {
		t.Fatal("malformed hash verified")
	}
}
