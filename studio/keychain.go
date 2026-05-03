package main

import "github.com/zalando/go-keyring"

const (
	keychainService = "flutterprobe.studio"
	keychainUser    = "anthropic-api-key"
)

func keychainGet() (string, error)    { return keyring.Get(keychainService, keychainUser) }
func keychainSet(key string) error    { return keyring.Set(keychainService, keychainUser, key) }
func keychainDelete() error           { return keyring.Delete(keychainService, keychainUser) }
