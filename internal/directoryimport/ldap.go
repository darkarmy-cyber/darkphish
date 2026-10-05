package directoryimport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

const (
	maxDirectoryMembers = 5000
	maxDirectoryDepth   = 20
	defaultTimeout      = 15 * time.Second
)

var (
	ErrInvalidConfig = errors.New("invalid LDAP import configuration")
	ErrTooManyEntries = errors.New("LDAP import exceeds safety limit")
)

type AttributeMapping struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Position  string `json:"position"`
}

type Config struct {
	URL              string           `json:"url"`
	BindDN           string           `json:"bind_dn"`
	BindPassword     string           `json:"bind_password"`
	GroupDN          string           `json:"group_dn"`
	ExclusionGroupDN string           `json:"exclusion_group_dn,omitempty"`
	EmailDomains     []string         `json:"email_domains,omitempty"`
	Attributes       AttributeMapping `json:"attributes,omitempty"`
}

type Recipient struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
}

type Preview struct {
	Recipients []Recipient `json:"recipients"`
	Matched    int         `json:"matched"`
	Excluded   int         `json:"excluded"`
	Skipped    int         `json:"skipped"`
	Warnings   []string    `json:"warnings,omitempty"`
}

type directoryEntry struct {
	DN         string
	ObjectClass []string
	Members    []string
	Values     map[string]string
}

type entryReader interface {
	ReadEntry(ctx context.Context, dn string, attributes []string) (directoryEntry, error)
	Close() error
}

type ldapReader struct {
	conn *ldap.Conn
}

func PreviewLDAP(ctx context.Context, cfg Config) (Preview, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return Preview{}, err
	}
	reader, err := openLDAP(cfg)
	if err != nil {
		return Preview{}, err
	}
	defer reader.Close()
	return previewWithReader(ctx, reader, cfg)
}

func normalizeConfig(cfg Config) (Config, error) {
	rawURL := strings.TrimSpace(cfg.URL)
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "ldaps") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("%w: URL must be an ldaps:// endpoint without credentials or query parameters", ErrInvalidConfig)
	}
	if u.Path != "" && u.Path != "/" {
		return Config{}, fmt.Errorf("%w: LDAP URL path is not supported", ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.BindDN) == "" || strings.TrimSpace(cfg.BindPassword) == "" || strings.TrimSpace(cfg.GroupDN) == "" {
		return Config{}, fmt.Errorf("%w: bind DN, bind password and group DN are required", ErrInvalidConfig)
	}
	if _, err := ldap.ParseDN(cfg.BindDN); err != nil {
		return Config{}, fmt.Errorf("%w: invalid bind DN", ErrInvalidConfig)
	}
	if _, err := ldap.ParseDN(cfg.GroupDN); err != nil {
		return Config{}, fmt.Errorf("%w: invalid group DN", ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.ExclusionGroupDN) != "" {
		if _, err := ldap.ParseDN(cfg.ExclusionGroupDN); err != nil {
			return Config{}, fmt.Errorf("%w: invalid exclusion group DN", ErrInvalidConfig)
		}
	}
	cfg.URL = rawURL
	cfg.BindDN = strings.TrimSpace(cfg.BindDN)
	cfg.GroupDN = strings.TrimSpace(cfg.GroupDN)
	cfg.ExclusionGroupDN = strings.TrimSpace(cfg.ExclusionGroupDN)
	cfg.Attributes = normalizedMapping(cfg.Attributes)
	for i, domain := range cfg.EmailDomains {
		cfg.EmailDomains[i] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
		if cfg.EmailDomains[i] == "" || strings.ContainsAny(cfg.EmailDomains[i], " /\\") {
			return Config{}, fmt.Errorf("%w: invalid email domain filter", ErrInvalidConfig)
		}
	}
	return cfg, nil
}

func normalizedMapping(m AttributeMapping) AttributeMapping {
	if strings.TrimSpace(m.FirstName) == "" {
		m.FirstName = "givenName"
	}
	if strings.TrimSpace(m.LastName) == "" {
		m.LastName = "sn"
	}
	if strings.TrimSpace(m.Email) == "" {
		m.Email = "mail"
	}
	if strings.TrimSpace(m.Position) == "" {
		m.Position = "title"
	}
	return AttributeMapping{
		FirstName: strings.TrimSpace(m.FirstName),
		LastName:  strings.TrimSpace(m.LastName),
		Email:     strings.TrimSpace(m.Email),
		Position:  strings.TrimSpace(m.Position),
	}
}

func openLDAP(cfg Config) (entryReader, error) {
	u, _ := url.Parse(cfg.URL)
	serverName := u.Hostname()
	conn, err := ldap.DialURL(cfg.URL,
		ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}),
		ldap.DialWithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: serverName,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to LDAP directory: %w", err)
	}
	conn.SetTimeout(defaultTimeout)
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("bind to LDAP directory: %w", err)
	}
	return &ldapReader{conn: conn}, nil
}

func (r *ldapReader) Close() error {
	r.conn.Close()
	return nil
}

