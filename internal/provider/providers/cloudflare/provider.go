// Package cloudflare implements a Cloudflare DDNS provider.
package cloudflare

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"

	cf "github.com/cloudflare/cloudflare-go/v6"
	cfdns "github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/qdm12/ddns-updater/internal/models"
	"github.com/qdm12/ddns-updater/internal/provider/constants"
	"github.com/qdm12/ddns-updater/internal/provider/errors"
	"github.com/qdm12/ddns-updater/internal/provider/utils"
	"github.com/qdm12/ddns-updater/pkg/publicip/ipversion"
)

type Provider struct {
	domain         string
	owner          string
	ipVersion      ipversion.IPVersion
	ipv6Suffix     netip.Prefix
	key            string
	token          string
	email          string
	userServiceKey string
	zoneIdentifier string
	proxied        bool
	ttl            uint32
}

func New(data json.RawMessage, domain, owner string,
	ipVersion ipversion.IPVersion, ipv6Suffix netip.Prefix) (
	p *Provider, err error,
) {
	extraSettings := struct {
		Key            string `json:"key"`
		Token          string `json:"token"`
		Email          string `json:"email"`
		UserServiceKey string `json:"user_service_key"`
		ZoneIdentifier string `json:"zone_identifier"`
		Proxied        bool   `json:"proxied"`
		TTL            uint32 `json:"ttl"`
	}{}
	err = json.Unmarshal(data, &extraSettings)
	if err != nil {
		return nil, err
	}

	err = validateSettings(domain, extraSettings.Email, extraSettings.Key, extraSettings.UserServiceKey,
		extraSettings.ZoneIdentifier, extraSettings.TTL)
	if err != nil {
		return nil, fmt.Errorf("validating provider specific settings: %w", err)
	}

	return &Provider{
		domain:         domain,
		owner:          owner,
		ipVersion:      ipVersion,
		ipv6Suffix:     ipv6Suffix,
		key:            extraSettings.Key,
		token:          extraSettings.Token,
		email:          extraSettings.Email,
		userServiceKey: extraSettings.UserServiceKey,
		zoneIdentifier: extraSettings.ZoneIdentifier,
		proxied:        extraSettings.Proxied,
		ttl:            extraSettings.TTL,
	}, nil
}

var (
	keyRegex            = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	userServiceKeyRegex = regexp.MustCompile(`^v1\.0.+$`)
	regexEmail          = regexp.MustCompile(`[a-zA-Z0-9-_.+]+@[a-zA-Z0-9-_.]+\.[a-zA-Z]{2,10}`)
)

func validateSettings(domain, email, key, userServiceKey, zoneIdentifier string, ttl uint32) (err error) {
	err = utils.CheckDomain(domain)
	if err != nil {
		return fmt.Errorf("%w: %w", errors.ErrDomainNotValid, err)
	}

	switch {
	case email != "", key != "": // email and key must be provided
		switch {
		case !keyRegex.MatchString(key):
			return fmt.Errorf("%w: key %q does not match regex %q",
				errors.ErrKeyNotValid, key, keyRegex)
		case !regexEmail.MatchString(email):
			return fmt.Errorf("%w: email %q does not match regex %q",
				errors.ErrEmailNotValid, email, regexEmail)
		}
	case userServiceKey != "": // only user service key
		if !userServiceKeyRegex.MatchString(userServiceKey) {
			return fmt.Errorf("%w: %q does not match regex %q",
				errors.ErrUserServiceKeyNotValid, userServiceKey, userServiceKeyRegex)
		}
	default: // constants.API token only
	}
	switch {
	case zoneIdentifier == "":
		return fmt.Errorf("%w", errors.ErrZoneIdentifierNotSet)
	case ttl == 0:
		return fmt.Errorf("%w", errors.ErrTTLNotSet)
	}
	return nil
}

func (p *Provider) String() string {
	return utils.ToString(p.domain, p.owner, constants.Cloudflare, p.ipVersion)
}

func (p *Provider) Domain() string {
	return p.domain
}

func (p *Provider) Owner() string {
	return p.owner
}

func (p *Provider) IPVersion() ipversion.IPVersion {
	return p.ipVersion
}

func (p *Provider) IPv6Suffix() netip.Prefix {
	return p.ipv6Suffix
}

func (p *Provider) Proxied() bool {
	return p.proxied
}

func (p *Provider) BuildDomainName() string {
	return utils.BuildDomainName(p.owner, p.domain)
}

