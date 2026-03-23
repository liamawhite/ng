package gateway

import (
	"testing"
)

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"id", "id"},
		{"title", "title"},
		{"area_id", "areaId"},
		{"parent_id", "parentId"},
		{"project_ids", "projectIds"},
		{"area_ids", "areaIds"},
		{"item_parent_id_field", "itemParentIdField"},
		{"request_id_field", "requestIdField"},
		{"response_items_field", "responseItemsField"},
		{"already_camel", "alreadyCamel"}, // no-op on the no-underscore parts
		{"", ""},
		{"single", "single"},
		{"__double_leading", "DoubleLeading"}, // empty segments between underscores are skipped
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := toCamelCase(tc.in)
			if got != tc.want {
				t.Errorf("toCamelCase(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMethodNamePromotion(t *testing.T) {
	// Validate the first-letter capitalisation used in execute() to accept
	// lowerCamelCase method names from clients.
	tests := []struct {
		in   string
		want string
	}{
		{"list", "List"},
		{"listAreas", "ListAreas"},
		{"List", "List"},          // already upper — unchanged
		{"ListAreas", "ListAreas"}, // already upper — unchanged
		{"get", "Get"},
		{"create", "Create"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := upperFirst(tc.in)
			if got != tc.want {
				t.Errorf("upperFirst(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
