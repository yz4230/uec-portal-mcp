package uecauth

import (
	"fmt"
	"os"
)

const (
	EnvUsername  = "PORTAL_USERNAME"
	EnvPassword  = "PORTAL_PASSWORD"
	EnvOTPSecret = "PORTAL_OTP_SECRET"
)

// LoadCredentialsFromEnv is a convenience for command-line applications.
// Callers may instead construct Credentials without using environment variables.
func LoadCredentialsFromEnv() (*Credentials, error) {
	username, err := requiredEnv(EnvUsername)
	if err != nil {
		return nil, err
	}
	password, err := requiredEnv(EnvPassword)
	if err != nil {
		return nil, err
	}
	otpSecret, err := requiredEnv(EnvOTPSecret)
	if err != nil {
		return nil, err
	}
	return &Credentials{Username: username, Password: password, OTPSecret: otpSecret}, nil
}

func requiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is not set", key)
	}
	return value, nil
}
