package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const ClientID = "Ov23liP6o0infpaUR8Eg"

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

func RequestDeviceCode() (*DeviceCodeResponse, error) {
	url := "https://github.com/login/device/code"
	data := fmt.Sprintf("client_id=%s&scope=repo,delete_repo", ClientID)

	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub device code request failed: %s", string(body))
	}

	var deviceResp DeviceCodeResponse
	if err := json.NewDecoder(strings.NewReader(string(body))).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	if deviceResp.DeviceCode == "" || deviceResp.UserCode == "" || deviceResp.VerificationURI == "" {
		return nil, fmt.Errorf("invalid device code response from GitHub")
	}

	if deviceResp.Interval == 0 {
		deviceResp.Interval = 5
	}
	if deviceResp.ExpiresIn == 0 {
		deviceResp.ExpiresIn = 900
	}

	return &deviceResp, nil
}

func PollForAccessToken(deviceCode string, interval, expiresIn int) (string, error) {
	url := "https://github.com/login/oauth/access_token"
	data := fmt.Sprintf("client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code", ClientID, deviceCode)

	if interval < 1 {
		interval = 5
	}

	timeout := time.Now().Add(time.Duration(expiresIn) * time.Second)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(timeout) {
				return "", fmt.Errorf("authentication timeout")
			}

			req, err := http.NewRequest("POST", url, strings.NewReader(data))
			if err != nil {
				continue
			}

			req.Header.Set("Accept", "application/json")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			var tokenResp accessTokenResponse
			if err := json.Unmarshal(body, &tokenResp); err != nil {
				continue
			}

			switch tokenResp.Error {
			case "authorization_pending":
				fmt.Print(".")
				continue
			case "slow_down":
				ticker.Reset(time.Duration(interval+5) * time.Second)
				continue
			case "expired_token":
				return "", fmt.Errorf("device code expired, please try again")
			case "access_denied":
				return "", fmt.Errorf("access denied by user")
			case "":
				if tokenResp.AccessToken != "" {
					fmt.Println()
					return tokenResp.AccessToken, nil
				}
			default:
				return "", fmt.Errorf("oauth error: %s", tokenResp.Error)
			}
		}
	}
}

func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
