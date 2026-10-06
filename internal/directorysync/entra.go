package directorysync

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
)

const (
	maxGraphUsers = 5000
	maxGraphPages = 64
	maxGraphBody  = 4 << 20
)

var ErrGraphBoundary = errors.New("Microsoft Graph response crossed the approved endpoint boundary")

type EntraConfig struct {
	TenantID      string
	ClientID      string
	ClientSecret  string
	RemoteGroupID string
	EmailDomains  []string
}

type Recipient struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type graphUser struct {
	ID                string `json:"id"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	GivenName         string `json:"givenName"`
	Surname           string `json:"surname"`
	JobTitle          string `json:"jobTitle"`
}

type graphPage struct {
	Value    []graphUser `json:"value"`
	NextLink string      `json:"@odata.nextLink"`
}

func httpClient() *http.Client {
	transport := &http.Transport{
		Proxy:           nil,
		DialContext:     dialer.Dialer().DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func acquireToken(ctx context.Context, client *http.Client, cfg EntraConfig) (string, error) {
	tokenURL := "https://login.microsoftonline.com/" + url.PathEscape(strings.TrimSpace(cfg.TenantID)) + "/oauth2/v2.0/token"
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Microsoft identity token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Microsoft identity token request returned status %d", resp.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil {
		return "", fmt.Errorf("decode Microsoft identity token: %w", err)
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return "", errors.New("Microsoft identity token response was incomplete")
	}
	return token.AccessToken, nil
}

func firstGraphURL(cfg EntraConfig) string {
	group := url.PathEscape(strings.TrimSpace(cfg.RemoteGroupID))
	values := url.Values{}
	values.Set("$select", "id,mail,userPrincipalName,givenName,surname,jobTitle")
	values.Set("$top", "999")
	return "https://graph.microsoft.com/v1.0/groups/" + group + "/transitiveMembers/microsoft.graph.user?" + values.Encode()
}

func validateGraphURL(raw, remoteGroupID string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "graph.microsoft.com") || u.User != nil || u.Fragment != "" {
		return ErrGraphBoundary
	}
	expectedPath := "/v1.0/groups/" + url.PathEscape(strings.TrimSpace(remoteGroupID)) + "/transitiveMembers/microsoft.graph.user"
	if u.EscapedPath() != expectedPath {
		return ErrGraphBoundary
	}
	return nil
}

func allowedDomain(email string, domains map[string]struct{}) bool {
	if len(domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	_, ok := domains[strings.ToLower(email[at+1:])]
	return ok
}

func normalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return email
	}
	return email[:at+1] + strings.ToLower(email[at+1:])
}

func PreviewEntraGroup(ctx context.Context, cfg EntraConfig) ([]Recipient, error) {
	if strings.TrimSpace(cfg.TenantID) == "" || strings.TrimSpace(cfg.ClientID) == "" || cfg.ClientSecret == "" || strings.TrimSpace(cfg.RemoteGroupID) == "" {
		return nil, errors.New("Microsoft Entra connector configuration is incomplete")
	}
	client := httpClient()
	token, err := acquireToken(ctx, client, cfg)
	if err != nil {
		return nil, err
	}
	domains := make(map[string]struct{}, len(cfg.EmailDomains))
	for _, domain := range cfg.EmailDomains {
		domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
		if domain != "" {
			domains[domain] = struct{}{}
		}
	}
	next := firstGraphURL(cfg)
	byEmail := make(map[string]Recipient)
	for page := 0; next != ""; page++ {
		if page >= maxGraphPages {
			return nil, errors.New("Microsoft Graph pagination exceeded safety limit")
		}
		if err := validateGraphURL(next, cfg.RemoteGroupID); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("ConsistencyLevel", "eventual")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request Microsoft Graph group members: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, fmt.Errorf("Microsoft Graph returned status %d", resp.StatusCode)
		}
		var pageData graphPage
		err = json.NewDecoder(io.LimitReader(resp.Body, maxGraphBody)).Decode(&pageData)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode Microsoft Graph group members: %w", err)
		}
		for _, user := range pageData.Value {
			email := user.Mail
			if strings.TrimSpace(email) == "" {
				email = user.UserPrincipalName
			}
			email = normalizeEmail(email)
			if !strings.Contains(email, "@") || !allowedDomain(email, domains) {
				continue
			}
			parsed, parseErr := mail.ParseAddress(email)
			if parseErr != nil || parsed.Name != "" || parsed.Address != email {
				continue
			}
			key := email
			if _, exists := byEmail[key]; exists {
				continue
			}
			if len(email) > 320 || len(user.GivenName) > 256 || len(user.Surname) > 256 || len(user.JobTitle) > 512 {
				continue
			}
			byEmail[key] = Recipient{
				Email: email, FirstName: strings.TrimSpace(user.GivenName),
				LastName: strings.TrimSpace(user.Surname), Position: strings.TrimSpace(user.JobTitle),
			}
			if len(byEmail) > maxGraphUsers {
				return nil, errors.New("Microsoft Graph group exceeds managed preview limit")
			}
		}
		next = strings.TrimSpace(pageData.NextLink)
	}
	out := make([]Recipient, 0, len(byEmail))
	for _, recipient := range byEmail {
		out = append(out, recipient)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Email < out[j].Email
	})
	return out, nil
}
