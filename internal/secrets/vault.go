package secrets

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type VaultTransitOptions struct {
	Address   string
	Token     string
	Namespace string
	Mount     string
	KeyName   string
	CACert    string
	Client    *http.Client
}

type VaultTransitProvider struct {
	address   *url.URL
	token     string
	namespace string
	mount     string
	keyName   string
	client    *http.Client
}

func NewVaultTransitProvider(options VaultTransitOptions) (*VaultTransitProvider, error) {
	address, err := url.Parse(strings.TrimRight(options.Address, "/"))
	if err != nil || address.Host == "" || address.User != nil || (address.Scheme != "https" && address.Scheme != "http") {
		return nil, errors.New("invalid Vault address")
	}
	if options.Token == "" || !keyIDPattern.MatchString(options.KeyName) {
		return nil, errors.New("Vault token and valid Transit key name are required")
	}
	if options.Mount == "" {
		options.Mount = "transit"
	}
	if !keyIDPattern.MatchString(options.Mount) {
		return nil, errors.New("invalid Vault Transit mount")
	}
	client := options.Client
	if client == nil {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		if options.CACert != "" {
			pem, err := os.ReadFile(options.CACert)
			if err != nil {
				return nil, fmt.Errorf("read Vault CA certificate: %w", err)
			}
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			if !roots.AppendCertsFromPEM(pem) {
				return nil, errors.New("Vault CA certificate contains no usable certificates")
			}
			transport.TLSClientConfig.RootCAs = roots
		}
		client = &http.Client{Transport: transport, Timeout: 10 * time.Second}
	}
	return &VaultTransitProvider{address: address, token: options.Token, namespace: options.Namespace, mount: options.Mount, keyName: options.KeyName, client: client}, nil
}

func (p *VaultTransitProvider) Name() string { return "vault" }

func (p *VaultTransitProvider) ActiveKey(context.Context) (KeyReference, error) {
	return KeyReference{Provider: p.Name(), KeyID: p.keyName}, nil
}

func (p *VaultTransitProvider) transit(ctx context.Context, operation string, reference KeyReference, requestValue map[string]string) (map[string]interface{}, error) {
	if reference.Provider != p.Name() || !keyIDPattern.MatchString(reference.KeyID) {
		return nil, ErrWrongProvider
	}
	body, err := json.Marshal(requestValue)
	if err != nil {
		return nil, err
	}
	endpoint := *p.address
	endpoint.Path = path.Join(endpoint.Path, "v1", p.mount, operation, reference.KeyID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Vault-Token", p.token)
	if p.namespace != "" {
		request.Header.Set("X-Vault-Namespace", p.namespace)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, ErrProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w (Vault Transit returned status %d)", ErrProviderUnavailable, response.StatusCode)
	}
	decoded := struct {
		Data map[string]interface{} `json:"data"`
	}{}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decoded); err != nil || decoded.Data == nil {
		return nil, ErrProviderUnavailable
	}
	return decoded.Data, nil
}

func (p *VaultTransitProvider) EncryptDataKey(ctx context.Context, reference KeyReference, plaintext []byte) ([]byte, error) {
	data, err := p.transit(ctx, "encrypt", reference, map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)})
	if err != nil {
		return nil, err
	}
	ciphertext, ok := data["ciphertext"].(string)
	if !ok || !strings.HasPrefix(ciphertext, "vault:v") {
		return nil, ErrProviderUnavailable
	}
	return []byte(ciphertext), nil
}

func (p *VaultTransitProvider) DecryptDataKey(ctx context.Context, reference KeyReference, wrapped []byte) ([]byte, error) {
	data, err := p.transit(ctx, "decrypt", reference, map[string]string{"ciphertext": string(wrapped)})
	if err != nil {
		return nil, err
	}
	plaintext, ok := data["plaintext"].(string)
	if !ok {
		return nil, ErrProviderUnavailable
	}
	decoded, err := base64.StdEncoding.DecodeString(plaintext)
	if err != nil || len(decoded) != 32 {
		return nil, ErrProviderUnavailable
	}
	return decoded, nil
}
