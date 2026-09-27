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
	AccessListID          string                 `json:"access_list_id"`
	Meta                  map[string]interface{} `json:"meta"`
}

// APIError represents error responses returned by NPM.
type APIError struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}
