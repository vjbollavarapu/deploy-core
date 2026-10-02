package volumes

import "testing"

func TestDockerCommandLabelsOmitControlPlaneMetadata(t *testing.T) {
	labels := dockerCommandLabels(map[string]any{
		"readOnly":                   false,
		"priority":                   1,
		"nested":                     map[string]any{"a": "b"},
		"empty":                      "   ",
		"deploycore.managed":         "true",
		"deploycore.owner":           "platform",
		"deploycore.organization_id": "org-1",
		"tier":                       "app",
	})
	if _, ok := labels["readOnly"]; ok {
		t.Fatalf("readOnly leaked into docker labels: %#v", labels)
	}
	if _, ok := labels["priority"]; ok {
		t.Fatalf("numeric label leaked: %#v", labels)
	}
	if _, ok := labels["nested"]; ok {
		t.Fatalf("object label leaked: %#v", labels)
	}
	if _, ok := labels["empty"]; ok {
		t.Fatalf("blank label leaked: %#v", labels)
	}
	if labels["deploycore.managed"] != "true" || labels["deploycore.owner"] != "platform" || labels["deploycore.organization_id"] != "org-1" || labels["tier"] != "app" {
		t.Fatalf("string labels = %#v", labels)
	}
	for _, value := range labels {
		if value == "" {
			t.Fatal("empty docker label")
		}
	}
}
