package routing

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusUnknown   Status = "unknown"
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDisabled  Status = "disabled"
)

type Route struct {
	ID            string
	Name          string
	Models        []string
	ModelMappings map[string]string
	Priority      int
	Group         string
	Weight        int
	Price         float64
	Enabled       bool
	MaxRetries    int
	Status        Status
	ResponseTime  time.Duration
}

type ModelMapping struct {
	PlatformModel string
	UpstreamModel string
	ChannelID     string
	Enabled       *bool
}

func (m ModelMapping) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

type Group struct {
	Name     string
	Priority int
}

type Selection struct {
	Route         Route
	UpstreamModel string
}

// ResolveUpstreamModel applies channel-scoped mappings first. A mapping
// without a channel id only applies to channels that advertise its upstream
// model, so routing never needs to infer a provider from the channel URL.
func ResolveUpstreamModel(route *Route, requested string, mappings []ModelMapping) (string, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", false
	}
	if route != nil {
		if mapped := strings.TrimSpace(route.ModelMappings[requested]); mapped != "" {
			return mapped, true
		}
	}
	for _, mapping := range mappings {
		if !mapping.IsEnabled() || strings.TrimSpace(mapping.PlatformModel) != requested {
			continue
		}
		upstream := strings.TrimSpace(mapping.UpstreamModel)
		channelID := strings.TrimSpace(mapping.ChannelID)
		if channelID != "" {
			if route == nil || channelID != route.ID {
				continue
			}
		} else if route == nil || !supportsModel(route.Models, upstream) {
			continue
		}
		return upstream, true
	}
	if route != nil && supportsModel(route.Models, requested) {
		return requested, true
	}
	return "", false
}

func supportsModel(models []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false
	}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == requested || model == "*" {
			return true
		}
	}
	return false
}

type candidate struct {
	route         Route
	upstreamModel string
	groupPriority int
}

func Select(requested string, routes []Route, mappings []ModelMapping, groups []Group, excluded map[string]bool) (*Selection, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return nil, errModelRequired
	}
	groupPriority := make(map[string]int, len(groups))
	for _, group := range groups {
		groupPriority[strings.TrimSpace(group.Name)] = group.Priority
	}
	candidates := make([]candidate, 0, len(routes))
	for _, route := range routes {
		if !route.Enabled || route.Status == StatusDisabled || excluded != nil && excluded[route.ID] {
			continue
		}
		upstream, ok := ResolveUpstreamModel(&route, requested, mappings)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{route: route, upstreamModel: upstream, groupPriority: groupPriority[strings.TrimSpace(route.Group)]})
	}
	if len(candidates) == 0 {
		return nil, errNoRoute
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidateLess(candidates[i], candidates[j]) })
	best := candidates[0]
	return &Selection{Route: best.route, UpstreamModel: best.upstreamModel}, nil
}

func candidateLess(left, right candidate) bool {
	if a, b := healthRank(left.route.Status), healthRank(right.route.Status); a != b {
		return a < b
	}
	if left.groupPriority != right.groupPriority {
		return left.groupPriority > right.groupPriority
	}
	if left.route.Priority != right.route.Priority {
		return left.route.Priority > right.route.Priority
	}
	if left.route.Price != right.route.Price {
		return left.route.Price < right.route.Price
	}
	if left.route.ResponseTime != right.route.ResponseTime {
		return left.route.ResponseTime < right.route.ResponseTime
	}
	if left.route.Weight != right.route.Weight {
		return left.route.Weight > right.route.Weight
	}
	return left.route.ID < right.route.ID
}

func healthRank(status Status) int {
	switch status {
	case StatusHealthy:
		return 0
	case StatusUnknown:
		return 1
	case StatusUnhealthy:
		return 2
	case StatusDisabled:
		return 3
	default:
		return 4
	}
}

func IsRetryableStatus(status int) bool {
	return status == 401 || status == 403 || status == 404 || status == 408 || status == 429 || status >= 500
}

func ResponseFailed(body []byte) bool {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return false
	}
	if looksLikeSSE(body) {
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload != "" && payload != "[DONE]" && jsonEventFailed([]byte(payload)) {
				return true
			}
		}
		return false
	}
	if jsonEventFailed(body) {
		return true
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	response, _ := payload["response"].(map[string]any)
	status, _ := response["status"].(string)
	return status == "failed" || status == "incomplete"
}

func IsRetryableError(status int, body []byte) bool {
	return IsRetryableStatus(status) || ResponseFailed(body)
}

func looksLikeSSE(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "data:") && (strings.Contains(text, "event:") || strings.Contains(text, "response."))
}

func jsonEventFailed(data []byte) bool {
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	typ, _ := payload["type"].(string)
	if typ == "response.failed" || typ == "error" || typ == "response.incomplete" {
		return true
	}
	if response, ok := payload["response"].(map[string]any); ok {
		status, _ := response["status"].(string)
		if status == "failed" || status == "incomplete" {
			return true
		}
	}
	_, hasError := payload["error"]
	return hasError
}

type routingError string

func (e routingError) Error() string { return string(e) }

const (
	errModelRequired routingError = "model is required"
	errNoRoute       routingError = "no available route for model"
)
