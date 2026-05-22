package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/odoo/github-pr/backend/models"
)

// Default App Client ID (User should replace with their registered GitHub App/OAuth Client ID)
const DefaultClientID = "Ov23li2HarsMBh78y8nb"

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

type AuthHandler struct {
	clientID string
	client   *http.Client
}

func NewAuthHandler(clientID string) *AuthHandler {
	if clientID == "" {
		clientID = DefaultClientID
	}
	return &AuthHandler{
		clientID: clientID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// RequestDeviceCode initiates OAuth Device Authorization Flow
func (h *AuthHandler) RequestDeviceCode() (*DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", h.clientID)
	data.Set("scope", "repo read:org")

	req, err := http.NewRequest(
		"POST",
		"https://github.com/login/device/code",
		bytes.NewBufferString(data.Encode()),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"device code request failed: status=%d body=%s",
			resp.StatusCode,
			string(body),
		)
	}

	var result DeviceCodeResponse

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// PollForToken blocks and polls GitHub until the user authorizes the app or verification expires
func (h *AuthHandler) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			data := url.Values{}
			data.Set("client_id", h.clientID)
			data.Set("device_code", deviceCode)
			data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

			req, err := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", bytes.NewBufferString(data.Encode()))
			if err != nil {
				return "", err
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := h.client.Do(req)
			if err != nil {
				return "", err
			}

			var tokenResp TokenResponse
			err = json.NewDecoder(resp.Body).Decode(&tokenResp)
			resp.Body.Close()
			if err != nil {
				return "", err
			}

			if tokenResp.Error != "" {
				switch tokenResp.Error {
				case "authorization_pending":
					// User has not finished entering code yet, continue polling
					continue
				case "slow_down":
					// Backoff: increase polling interval
					ticker.Reset(time.Duration(interval+5) * time.Second)
					continue
				case "expired_token":
					return "", fmt.Errorf("authorization code expired, please request login again")
				case "access_denied":
					return "", fmt.Errorf("user denied authorization request")
				default:
					return "", fmt.Errorf("oauth error: %s", tokenResp.Error)
				}
			}

			if tokenResp.AccessToken != "" {
				return tokenResp.AccessToken, nil
			}
		}
	}
}

// FetchUserProfile obtains Github user profile metadata
func (h *AuthHandler) FetchUserProfile(accessToken string) (*models.User, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch profile, status: %d", resp.StatusCode)
	}

	var user models.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}
