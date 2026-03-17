package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flutterprobe/probe/internal/cloud"
	"github.com/spf13/cobra"
)

// userConfig is the JSON structure stored in ~/.flutterprobe/config.json.
type userConfig struct {
	Wallet   string `json:"wallet,omitempty"`
	AIAPIKey string `json:"ai_api_key,omitempty"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage FlutterProbe user configuration",
	Long: `Manage user-level configuration stored in ~/.flutterprobe/config.json.

This includes wallet addresses for x402 payments, API keys, and other
settings that persist across projects.`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Supported keys:
  wallet       Ethereum wallet address for x402 payments
  ai.api_key   API key for AI-powered features`,
	Example: `  probe config set wallet 0x1234...abcd
  probe config set ai.api_key sk-...`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a configuration value.

Supported keys:
  wallet       Ethereum wallet address for x402 payments
  ai.api_key   API key for AI-powered features`,
	Example: `  probe config get wallet
  probe config get ai.api_key`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	configDir, err := cloud.ConfigDir()
	if err != nil {
		return fmt.Errorf("locating config dir: %w", err)
	}

	switch key {
	case "wallet":
		// Save wallet address using the cloud package.
		if err := cloud.SaveWallet(configDir, value); err != nil {
			return fmt.Errorf("saving wallet: %w", err)
		}
		fmt.Printf("Wallet address saved: %s\n", value)
		return nil

	case "ai.api_key":
		cfg, err := loadUserConfig(configDir)
		if err != nil {
			cfg = &userConfig{}
		}
		cfg.AIAPIKey = value
		if err := saveUserConfig(configDir, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Println("AI API key saved.")
		return nil

	default:
		return fmt.Errorf("unknown config key: %s (supported: wallet, ai.api_key)", key)
	}
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	configDir, err := cloud.ConfigDir()
	if err != nil {
		return fmt.Errorf("locating config dir: %w", err)
	}

	switch key {
	case "wallet":
		path := filepath.Join(configDir, "wallet.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("wallet not configured: run 'probe config set wallet <ADDRESS>'")
		}
		var wf struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(data, &wf); err != nil {
			return fmt.Errorf("invalid wallet.json: %w", err)
		}
		fmt.Println(wf.Address)
		return nil

	case "ai.api_key":
		cfg, err := loadUserConfig(configDir)
		if err != nil {
			return fmt.Errorf("config not found: run 'probe config set ai.api_key <KEY>'")
		}
		if cfg.AIAPIKey == "" {
			return fmt.Errorf("ai.api_key not set")
		}
		// Mask the key for security: show first 8 and last 4 chars.
		key := cfg.AIAPIKey
		if len(key) > 12 {
			fmt.Printf("%s...%s\n", key[:8], key[len(key)-4:])
		} else {
			fmt.Println(key)
		}
		return nil

	default:
		return fmt.Errorf("unknown config key: %s (supported: wallet, ai.api_key)", key)
	}
}

func loadUserConfig(configDir string) (*userConfig, error) {
	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg userConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveUserConfig(configDir string, cfg *userConfig) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, "config.json"), data, 0600)
}
