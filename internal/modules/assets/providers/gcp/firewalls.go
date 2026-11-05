package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ArkGravity/optimus/internal/models"
	"github.com/ArkGravity/optimus/internal/modules/assets/query"
	"github.com/ArkGravity/optimus/internal/modules/assets/resource"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"google.golang.org/api/compute/v1"
)

const maxFirewallPageBytes = 16 << 20

var firewallName = regexp.MustCompile(`^[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?$`)

// Compute's Priority scalar loses JSON presence. Observe only priority and ID
// before the official SDK decodes the identical bounded page. State belongs to
// this fresh client/unit and is replaced on each page, never cached or published.
type firewallPriorities struct{ values map[string]int32 }
type firewallTransport struct {
	base       http.RoundTripper
	priorities *firewallPriorities
}

func (t firewallTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFirewallPageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFirewallPageBytes {
		return nil, configError()
	}
	var page struct {
		Items []struct {
			ID       string          `json:"id"`
			Priority json.RawMessage `json:"priority"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil, configError()
	}
	values := make(map[string]int32, len(page.Items))
	for _, item := range page.Items {
		if item.ID == "" {
			return nil, configError()
		}
		if _, exists := values[item.ID]; exists {
			return nil, configError()
		}
		priority := int32(1000) // Documented API default; explicit zero is distinct.
		if len(item.Priority) != 0 {
			if string(item.Priority) == "null" || json.Unmarshal(item.Priority, &priority) != nil || priority < 0 || priority > 65535 {
				return nil, configError()
			}
		}
		values[item.ID] = priority
	}
	t.priorities.values = values
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}

func (p *Provider) firewallClient(ctx context.Context, key *credentials.CloudKey) (*compute.Service, *firewallPriorities, error) {
	presence := &firewallPriorities{}
	opts, err := p.clientOptions(ctx, key, computeEndpoint, func(base http.RoundTripper) http.RoundTripper {
		return firewallTransport{base: base, priorities: presence}
	})
	if err != nil {
		return nil, nil, err
	}
	client, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, nil, safeError(err)
	}
	return client, presence, nil
}

func (p *Provider) firewalls(ctx context.Context, client *compute.Service, presence *firewallPriorities, project string) ([]models.GCPFirewall, error) {
	rows := []models.GCPFirewall{}
	pagination := pages{}
	token := ""
	for {
		page, err := client.Firewalls.List(project).MaxResults(500).PageToken(token).Context(ctx).Do()
		if err != nil {
			return nil, requestError(ctx, err)
		}
		if page == nil || len(presence.values) != len(page.Items) {
			return nil, configError()
		}
		if page.Warning != nil {
			if err := partialWarning(page.Warning.Code, len(page.Items)); err != nil {
				return nil, err
			}
		}
		for _, item := range page.Items {
			if item == nil || item.Id == 0 || !firewallName.MatchString(item.Name) {
				return nil, configError()
			}
			id := strconv.FormatUint(item.Id, 10)
			if err := pagination.unique(id); err != nil {
				return nil, err
			}
			if err := pagination.unique("name/" + item.Name); err != nil {
				return nil, err
			}
			priority, ok := presence.values[id]
			if !ok {
				return nil, configError()
			}
			if item.SelfLink != "" {
				ref, err := canonicalRef(item.SelfLink, project, "firewalls")
				if err != nil || ref != "projects/"+project+"/global/firewalls/"+item.Name {
					return nil, configError()
				}
			}
			network, err := canonicalRef(item.Network, project, "networks")
			if err != nil {
				return nil, err
			}
			rules, direction, err := firewallRules(item, priority)
			if err != nil {
				return nil, err
			}
			inbound, outbound := []resource.Rule{}, []resource.Rule{}
			if direction == "inbound" {
				inbound = rules
			} else {
				outbound = rules
			}
			disabled := item.Disabled
			nativeDirection := "INGRESS"
			if direction == "outbound" {
				nativeDirection = "EGRESS"
			}
			facts := query.FirewallNativeDetails{Priority: &priority, Disabled: &disabled, Direction: nativeDirection}
			rows = append(rows, models.GCPFirewall{FirewallID: id, Name: item.Name, NetworkRef: network, Description: item.Description, InboundRules: jsonValue(inbound), OutboundRules: jsonValue(outbound), Tags: jsonValue(map[string]string{}), NativeDetails: jsonValue(facts)})
		}
		done, err := pagination.next(page.NextPageToken)
		if err != nil {
			return nil, err
		}
		if done {
			return rows, nil
		}
		token = page.NextPageToken
	}
}

func firewallRules(item *compute.Firewall, priority int32) ([]resource.Rule, string, error) {
	direction := "inbound"
	switch item.Direction {
	case "", "INGRESS":
	case "EGRESS":
		direction = "outbound"
	default:
		return nil, "", configError()
	}
	if (len(item.Allowed) == 0) == (len(item.Denied) == 0) {
		return nil, "", configError()
	}
	if direction == "outbound" && (len(item.SourceTags) != 0 || len(item.SourceServiceAccounts) != 0 || len(item.SourceRanges) != 0) {
		return nil, "", configError()
	}
	if (len(item.SourceServiceAccounts) != 0 || len(item.TargetServiceAccounts) != 0) && (len(item.SourceTags) != 0 || len(item.TargetTags) != 0) {
		return nil, "", configError()
	}
	sources, destinations := []string{}, []string{}
	for _, raw := range item.SourceRanges {
		if raw == "" {
			return nil, "", configError()
		}
		if err := appendCIDR(&sources, raw); err != nil {
			return nil, "", err
		}
	}
	for _, raw := range item.DestinationRanges {
		if raw == "" {
			return nil, "", configError()
		}
		if err := appendCIDR(&destinations, raw); err != nil {
			return nil, "", err
		}
	}
	for _, values := range [][]string{item.SourceTags, item.TargetTags, item.SourceServiceAccounts, item.TargetServiceAccounts} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, "", configError()
			}
		}
	}
	enabled := !item.Disabled // The API explicitly defines omitted disabled as enabled.
	base := resource.Rule{Direction: direction, Priority: &priority, Enabled: &enabled, Description: item.Description, SourceRanges: sources, DestinationRanges: destinations, SourceNetworkTags: item.SourceTags, TargetNetworkTags: item.TargetTags, SourceServiceAccounts: item.SourceServiceAccounts, TargetServiceAccounts: item.TargetServiceAccounts}
	rules := []resource.Rule{}
	appendRules := func(protocol string, ports []string, action string) error {
		if !validProtocol(protocol) {
			return configError()
		}
		if len(ports) > 0 && protocol != "tcp" && protocol != "udp" && protocol != "6" && protocol != "17" {
			return configError()
		}
		allPorts := len(ports) == 0
		if allPorts {
			ports = []string{""}
		}
		for _, raw := range ports {
			rule := base
			rule.Protocol = protocol
			rule.Action = action
			if raw != "" {
				parts := strings.Split(raw, "-")
				if len(parts) > 2 {
					return configError()
				}
				from, err := strconv.ParseUint(parts[0], 10, 16)
				if err != nil {
					return configError()
				}
				to := from
				if len(parts) == 2 {
					to, err = strconv.ParseUint(parts[1], 10, 16)
					if err != nil || to < from {
						return configError()
					}
				}
				low, high := int32(from), int32(to)
				rule.FromPort = &low
				rule.ToPort = &high
			} else if !allPorts {
				return configError()
			}
			rules = append(rules, rule)
		}
		return nil
	}
	for _, entry := range item.Allowed {
		if entry == nil {
			return nil, "", configError()
		}
		if err := appendRules(entry.IPProtocol, entry.Ports, "allow"); err != nil {
			return nil, "", err
		}
	}
	for _, entry := range item.Denied {
		if entry == nil {
			return nil, "", configError()
		}
		if err := appendRules(entry.IPProtocol, entry.Ports, "deny"); err != nil {
			return nil, "", err
		}
	}
	return rules, direction, nil
}

func validProtocol(value string) bool {
	switch value {
	case "all", "tcp", "udp", "icmp", "icmpv6", "esp", "ah", "ipip", "sctp":
		return true
	}
	_, err := strconv.ParseUint(value, 10, 8)
	return err == nil
}