func (r *ldapReader) ReadEntry(ctx context.Context, dn string, attributes []string) (directoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return directoryEntry{}, err
	}
	attrs := append([]string{"objectClass", "member"}, attributes...)
	result, err := r.conn.Search(ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		int(defaultTimeout/time.Second),
		false,
		"(objectClass=*)",
		attrs,
		nil,
	))
	if err != nil {
		return directoryEntry{}, err
	}
	if len(result.Entries) != 1 {
		return directoryEntry{}, fmt.Errorf("directory entry not found: %s", dn)
	}
	entry := result.Entries[0]
	values := make(map[string]string, len(attributes))
	for _, attr := range attributes {
		values[attr] = strings.TrimSpace(entry.GetAttributeValue(attr))
	}
	return directoryEntry{
		DN:          entry.DN,
		ObjectClass: entry.GetAttributeValues("objectClass"),
		Members:     entry.GetAttributeValues("member"),
		Values:      values,
	}, nil
}

func previewWithReader(ctx context.Context, reader entryReader, cfg Config) (Preview, error) {
	attributes := uniqueStrings([]string{
		cfg.Attributes.FirstName,
		cfg.Attributes.LastName,
		cfg.Attributes.Email,
		cfg.Attributes.Position,
	})
	excluded := map[string]struct{}{}
	if cfg.ExclusionGroupDN != "" {
		var err error
		excluded, err = collectLeafDNs(ctx, reader, cfg.ExclusionGroupDN, attributes)
		if err != nil {
			return Preview{}, fmt.Errorf("resolve exclusion group: %w", err)
		}
	}
	leaves, err := collectLeafDNs(ctx, reader, cfg.GroupDN, attributes)
	if err != nil {
		return Preview{}, fmt.Errorf("resolve import group: %w", err)
	}

	result := Preview{}
	byEmail := make(map[string]Recipient)
	dns := make([]string, 0, len(leaves))
	for dn := range leaves {
		dns = append(dns, dn)
	}
	sort.Strings(dns)
	for _, dn := range dns {
		if _, skip := excluded[dn]; skip {
			result.Excluded++
			continue
		}
		entry, err := reader.ReadEntry(ctx, dn, attributes)
		if err != nil {
			return Preview{}, fmt.Errorf("read directory member %s: %w", dn, err)
		}
		email := strings.ToLower(strings.TrimSpace(entry.Values[cfg.Attributes.Email]))
		if !allowedEmail(email, cfg.EmailDomains) {
			result.Skipped++
			continue
		}
		parsed, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(parsed.Address, email) {
			result.Skipped++
			continue
		}
		recipient := Recipient{
			Email:     email,
			FirstName: strings.TrimSpace(entry.Values[cfg.Attributes.FirstName]),
			LastName:  strings.TrimSpace(entry.Values[cfg.Attributes.LastName]),
			Position:  strings.TrimSpace(entry.Values[cfg.Attributes.Position]),
		}
		if _, exists := byEmail[email]; exists {
			result.Skipped++
			continue
		}
		byEmail[email] = recipient
	}
	for _, recipient := range byEmail {
		result.Recipients = append(result.Recipients, recipient)
	}
	sort.Slice(result.Recipients, func(i, j int) bool {
		return result.Recipients[i].Email < result.Recipients[j].Email
	})
	result.Matched = len(result.Recipients)
	if result.Skipped > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d directory entries were skipped because they were duplicates, invalid, missing email, or outside the allowed domains", result.Skipped))
	}
	return result, nil
}

func collectLeafDNs(ctx context.Context, reader entryReader, rootDN string, attributes []string) (map[string]struct{}, error) {
	leaves := make(map[string]struct{})
	visited := make(map[string]struct{})
	var walk func(string, int) error
	walk = func(dn string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > maxDirectoryDepth {
			return fmt.Errorf("%w: nested group depth exceeds %d", ErrTooManyEntries, maxDirectoryDepth)
		}
		key := strings.ToLower(strings.TrimSpace(dn))
		if _, ok := visited[key]; ok {
			return nil
		}
		visited[key] = struct{}{}
		if len(visited) > maxDirectoryMembers {
			return ErrTooManyEntries
		}
		entry, err := reader.ReadEntry(ctx, dn, attributes)
		if err != nil {
			return err
		}
		if len(entry.Members) == 0 && !isGroup(entry.ObjectClass) {
			leaves[key] = struct{}{}
			return nil
		}
		for _, memberDN := range entry.Members {
			if err := walk(memberDN, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(rootDN, 0); err != nil {
		return nil, err
	}
	return leaves, nil
}

func isGroup(classes []string) bool {
	for _, class := range classes {
		switch strings.ToLower(strings.TrimSpace(class)) {
		case "group", "groupofnames", "groupofuniquenames", "posixgroup":
			return true
		}
	}
	return false
}

func allowedEmail(email string, domains []string) bool {
	if email == "" {
		return false
	}
	if len(domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, allowed := range domains {
		if domain == allowed {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
