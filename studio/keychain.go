package main

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const (
	keychainService = "flutterprobe.studio"
	keychainUser    = "anthropic-api-key"
)

func keychainGet() (string, error) { return keyring.Get(keychainService, keychainUser) }
func keychainSet(key string) error { return keyring.Set(keychainService, keychainUser, key) }
func keychainDelete() error        { return keyring.Delete(keychainService, keychainUser) }

// keychainHas reports whether an item is saved. A missing item is not an error; anything else is.
func keychainHas(service, user string) (bool, error) {
	_, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
