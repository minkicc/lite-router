package routing

import "testing"

func TestRetryableStatusClassification(t *testing.T) {
	for _, status := range []int{408, 425, 429, 500, 502, 503, 504} {
		if !IsRetryableStatus(status) || !IsRetryableOnSameRoute(status) {
			t.Fatalf("status %d should retry on the same route", status)
		}
	}
	for _, status := range []int{401, 403, 404} {
		if !IsRetryableStatus(status) || IsRetryableOnSameRoute(status) {
			t.Fatalf("status %d should switch routes without same-route retry", status)
		}
	}
}

func TestResolveMappingUsesAdvertisedUpstreamModel(t *testing.T) {
	mappings := []ModelMapping{{
		PlatformModel: "gpt-5.6-sol",
		UpstreamModel: "deepseek-v4-pro",
	}}
	deepseek := &Route{ID: "#4", Models: []string{"deepseek-v4-pro"}}
	mdlbus := &Route{ID: "#3", Models: []string{"gpt-5.6-sol"}}

	if got, ok := ResolveUpstreamModel(deepseek, "gpt-5.6-sol", mappings); !ok || got != "deepseek-v4-pro" {
		t.Fatalf("deepseek mapping = %q, %v", got, ok)
	}
	if got, ok := ResolveUpstreamModel(mdlbus, "gpt-5.6-sol", mappings); !ok || got != "gpt-5.6-sol" {
		t.Fatalf("mdlbus native model = %q, %v", got, ok)
	}
}

func TestResolveMappingCanBeScopedToChannel(t *testing.T) {
	mappings := []ModelMapping{{
		PlatformModel: "alias",
		UpstreamModel: "shared-upstream",
		ChannelID:     "#2",
	}}
	first := &Route{ID: "#1", Models: []string{"shared-upstream"}}
	second := &Route{ID: "#2", Models: []string{"shared-upstream"}}

	if got, ok := ResolveUpstreamModel(first, "alias", mappings); ok {
		t.Fatalf("unexpected mapping on #1: %q", got)
	}
	if got, ok := ResolveUpstreamModel(second, "alias", mappings); !ok || got != "shared-upstream" {
		t.Fatalf("#2 mapping = %q, %v", got, ok)
	}
}

func TestSelectFallsBackToLowerPriorityMappedChannel(t *testing.T) {
	routes := []Route{
		{ID: "#3", Models: []string{"gpt-5.6-sol"}, Priority: 3, Enabled: true, Status: StatusHealthy},
		{ID: "#4", Models: []string{"deepseek-v4-pro"}, Priority: 0, Enabled: true, Status: StatusHealthy},
	}
	mappings := []ModelMapping{{PlatformModel: "gpt-5.6-sol", UpstreamModel: "deepseek-v4-pro"}}

	selected, err := Select("gpt-5.6-sol", routes, mappings, nil, map[string]bool{"#3": true})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Route.ID != "#4" || selected.UpstreamModel != "deepseek-v4-pro" {
		t.Fatalf("selection = %s/%s", selected.Route.ID, selected.UpstreamModel)
	}
}
