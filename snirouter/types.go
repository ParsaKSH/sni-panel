package main

type Mapping struct {
	SNI      string `json:"sni"`
	Upstream string `json:"upstream"`
}

type HTTPPath struct {
	PathPrefix string `json:"path_prefix"`
	Upstream   string `json:"upstream"`
}

type HTTPHost struct {
	Host     string     `json:"host"`
	Paths    []HTTPPath `json:"paths"`
	Fallback string     `json:"fallback,omitempty"`
}

type Config struct {
	// STREAM
	ListenPort443 bool      `json:"listen_port_443"`
	DefaultUP     string    `json:"default_upstream"`
	Mappings      []Mapping `json:"mappings"`

	// HTTP
	HTTPEnabled   bool       `json:"http_enabled"`
	DefaultHTTPUP string     `json:"default_http_upstream"`
	HTTPHosts     []HTTPHost `json:"http_hosts"`

	// Admin
	AdminPath string `json:"admin_path"`
}

type adminCred struct {
	User string
	Pass string
}

type XUICandidate struct {
	ID     int    `json:"id"`
	Remark string `json:"remark,omitempty"`
	Type   string `json:"type"` // "tls" | "http"
	Port   int    `json:"port"`
	SNI    string `json:"sni,omitempty"`
	Host   string `json:"host,omitempty"`
	Path   string `json:"path,omitempty"`
}

type span struct{ s, e int }
