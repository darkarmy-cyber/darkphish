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
	"strconv"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/dialer"
	"github.com/go-ldap/ldap/v3"
)

const (
	maxDirectoryMembers = 5000
	maxDirectoryDepth   = 20
	maxEmailDomains     = 100
	maxDomainBytes      = 8192
	maxNameBytes        = 256
	maxEmailBytes       = 320
	maxPositionBytes    = 512
	maxDNBytes          = 4096
	maxObjectClasses    = 64
	maxObjectClassBytes = 8192
	maxMembershipBytes  = 2 << 20
	maxPreviewBytes     = 2 << 20
	defaultTimeout      = 5 * time.Second
)

var (
	ErrInvalidConfig   = errors.New("invalid LDAP import configuration")
	ErrTooManyEntries  = errors.New("LDAP import exceeds safety limit")
	ErrUnsupportedLDAP = errors.New("unsupported LDAP group schema")
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
	domainSet        map[string]struct{}
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
	DN          string
	ObjectClass []string
	Members     []string
	Values      map[string]string
	Oversized   bool
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
	cancelWatchDone := make(chan struct{})
	defer close(cancelWatchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = reader.Close()
		case <-cancelWatchDone:
		}
	}()
	return previewWithReader(ctx, reader, cfg)
}

func NormalizeAuditIdentity(cfg Config) (string, string, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return "", "", err
	}
	u, _ := url.Parse(cfg.URL)
	port := u.Port()
	if port == "" {
		port = "636"
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port), cfg.GroupDN, nil
}

func normalizeConfig(cfg Config) (Config, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || !strings.EqualFold(u.Scheme, "ldaps") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("%w: URL must be an ldaps:// endpoint without credentials or query parameters", ErrInvalidConfig)
	}
	if u.Path != "" && u.Path != "/" {
		return Config{}, fmt.Errorf("%w: LDAP URL path is not supported", ErrInvalidConfig)
	}
	u.Scheme = "ldaps"
	u.Host = strings.ToLower(u.Host)
	cfg.URL = u.String()
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
	cfg.BindDN = strings.TrimSpace(cfg.BindDN)
	cfg.GroupDN = strings.TrimSpace(cfg.GroupDN)
	cfg.ExclusionGroupDN = strings.TrimSpace(cfg.ExclusionGroupDN)
	cfg.Attributes = normalizedMapping(cfg.Attributes)
	for _, attribute := range []string{cfg.Attributes.FirstName, cfg.Attributes.LastName, cfg.Attributes.Email, cfg.Attributes.Position} {
		if !validAttributeName(attribute) {
			return Config{}, fmt.Errorf("%w: invalid LDAP attribute name", ErrInvalidConfig)
		}
	}
	if len(cfg.EmailDomains) > maxEmailDomains {
		return Config{}, fmt.Errorf("%w: too many email domain filters", ErrInvalidConfig)
	}
	cfg.domainSet = make(map[string]struct{}, len(cfg.EmailDomains))
	normalizedDomains := make([]string, 0, len(cfg.EmailDomains))
	totalDomainBytes := 0
	for _, domain := range cfg.EmailDomains {
		domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
		if domain == "" || strings.ContainsAny(domain, " /\\") {
			return Config{}, fmt.Errorf("%w: invalid email domain filter", ErrInvalidConfig)
		}
		totalDomainBytes += len(domain)
		if totalDomainBytes > maxDomainBytes {
			return Config{}, fmt.Errorf("%w: email domain filters exceed size limit", ErrInvalidConfig)
		}
		if _, exists := cfg.domainSet[domain]; exists {
			continue
		}
		cfg.domainSet[domain] = struct{}{}
		normalizedDomains = append(normalizedDomains, domain)
	}
	cfg.EmailDomains = normalizedDomains
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
	restrictedDialer := dialer.Dialer()
	restrictedDialer.Timeout = defaultTimeout
	conn, err := ldap.DialURL(cfg.URL,
		ldap.DialWithDialer(restrictedDialer),
		ldap.DialWithTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: u.Hostname(),
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

func (r *ldapReader) searchBase(dn string, attributes []string) (*ldap.Entry, error) {
	result, err := r.conn.Search(ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		int(defaultTimeout/time.Second),
		false,
		"(objectClass=*)",
		attributes,
		nil,
	))
	if err != nil {
		return nil, err
	}
	if len(result.Entries) != 1 {
		return nil, fmt.Errorf("directory entry not found")
	}
	return result.Entries[0], nil
}

