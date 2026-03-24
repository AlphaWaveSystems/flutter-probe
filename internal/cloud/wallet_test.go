package cloud_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
)

func TestSaveAndLoadWallet(t *testing.T) {
	dir := t.TempDir()

	if err := cloud.SaveWallet(dir, "0xABCD1234"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file was created with correct permissions
	path := filepath.Join(dir, "wallet.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions: got %o, want 0600", info.Mode().Perm())
	}

	// Set env var for private key
	t.Setenv("FLUTTERPROBE_WALLET_KEY", "deadbeef")

	w, err := cloud.LoadWallet(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if w.Address != "0xABCD1234" {
		t.Errorf("address: %q", w.Address)
	}
	if w.PrivateKey != "deadbeef" {
		t.Errorf("private key: %q", w.PrivateKey)
	}
}

func TestLoadWallet_MissingFile(t *testing.T) {
	_, err := cloud.LoadWallet(t.TempDir())
	if err == nil {
		t.Error("expected error for missing wallet file")
	}
}

func TestLoadWallet_MissingEnvVar(t *testing.T) {
	dir := t.TempDir()
	if err := cloud.SaveWallet(dir, "0x1234"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FLUTTERPROBE_WALLET_KEY", "")

	_, err := cloud.LoadWallet(dir)
	if err == nil {
		t.Error("expected error when env var is empty")
	}
}

func TestLoadWallet_EmptyAddress(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"address":""}`)
	if err := os.WriteFile(filepath.Join(dir, "wallet.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FLUTTERPROBE_WALLET_KEY", "key")

	_, err := cloud.LoadWallet(dir)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestLoadWallet_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wallet.json"), []byte("{invalid}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := cloud.LoadWallet(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveWallet_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	if err := cloud.SaveWallet(dir, "0x1234"); err != nil {
		t.Fatalf("save should create dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wallet.json")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestSignPayment(t *testing.T) {
	// Use a valid P-256 private key (32 bytes)
	// This is a test key, not used for any real purpose
	w := &cloud.WalletConfig{
		Address:    "0xTestAddress",
		PrivateKey: "0x" + "a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1",
	}

	result, err := w.SignPayment("100", "USDC", "base", "0xReceiver")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Result should be base64-encoded JSON
	decoded, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	var header struct {
		Signature string `json:"signature"`
		Amount    string `json:"amount"`
		Currency  string `json:"currency"`
		Network   string `json:"network"`
		Sender    string `json:"sender"`
	}
	if err := json.Unmarshal(decoded, &header); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if header.Amount != "100" {
		t.Errorf("amount: %q", header.Amount)
	}
	if header.Currency != "USDC" {
		t.Errorf("currency: %q", header.Currency)
	}
	if header.Network != "base" {
		t.Errorf("network: %q", header.Network)
	}
	if header.Sender != "0xTestAddress" {
		t.Errorf("sender: %q", header.Sender)
	}
	if header.Signature == "" || header.Signature[:2] != "0x" {
		t.Errorf("signature should start with 0x: %q", header.Signature)
	}
	// Signature should be hex-encoded 65 bytes = "0x" + 130 hex chars
	if len(header.Signature) != 132 {
		t.Errorf("signature length: got %d, want 132", len(header.Signature))
	}
}

func TestSignPayment_InvalidKey(t *testing.T) {
	w := &cloud.WalletConfig{
		Address:    "0xAddr",
		PrivateKey: "not-hex",
	}
	_, err := w.SignPayment("100", "USDC", "base", "0xReceiver")
	if err == nil {
		t.Error("expected error for invalid hex key")
	}
}

func TestPaymentRequirement_IsExpired(t *testing.T) {
	// Expired
	pr := cloud.PaymentRequirement{
		ExpiresAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	if !pr.IsExpired() {
		t.Error("should be expired")
	}

	// Not expired
	pr.ExpiresAt = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	if pr.IsExpired() {
		t.Error("should not be expired")
	}

	// Invalid date = treated as expired
	pr.ExpiresAt = "not-a-date"
	if !pr.IsExpired() {
		t.Error("invalid date should be treated as expired")
	}

	// Empty = treated as expired
	pr.ExpiresAt = ""
	if !pr.IsExpired() {
		t.Error("empty date should be treated as expired")
	}
}

func TestConfigDir(t *testing.T) {
	dir, err := cloud.ConfigDir()
	if err != nil {
		t.Fatalf("config dir: %v", err)
	}
	if dir == "" {
		t.Error("config dir should not be empty")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("config dir should be absolute: %q", dir)
	}
}
