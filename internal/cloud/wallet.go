package cloud

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WalletConfig holds the user's wallet credentials for x402 payments.
type WalletConfig struct {
	Address    string `json:"address"`
	PrivateKey string `json:"-"` // loaded from env, never persisted to disk
}

// walletFile is the JSON structure stored in ~/.flutterprobe/wallet.json.
type walletFile struct {
	Address string `json:"address"`
}

// paymentHeader matches the server-side PaymentHeader structure.
type paymentHeader struct {
	Signature string `json:"signature"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	Sender    string `json:"sender"`
	TxHash    string `json:"tx_hash,omitempty"`
}

// LoadWallet loads the wallet address from ~/.flutterprobe/wallet.json and
// the private key from the FLUTTERPROBE_WALLET_KEY environment variable.
func LoadWallet(configDir string) (*WalletConfig, error) {
	path := filepath.Join(configDir, "wallet.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wallet not configured: run 'probe config set wallet <ADDRESS>' first")
	}

	var wf walletFile
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("invalid wallet.json: %w", err)
	}
	if wf.Address == "" {
		return nil, fmt.Errorf("wallet.json has no address")
	}

	privKey := os.Getenv("FLUTTERPROBE_WALLET_KEY")
	if privKey == "" {
		return nil, fmt.Errorf("FLUTTERPROBE_WALLET_KEY env var not set")
	}

	return &WalletConfig{
		Address:    wf.Address,
		PrivateKey: privKey,
	}, nil
}

// SaveWallet stores the wallet address in ~/.flutterprobe/wallet.json.
func SaveWallet(configDir string, address string) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	wf := walletFile{Address: address}
	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(configDir, "wallet.json")
	return os.WriteFile(path, data, 0600)
}

// SignPayment creates an EIP-712 typed data signature for an x402 payment
// and returns the base64-encoded JSON payment header.
func (w *WalletConfig) SignPayment(amount, currency, network, receiver string) (string, error) {
	privKeyBytes, err := hex.DecodeString(strings.TrimPrefix(w.PrivateKey, "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid private key hex: %w", err)
	}

	ecdhKey, err := ecdh.P256().NewPrivateKey(privKeyBytes)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(ecdhKey)
	if err != nil {
		return "", fmt.Errorf("marshaling key: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return "", fmt.Errorf("parsing key: %w", err)
	}
	privKey := parsed.(*ecdsa.PrivateKey)

	// Build the EIP-712 hash.
	msgHash := buildEIP712Hash(amount, currency, network, receiver)

	// Sign with ECDSA.
	r, s, err := ecdsa.Sign(rand.Reader, privKey, msgHash)
	if err != nil {
		return "", fmt.Errorf("signing: %w", err)
	}

	// Encode as 65-byte Ethereum-style signature: r (32) + s (32) + v (1).
	sig := make([]byte, 65)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)
	sig[64] = 27 // v = 27 (even y parity)

	ph := paymentHeader{
		Signature: "0x" + hex.EncodeToString(sig),
		Amount:    amount,
		Currency:  currency,
		Network:   network,
		Sender:    w.Address,
	}

	jsonData, err := json.Marshal(ph)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// buildEIP712Hash constructs the EIP-712 typed data hash for an x402 payment.
// This must match the server-side implementation.
func buildEIP712Hash(amount, currency, network, receiver string) []byte {
	domainSep := sha256Sum([]byte("EIP712Domain(string name,string version)"))
	nameHash := sha256Sum([]byte("x402"))
	versionHash := sha256Sum([]byte("1"))

	domainData := make([]byte, 0, 96)
	domainData = append(domainData, domainSep...)
	domainData = append(domainData, nameHash...)
	domainData = append(domainData, versionHash...)
	domainHash := sha256Sum(domainData)

	typeHash := sha256Sum([]byte("Payment(string amount,string currency,string network,string receiver)"))

	paymentData := make([]byte, 0, 160)
	paymentData = append(paymentData, typeHash...)
	paymentData = append(paymentData, sha256Sum([]byte(amount))...)
	paymentData = append(paymentData, sha256Sum([]byte(currency))...)
	paymentData = append(paymentData, sha256Sum([]byte(network))...)
	paymentData = append(paymentData, sha256Sum([]byte(receiver))...)
	structHash := sha256Sum(paymentData)

	msg := make([]byte, 0, 66)
	msg = append(msg, 0x19, 0x01)
	msg = append(msg, domainHash...)
	msg = append(msg, structHash...)

	return sha256Sum(msg)
}

// sha256Sum computes SHA-256. Must match server-side keccak256 stand-in.
func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// ConfigDir returns the default FlutterProbe config directory (~/.flutterprobe).
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".flutterprobe"), nil
}

// PaymentRequirement is parsed from a 402 response body.
type PaymentRequirement struct {
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	Network     string `json:"network"`
	Receiver    string `json:"receiver"`
	Description string `json:"description"`
	ExpiresAt   string `json:"expires_at"`
}

// IsExpired checks if the payment requirement has expired.
func (pr *PaymentRequirement) IsExpired() bool {
	t, err := time.Parse(time.RFC3339, pr.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t)
}
