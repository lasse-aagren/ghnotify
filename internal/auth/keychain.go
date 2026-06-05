package auth

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const keychainService = "ghnotify"

type KeyType string

const (
	KeyAccessToken       KeyType = "access_token"
	KeyRefreshToken      KeyType = "refresh_token"
	KeyPAT               KeyType = "pat"
	KeyOAuthClientSecret KeyType = "oauth_client_secret"
	KeyUsername          KeyType = "username"
)

func accountKey(host string, key KeyType) string {
	return fmt.Sprintf("%s:%s", host, key)
}

func GetSecret(host string, key KeyType) (string, error) {
	return keyring.Get(keychainService, accountKey(host, key))
}

func SetSecret(host string, key KeyType, value string) error {
	return keyring.Set(keychainService, accountKey(host, key), value)
}

func DeleteSecret(host string, key KeyType) error {
	return keyring.Delete(keychainService, accountKey(host, key))
}

func HasSecret(host string, key KeyType) bool {
	val, err := GetSecret(host, key)
	return err == nil && val != ""
}
