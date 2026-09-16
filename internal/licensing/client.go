package licensing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxLicenseResponseBytes int64 = 1 << 20

var ErrLicenseService = errors.New("license service request failed")

type Client struct {
	baseURL        *url.URL
	httpClient     *http.Client
	productVersion string
	allowHTTP      bool
}

type ActivationResponse struct {
	Lease        json.RawMessage `json:"lease"`
	RefreshToken string          `json:"refresh_token"`
}

type activationRequest struct {
	LicenseKey     string `json:"license_key"`
	InstallationID string `json:"installation_id"`
	ProductVersion string `json:"product_version"`
}

type refreshRequest struct {
	InstallationID string `json:"installation_id"`
	ProductVersion string `json:"product_version"`
	RefreshToken   string `json:"refresh_token"`
}

func NewClient(rawBaseURL, productVersion string, httpClient *http.Client) (*Client, error) {
	return newClient(rawBaseURL, productVersion, httpClient, false)
}

func newClient(rawBaseURL, productVersion string, httpClient *http.Client, allowHTTP bool) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid licensing service URL")
	}
	if parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http") {
		return nil, errors.New("licensing service URL must use HTTPS")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	// Never forward activation credentials to a redirect destination. Copy the
	// supplied client so its caller's redirect policy is not changed.
	privateClient := *httpClient
	privateClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if privateClient.Timeout <= 0 || privateClient.Timeout > 15*time.Second {
		privateClient.Timeout = 15 * time.Second
	}
	if strings.TrimSpace(productVersion) == "" {
		return nil, errors.New("product version is required")
	}
	return &Client{baseURL: parsed, httpClient: &privateClient, productVersion: productVersion, allowHTTP: allowHTTP}, nil
}

func (c *Client) Activate(ctx context.Context, licenseKey, installationID string) (ActivationResponse, error) {
	if strings.TrimSpace(licenseKey) == "" || strings.TrimSpace(installationID) == "" {
		return ActivationResponse{}, errors.New("license key and installation id are required")
	}
	response, err := c.post(ctx, "/activate", activationRequest{
		LicenseKey: licenseKey, InstallationID: installationID, ProductVersion: c.productVersion,
	})
	if err != nil {
		return ActivationResponse{}, err
	}
	if strings.TrimSpace(response.RefreshToken) == "" {
		return ActivationResponse{}, fmt.Errorf("%w: missing refresh token", ErrLicenseService)
	}
	return response, nil
}

func (c *Client) Refresh(ctx context.Context, refreshToken, installationID string) (ActivationResponse, error) {
	if strings.TrimSpace(refreshToken) == "" || strings.TrimSpace(installationID) == "" {
		return ActivationResponse{}, errors.New("refresh token and installation id are required")
	}
	return c.post(ctx, "/refresh", refreshRequest{
		RefreshToken: refreshToken, InstallationID: installationID, ProductVersion: c.productVersion,
	})
}

func (c *Client) post(ctx context.Context, endpoint string, payload any) (ActivationResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return ActivationResponse{}, err
	}
	endpointURL := *c.baseURL
	endpointURL.Path = strings.TrimRight(c.baseURL.Path, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return ActivationResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Darkphish/"+c.productVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ActivationResponse{}, fmt.Errorf("%w: transport failure", ErrLicenseService)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxLicenseResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return ActivationResponse{}, fmt.Errorf("%w: read response", ErrLicenseService)
	}
	if int64(len(raw)) > maxLicenseResponseBytes {
		return ActivationResponse{}, fmt.Errorf("%w: response too large", ErrLicenseService)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Deliberately do not reflect a potentially attacker-controlled service
		// body that could contain license/account details or HTML.
		return ActivationResponse{}, fmt.Errorf("%w: HTTP %d", ErrLicenseService, resp.StatusCode)
	}
	var result ActivationResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return ActivationResponse{}, fmt.Errorf("%w: invalid response", ErrLicenseService)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ActivationResponse{}, fmt.Errorf("%w: trailing response data", ErrLicenseService)
	}
	if len(result.Lease) == 0 || bytes.Equal(bytes.TrimSpace(result.Lease), []byte("null")) {
		return ActivationResponse{}, fmt.Errorf("%w: missing lease", ErrLicenseService)
	}
	return result, nil
}