func (p *Provider) HTML() models.HTMLRow {
	return models.HTMLRow{
		Domain:    fmt.Sprintf("<a href=\"http://%s\">%s</a>", p.BuildDomainName(), p.BuildDomainName()),
		Owner:     p.Owner(),
		Provider:  "<a href=\"https://www.cloudflare.com\">Cloudflare</a>",
		IPVersion: p.ipVersion.String(),
	}
}

// createCloudflareClient 创建Cloudflare API客户端.
func (p *Provider) createCloudflareClient() (*cf.Client, error) {
	if p.token != "" {
		// 使用 API token
		return cf.NewClient(option.WithAPIToken(p.token)), nil
	} else if p.email != "" && p.key != "" {
		// 使用 email + API key
		return cf.NewClient(option.WithAPIKey(p.key), option.WithAPIEmail(p.email)), nil
	} else if p.userServiceKey != "" {
		// 使用 user service key
		return cf.NewClient(option.WithAPIKey(p.userServiceKey)), nil
	}
	return nil, fmt.Errorf("no authentication method available")
}

// Obtain domain ID.
func (p *Provider) getRecordID(ctx context.Context, cfClient *cf.Client, newIP netip.Addr) (
	identifier string, upToDate bool, err error,
) {
	// 获取 DNS 记录列表
	records, err := cfClient.DNS.Records.List(ctx, cfdns.RecordListParams{
		ZoneID: cf.F(p.zoneIdentifier),
		Type:   cf.F(cfdns.RecordListParamsType(recordTypeForIP(newIP))),
		Name: cf.F(cfdns.RecordListParamsName{
			Exact: cf.F(p.BuildDomainName()),
		}),
	})
	if err != nil {
		return "", false, fmt.Errorf("error listing DNS records: %w", err)
	}

	switch {
	case len(records.Result) == 0:
		return "", false, fmt.Errorf("%w", errors.ErrReceivedNoResult)
	case len(records.Result) > 1:
		return "", false, fmt.Errorf("%w: %d instead of 1",
			errors.ErrResultsCountReceived, len(records.Result))
	case records.Result[0].Content == newIP.String(): // up to date
		return records.Result[0].ID, true, nil
	}
	return records.Result[0].ID, false, nil
}

func (p *Provider) createRecord(ctx context.Context, cfClinet *cf.Client, ip netip.Addr) (recordID string, err error) {
	// 创建新的 DNS 记录
	result, err := cfClinet.DNS.Records.New(ctx, cfdns.RecordNewParams{
		ZoneID: cf.F(p.zoneIdentifier),
		Body: cfdns.ARecordParam{
			Name:    cf.F(p.BuildDomainName()),
			Type:    cf.F(recordTypeForIP(ip)),
			Content: cf.F(ip.String()),
			TTL:     cf.F(cfdns.TTL(p.ttl)),
			Proxied: cf.F(p.proxied),
			// Settings: cf.F(cfdns.ARecordSettingsParam{
			// 	IPV4Only: cf.F(p.ipVersion == ipversion.IP4),
			// 	IPV6Only: cf.F(p.ipVersion == ipversion.IP6),
			// }),
		},
	})
	if err != nil {
		return "", fmt.Errorf("error creating DNS record: %w", err)
	}
	return result.ID, nil
}

func (p *Provider) Update(ctx context.Context, _ *http.Client, ip netip.Addr) (newIP netip.Addr, err error) {
	cfClient, err := p.createCloudflareClient()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("failed to create cloudflare client: %w", err)
	}
	dnsRecordID, upToDate, err := p.getRecordID(ctx, cfClient, ip)
	switch {
	case stderrors.Is(err, errors.ErrReceivedNoResult):
		dnsRecordID, err = p.createRecord(ctx, cfClient, ip)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("creating record: %w", err)
		}
	case err != nil:
		return netip.Addr{}, fmt.Errorf("getting record id: %w", err)
	case upToDate:
		return ip, nil
	}

	_, err = cfClient.DNS.Records.Update(ctx, dnsRecordID, cfdns.RecordUpdateParams{
		ZoneID: cf.F(p.zoneIdentifier),
		Body: cfdns.ARecordParam{
			Name:    cf.F(p.BuildDomainName()),
			Type:    cf.F(recordTypeForIP(ip)),
			Content: cf.F(ip.String()),
			TTL:     cf.F(cfdns.TTL(p.ttl)),
			Proxied: cf.F(p.proxied),
		},
	})
	if err != nil {
		return netip.Addr{}, fmt.Errorf("updating DNS record: %w", err)
	}
	return ip, nil
}

func recordTypeForIP(ip netip.Addr) cfdns.ARecordType {
	recordType := cfdns.ARecordTypeA
	if ip.Is6() {
		recordType = cfdns.ARecordType(cfdns.AAAARecordTypeAAAA)
	}
	return recordType
}