func (r *ldapReader) ReadEntry(ctx context.Context, dn string, attributes []string) (directoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return directoryEntry{}, err
	}
	attrs := append([]string{"objectClass", "member", "uniqueMember"}, attributes...)
	entry, err := r.searchBase(dn, attrs)
	if err != nil {
		return directoryEntry{}, err
	}
	classes, _, err := boundedValues(entry.GetEqualFoldAttributeValues("objectClass"), maxObjectClasses, maxObjectClassBytes, 256)
	if err != nil {
		return directoryEntry{}, err
	}
	if hasClass(classes, "posixGroup") {
		return directoryEntry{}, ErrUnsupportedLDAP
	}
	members, memberBytes, err := boundedValues(entry.GetEqualFoldAttributeValues("member"), maxDirectoryMembers, maxMembershipBytes, maxDNBytes)
	if err != nil {
		return directoryEntry{}, err
	}
	for _, uniqueMember := range entry.GetEqualFoldAttributeValues("uniqueMember") {
		memberDN, parseErr := uniqueMemberDN(uniqueMember)
		if parseErr != nil {
			return directoryEntry{}, parseErr
		}
		if err := appendBoundedValue(&members, &memberBytes, memberDN, maxDirectoryMembers, maxMembershipBytes, maxDNBytes); err != nil {
			return directoryEntry{}, err
		}
	}
	rangeValues, rangeStart, rangeDone := rangedMemberValues(entry)
	for _, memberDN := range rangeValues {
		if err := appendBoundedValue(&members, &memberBytes, memberDN, maxDirectoryMembers, maxMembershipBytes, maxDNBytes); err != nil {
			return directoryEntry{}, err
		}
	}
	for !rangeDone {
		if err := ctx.Err(); err != nil {
			return directoryEntry{}, err
		}
		rangeAttr := fmt.Sprintf("member;range=%d-*", rangeStart)
		rangeEntry, err := r.searchBase(dn, []string{rangeAttr})
		if err != nil {
			return directoryEntry{}, err
		}
		values, next, done := rangedMemberValues(rangeEntry)
		for _, memberDN := range values {
			if err := appendBoundedValue(&members, &memberBytes, memberDN, maxDirectoryMembers, maxMembershipBytes, maxDNBytes); err != nil {
				return directoryEntry{}, err
			}
		}
		rangeStart, rangeDone = next, done
	}
	values := make(map[string]string, len(attributes))
	oversized := false
	for _, attr := range attributes {
		value := strings.TrimSpace(entry.GetEqualFoldAttributeValue(attr))
		if len(value) > maxPositionBytes {
			oversized = true
			continue
		}
		values[strings.ToLower(attr)] = value
	}
	for _, memberDN := range members {
		if len(memberDN) == 0 || len(memberDN) > maxDNBytes {
			return directoryEntry{}, ErrTooManyEntries
		}
	}
	return directoryEntry{
		DN:          entry.DN,
		ObjectClass: classes,
		Members:     members,
		Values:      values,
		Oversized:   oversized,
	}, nil
}

func boundedValues(values []string, maxCount, maxBytes, maxValueBytes int) ([]string, int, error) {
	if len(values) > maxCount {
		return nil, 0, ErrTooManyEntries
	}
	out := make([]string, 0, len(values))
	total := 0
	for _, value := range values {
		if len(value) == 0 || len(value) > maxValueBytes {
			return nil, 0, ErrTooManyEntries
		}
		total += len(value)
		if total > maxBytes {
			return nil, 0, ErrTooManyEntries
		}
		out = append(out, value)
	}
	return out, total, nil
}

