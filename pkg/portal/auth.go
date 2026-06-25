package portal

import (
	"fmt"
	"os"
)

type AuthConfig struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	OTPSecret string `json:"otp_secret"`
}

const (
	EnvUsername  = "PORTAL_USERNAME"
	EnvPassword  = "PORTAL_PASSWORD"
	EnvOTPSecret = "PORTAL_OTP_SECRET"
)

func getEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is not set", key)
	}
	return value, nil
}

func LoadAuthConfig() (*AuthConfig, error) {
	var config AuthConfig

	username, err := getEnv(EnvUsername)
	if err != nil {
		return nil, err
	}
	config.Username = username

	password, err := getEnv(EnvPassword)
	if err != nil {
		return nil, err
	}
	config.Password = password

	otpSecret, err := getEnv(EnvOTPSecret)
	if err != nil {
		return nil, err
	}
	config.OTPSecret = otpSecret

	return &config, nil
}
