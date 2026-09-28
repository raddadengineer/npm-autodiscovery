package npm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAccessListID(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"nil", nil, 0},
		{"empty string", "", 0},
		{"whitespace string", "   ", 0},
		{"zero string", "0", 0},
		{"none string", "none", 0},
		{"public string", "public", 0},
		{"valid string integer", "1", 1},
		{"valid string multidigit", "42", 42},
		{"string with spaces", " 15 ", 15},
		{"int 0", 0, 0},
		{"int positive", 5, 5},
		{"int negative", -1, 0},
		{"int64", int64(12), 12},
		{"float64", float64(7), 7},
		{"invalid string", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAccessListID(tt.input)
			if got != tt.expected {
				t.Errorf("ParseAccessListID(%v) = %d; expected %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestProxyHostRequestJSON(t *testing.T) {
	req := ProxyHostRequest{
		DomainNames:  []string{"pbs.halnt.dev"},
		AccessListID: 0,
	}

	bytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ProxyHostRequest: %v", err)
	}

	jsonStr := string(bytes)
	if !strings.Contains(jsonStr, `"access_list_id":0`) {
		t.Errorf("Expected json to contain '\"access_list_id\":0', got: %s", jsonStr)
	}

	req.AccessListID = 3
	bytes, err = json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ProxyHostRequest: %v", err)
	}

	jsonStr = string(bytes)
	if !strings.Contains(jsonStr, `"access_list_id":3`) {
		t.Errorf("Expected json to contain '\"access_list_id\":3', got: %s", jsonStr)
	}
}