func appendBoundedValue(values *[]string, totalBytes *int, value string, maxCount, maxBytes, maxValueBytes int) error {
	if len(value) == 0 || len(value) > maxValueBytes || len(*values) >= maxCount {
		return ErrTooManyEntries
	}
	if *totalBytes+len(value) > maxBytes {
		return ErrTooManyEntries
	}
	*values = append(*values, value)
	*totalBytes += len(value)
	return nil
}

func rangedMemberValues(entry *ldap.Entry) ([]string, int, bool) {
	for _, attr := range entry.Attributes {
		name := strings.ToLower(attr.Name)
		if !strings.HasPrefix(name, "member;range=") {
			continue
		}
		spec := strings.TrimPrefix(name, "member;range=")
		parts := strings.SplitN(spec, "-", 2)
		if len(parts) != 2 {
			return attr.Values, 0, true
		}
		if parts[1] == "*" {
			return attr.Values, 0, true
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return attr.Values, 0, true
		}
		return attr.Values, end + 1, false
	}
	return nil, 0, true
}

func previewWithReader(ctx context.Context, reader entryReader, cfg Config) (Preview, error) {
	attributes := uniqueStrings([]string{
		cfg.Attributes.FirstName,
		cfg.Attributes.LastName,
		cfg.Attributes.Email,
		cfg.Attributes.Position,
	})
	excluded := map[string]directoryEntry{}
	if cfg.ExclusionGroupDN != "" {
		var err error
		excluded, err = collectLeafEntries(ctx, reader, cfg.ExclusionGroupDN, attributes)
		if err != nil {
			return Preview{}, fmt.Errorf("resolve exclusion group: %w", err)
		}
	}
	leaves, err := collectLeafEntries(ctx, reader, cfg.GroupDN, attributes)
	if err != nil {
		return Preview{}, fmt.Errorf("resolve import group: %w", err)
	}

	result := Preview{}
	byEmail := make(map[string]Recipient)
	keys := make([]string, 0, len(leaves))
	for key := range leaves {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	totalBytes := 0
	for _, key := range keys {
		if _, skip := excluded[key]; skip {
			result.Excluded++
			continue
		}
		entry := leaves[key]
		if entry.Oversized {
			result.Skipped++
			continue
		}
		email := normalizeEmailDomain(strings.TrimSpace(entry.Values[strings.ToLower(cfg.Attributes.Email)]))
		first := strings.TrimSpace(entry.Values[strings.ToLower(cfg.Attributes.FirstName)])
		last := strings.TrimSpace(entry.Values[strings.ToLower(cfg.Attributes.LastName)])
		position := strings.TrimSpace(entry.Values[strings.ToLower(cfg.Attributes.Position)])
		if len(email) > maxEmailBytes || len(first) > maxNameBytes || len(last) > maxNameBytes || len(position) > maxPositionBytes {
			result.Skipped++
			continue
		}
		if !allowedEmail(email, cfg.domainSet) {
			result.Skipped++
			continue
		}
		if !validBareMailbox(email) {
			result.Skipped++
			continue
		}
		if _, exists := byEmail[email]; exists {
			result.Skipped++
			continue
		}
		recipient := Recipient{Email: email, FirstName: first, LastName: last, Position: position}
		totalBytes += len(email) + len(first) + len(last) + len(position)
		if totalBytes > maxPreviewBytes {
			return Preview{}, ErrTooManyEntries
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
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d directory entries were skipped because they were duplicates, invalid, oversized, missing email, or outside the allowed domains", result.Skipped))
	}
	return result, nil
}

func collectLeafEntries(ctx context.Context, reader entryReader, rootDN string, attributes []string) (map[string]directoryEntry, error) {
	leaves := make(map[string]directoryEntry)
	visited := make(map[string]struct{})
	var walk func(string, int) error
	walk = func(dn string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := canonicalDNKey(dn)
		if err != nil {
			return err
		}
		if _, ok := visited[key]; ok {
			return nil
		}
		if depth > maxDirectoryDepth {
			return fmt.Errorf("%w: nested group depth exceeds %d", ErrTooManyEntries, maxDirectoryDepth)
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
			leaves[key] = entry
			return nil
		}
		if isGroup(entry.ObjectClass) && len(entry.Members) == 0 {
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

func canonicalDNKey(value string) (string, error) {
	dn, err := ldap.ParseDN(value)
	if err != nil {
		return "", err
	}
	rdns := make([]string, 0, len(dn.RDNs))
	for _, rdn := range dn.RDNs {
		attrs := make([]string, 0, len(rdn.Attributes))
		for _, attr := range rdn.Attributes {
			attrs = append(attrs, strings.ToLower(attr.Type)+"\x00"+attr.Value)
		}
		sort.Strings(attrs)
		rdns = append(rdns, strings.Join(attrs, "+"))
	}
	return strings.Join(rdns, ","), nil
}

func hasClass(classes []string, want string) bool {
	for _, class := range classes {
		if strings.EqualFold(strings.TrimSpace(class), want) {
			return true
		}
	}
	return false
}

func isGroup(classes []string) bool {
	return hasClass(classes, "group") || hasClass(classes, "groupOfNames") || hasClass(classes, "groupOfUniqueNames")
}

func allowedEmail(email string, domains map[string]struct{}) bool {
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
	_, ok := domains[strings.ToLower(email[at+1:])]
	return ok
}

func validAttributeName(value string) bool {
	parts := strings.Split(value, ";")
	if len(parts) == 0 || !validAttributeType(parts[0]) {
		return false
	}
	for _, option := range parts[1:] {
		if option == "" {
			return false
		}
		for _, r := range option {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func validAttributeType(value string) bool {
	if value == "" {
		return false
	}
	if value[0] >= '0' && value[0] <= '9' {
		parts := strings.Split(value, ".")
		if len(parts) < 2 {
			return false
		}
		for _, part := range parts {
			if part == "" || (len(part) > 1 && part[0] == '0') {
				return false
			}
			for _, r := range part {
				if r < '0' || r > '9' {
					return false
				}
			}
		}
		return true
	}
	for i, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9') || (i > 0 && r == '-') {
			continue
		}
		return false
	}
	return true
}

func validBareMailbox(email string) bool {
	if email == "" {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" {
		return false
	}
	trimmed := strings.TrimSpace(email)
	if strings.HasPrefix(trimmed, "<") || strings.HasSuffix(trimmed, ">") {
		return false
	}
	return true
}

func normalizeEmailDomain(email string) string {
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return email
	}
	return email[:at+1] + strings.ToLower(email[at+1:])
}

func uniqueMemberDN(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidConfig
	}
	for i := len(value) - 1; i >= 1; i-- {
		if value[i] != '#' || isEscaped(value, i) || i+3 >= len(value) || value[i+1] != '\'' {
			continue
		}
		suffix := value[i+1:]
		if len(suffix) < 4 || suffix[len(suffix)-2] != '\'' || (suffix[len(suffix)-1] != 'B' && suffix[len(suffix)-1] != 'b') {
			continue
		}
		bits := suffix[1 : len(suffix)-2]
		if bits == "" {
			continue
		}
		validBits := true
		for _, bit := range bits {
			if bit != '0' && bit != '1' {
				validBits = false
				break
			}
		}
		if !validBits {
			continue
		}
		value = strings.TrimSpace(value[:i])
		break
	}
	if _, err := ldap.ParseDN(value); err != nil {
		return "", fmt.Errorf("%w: invalid uniqueMember DN", ErrInvalidConfig)
	}
	return value, nil
}

func isEscaped(value string, index int) bool {
	backslashes := 0
	for i := index - 1; i >= 0 && value[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
