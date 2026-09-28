package npm

import "encoding/json"

// AuthRequest represents credentials sent to /api/tokens.
type AuthRequest struct {
	Identity string `json:"identity"`
	Secret   string `json:"secret"`
}

// TokenResponse represents the JWT token returned by /api/tokens.
type TokenResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

// FlexBool handles JSON booleans that may be sent as boolean or integer (0/1) by NPM.
type FlexBool bool

func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*fb = FlexBool(b)
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*fb = FlexBool(n != 0)
		return nil
	}
	return nil
}

// ProxyHostLocation represents a custom location path route within a proxy host.
type ProxyHostLocation struct {
	Path           string `json:"path"`
	ForwardScheme  string `json:"forward_scheme"`
	ForwardHost    string `json:"forward_host"`
	ForwardPort    int    `json:"forward_port"`
	AdvancedConfig string `json:"advanced_config,omitempty"`
}

// ProxyHost represents an existing proxy host record in Nginx Proxy Manager.
type ProxyHost struct {
	ID                    int                    `json:"id"`
	CreatedOn             string                 `json:"created_on"`
	ModifiedOn            string                 `json:"modified_on"`
	OwnerUserID           int                    `json:"owner_user_id"`
	DomainNames           []string               `json:"domain_names"`
	ForwardHost           string                 `json:"forward_host"`
	ForwardPort           int                    `json:"forward_port"`
	ForwardScheme         string                 `json:"forward_scheme"`
	CertificateID         interface{}            `json:"certificate_id"`
	SSLForced             FlexBool               `json:"ssl_forced"`
	CachingEnabled        FlexBool               `json:"caching_enabled"`
	BlockExploits         FlexBool               `json:"block_exploits"`
	AllowWebsocketUpgrade FlexBool               `json:"allow_websocket_upgrade"`
	HTTP2Support          FlexBool               `json:"http2_support"`
	HSTSEnabled           FlexBool               `json:"hsts_enabled"`
	HSTSSubdomains        FlexBool               `json:"hsts_subdomains"`
	AdvancedConfig        string                 `json:"advanced_config"`
	Locations             []ProxyHostLocation    `json:"locations,omitempty"`
	AccessListID          interface{}            `json:"access_list_id"`
	Meta                  map[string]interface{} `json:"meta"`
	Enabled               FlexBool               `json:"enabled"`
}

// ProxyHostRequest represents the payload to create or update a proxy host in NPM.
type ProxyHostRequest struct {
	DomainNames           []string               `json:"domain_names"`
	ForwardScheme         string                 `json:"forward_scheme"`
	ForwardHost           string                 `json:"forward_host"`
	ForwardPort           int                    `json:"forward_port"`
	CertificateID         interface{}            `json:"certificate_id"`
	SSLForced             bool                   `json:"ssl_forced"`
	HSTSEnabled           bool                   `json:"hsts_enabled"`
	HSTSSubdomains        bool                   `json:"hsts_subdomains"`
	CachingEnabled        bool                   `json:"caching_enabled"`
	AllowWebsocketUpgrade bool                   `json:"allow_websocket_upgrade"`
	BlockExploits         bool                   `json:"block_exploits"`
	HTTP2Support          bool                   `json:"http2_support"`
	AdvancedConfig        string                 `json:"advanced_config"`
	Locations             []ProxyHostLocation    `json:"locations,omitempty"`
	AccessListID          int                    `json:"access_list_id"`
	Meta                  map[string]interface{} `json:"meta"`
}

// ParseAccessListID safely converts strings, numbers, or empty interface values
// into the integer required by the Nginx Proxy Manager REST API schema.
// Defaults to 0 (no access list / publicly accessible).
func ParseAccessListID(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		if val < 0 {
			return 0
		}
		return val
	case int64:
		if val < 0 {
			return 0
		}
		return int(val)
	case float64:
		if val < 0 {
			return 0
		}
		return int(val)
	case string:
		s := ""
		for _, r := range val {
			if r != ' ' && r != '\t' && r != '\r' && r != '\n' {
				s += string(r)
			}
		}
		if s == "" || s == "0" || s == "none" || s == "public" {
			return 0
		}
		var parsed int
		var factor = 1
		var valid = true
		for i, r := range s {
			if i == 0 && r == '-' {
				valid = false
				break
			}
			if r >= '0' && r <= '9' {
				parsed = parsed*10 + int(r-'0')
			} else {
				valid = false
				break
			}
		}
		_ = factor
		if valid {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

// Stream represents an existing Layer 4 TCP/UDP stream record in Nginx Proxy Manager.
type Stream struct {
	ID             int                    `json:"id"`
	CreatedOn      string                 `json:"created_on"`
	ModifiedOn     string                 `json:"modified_on"`
	OwnerUserID    int                    `json:"owner_user_id"`
	IncomingPort   int                    `json:"incoming_port"`
	ForwardingHost string                 `json:"forwarding_host"`
	ForwardingPort int                    `json:"forwarding_port"`
	TCPForwarding  FlexBool               `json:"tcp_forwarding"`
	UDPForwarding  FlexBool               `json:"udp_forwarding"`
	Meta           map[string]interface{} `json:"meta"`
	Enabled        FlexBool               `json:"enabled"`
}

// StreamRequest represents the payload to create or update a stream in NPM.
type StreamRequest struct {
	IncomingPort   int                    `json:"incoming_port"`
	ForwardingHost string                 `json:"forwarding_host"`
	ForwardingPort int                    `json:"forwarding_port"`
	TCPForwarding  bool                   `json:"tcp_forwarding"`
	UDPForwarding  bool                   `json:"udp_forwarding"`
	Meta           map[string]interface{} `json:"meta"`
}

// APIError represents error responses returned by NPM.
type APIError struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// Certificate represents an SSL certificate record in Nginx Proxy Manager.
type Certificate struct {
	ID          int      `json:"id"`
	CreatedOn   string   `json:"created_on"`
	ModifiedOn  string   `json:"modified_on"`
	Provider    string   `json:"provider"`
	NiceName    string   `json:"nice_name"`
	DomainNames []string `json:"domain_names"`
	ExpiresOn   string   `json:"expires_on"`
}

