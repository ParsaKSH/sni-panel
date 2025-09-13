package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()
	cfg, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(cfg)
}

func handleSetDefault(w http.ResponseWriter, r *http.Request) {
	var in struct{ Upstream string `json:"upstream"` }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Upstream) == "" {
		http.Error(w, "invalid upstream", 400)
		return
	}
	configMutex.Lock()
	cfg, err := loadConfig()
	if err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	cfg.DefaultUP = strings.TrimSpace(in.Upstream)
	if err := saveConfig(cfg); err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	configMutex.Unlock()
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func handleAddMapping(w http.ResponseWriter, r *http.Request) {
	var m Mapping
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}
	m.SNI = strings.TrimSpace(m.SNI)
	m.Upstream = strings.TrimSpace(m.Upstream)
	if m.SNI == "" || m.Upstream == "" {
		http.Error(w, "sni/upstream required", 400)
		return
	}
	configMutex.Lock()
	cfg, err := loadConfig()
	if err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	found := false
	for i := range cfg.Mappings {
		if strings.EqualFold(cfg.Mappings[i].SNI, m.SNI) {
			cfg.Mappings[i].Upstream = m.Upstream
			found = true
			break
		}
	}
	if !found {
		cfg.Mappings = append(cfg.Mappings, m)
	}
	if err := saveConfig(cfg); err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	configMutex.Unlock()
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func makeDeleteStreamHandler(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := base + "/api/stream/mapping/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.Error(w, "bad path", 400)
			return
		}
		target := strings.TrimPrefix(r.URL.Path, prefix)
		if strings.TrimSpace(target) == "" {
			http.Error(w, "missing sni", 400)
			return
		}

		configMutex.Lock()
		cfg, err := loadConfig()
		if err != nil {
			configMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}
		var out []Mapping
		for _, x := range cfg.Mappings {
			if !strings.EqualFold(x.SNI, target) {
				out = append(out, x)
			}
		}
		cfg.Mappings = out
		if err := saveConfig(cfg); err != nil {
			configMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}
		configMutex.Unlock()
		if err := applyAndReload(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(204)
	}
}

func handleSetDefaultHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct{ Upstream string `json:"upstream"` }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Upstream) == "" {
		http.Error(w, "invalid upstream", 400)
		return
	}
	configMutex.Lock()
	cfg, err := loadConfig()
	if err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	cfg.HTTPEnabled = true
	cfg.DefaultHTTPUP = strings.TrimSpace(in.Upstream)
	if err := saveConfig(cfg); err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	configMutex.Unlock()
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func handleAddHTTPRoute(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host       string `json:"host"`
		PathPrefix string `json:"path_prefix"`
		Upstream   string `json:"upstream"`
		Fallback   bool   `json:"fallback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}
	in.Host = strings.TrimSpace(in.Host)
	in.PathPrefix = strings.TrimSpace(in.PathPrefix)
	in.Upstream = strings.TrimSpace(in.Upstream)
	if in.Host == "" || in.Upstream == "" || (!in.Fallback && (in.PathPrefix == "" || !strings.HasPrefix(in.PathPrefix, "/"))) {
		http.Error(w, "host/upstream (and valid path_prefix if not fallback) required", 400)
		return
	}

	configMutex.Lock()
	cfg, err := loadConfig()
	if err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	cfg.HTTPEnabled = true
	found := false
	for i := range cfg.HTTPHosts {
		if strings.EqualFold(cfg.HTTPHosts[i].Host, in.Host) {
			found = true
			if in.Fallback {
				cfg.HTTPHosts[i].Fallback = in.Upstream
			} else {
				replaced := false
				for j := range cfg.HTTPHosts[i].Paths {
					if cfg.HTTPHosts[i].Paths[j].PathPrefix == in.PathPrefix {
						cfg.HTTPHosts[i].Paths[j].Upstream = in.Upstream
						replaced = true
						break
					}
				}
				if !replaced {
					cfg.HTTPHosts[i].Paths = append(cfg.HTTPHosts[i].Paths, HTTPPath{PathPrefix: in.PathPrefix, Upstream: in.Upstream})
				}
			}
			break
		}
	}
	if !found {
		nh := HTTPHost{Host: in.Host}
		if in.Fallback {
			nh.Fallback = in.Upstream
		} else {
			nh.Paths = []HTTPPath{{PathPrefix: in.PathPrefix, Upstream: in.Upstream}}
		}
		cfg.HTTPHosts = append(cfg.HTTPHosts, nh)
	}
	if err := saveConfig(cfg); err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	configMutex.Unlock()
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func makeDeleteHTTPRouteHandler(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := base + "/api/http/route/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.Error(w, "bad path", 400)
			return
		}
		host := strings.TrimPrefix(r.URL.Path, prefix)
		if strings.TrimSpace(host) == "" {
			http.Error(w, "missing host", 400)
			return
		}
		pathQ := r.URL.Query().Get("path")

		configMutex.Lock()
		cfg, err := loadConfig()
		if err != nil {
			configMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}
		var outHosts []HTTPHost
		for _, h := range cfg.HTTPHosts {
			if !strings.EqualFold(h.Host, host) {
				outHosts = append(outHosts, h)
				continue
			}
			if pathQ == "" || pathQ == "/" {
				continue
			}
			var newPaths []HTTPPath
			for _, p := range h.Paths {
				if p.PathPrefix != pathQ {
					newPaths = append(newPaths, p)
				}
			}
			h.Paths = newPaths
			outHosts = append(outHosts, h)
		}
		cfg.HTTPHosts = outHosts
		if err := saveConfig(cfg); err != nil {
			configMutex.Unlock()
			http.Error(w, err.Error(), 500)
			return
		}
		configMutex.Unlock()
		if err := applyAndReload(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(204)
	}
}

func handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func handleInstallNginx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := installNginx(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = sudoRun("bash", "-lc", "systemctl enable nginx --now || true")
	w.WriteHeader(204)
}

func handleXUIStatus(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(struct {
		Present bool   `json:"present"`
		Path    string `json:"path"`
	}{Present: xuiPresent(), Path: xuiDBPath})
}

func handleXUIScan(w http.ResponseWriter, r *http.Request) {
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	_ = writeAtomic(cachePath, []byte("{}"), 0644)

	items, err := scanXUI()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	payload := struct {
		UpdatedAt string         `json:"updated_at"`
		Items     []XUICandidate `json:"items"`
	}{UpdatedAt: time.Now().UTC().Format(time.RFC3339), Items: items}

	if b, _ := json.MarshalIndent(payload, "", "  "); len(b) > 0 {
		_ = writeAtomic(cachePath, b, 0644)
	}

	_ = json.NewEncoder(w).Encode(struct {
		Items []XUICandidate `json:"items"`
	}{Items: items})
}

func handleXUIApply(w http.ResponseWriter, r *http.Request) {
	var in struct{ IDs []int `json:"ids"` }
	_ = json.NewDecoder(r.Body).Decode(&in)
	idset := map[int]bool{}
	for _, id := range in.IDs {
		idset[id] = true
	}

	items, err := scanXUI()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	configMutex.Lock()
	cfg, err := loadConfig()
	if err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	applyCount := 0

	for _, it := range items {
		if len(idset) > 0 && !idset[it.ID] {
			continue
		}
		if it.Port <= 0 || it.Port > 65535 {
			continue
		}
		up := "127.0.0.1:" + strconv.Itoa(it.Port)

		if it.Type == "tls" && it.SNI != "" {
			found := false
			for i := range cfg.Mappings {
				if strings.EqualFold(cfg.Mappings[i].SNI, it.SNI) {
					cfg.Mappings[i].Upstream = up
					found = true
					break
				}
			}
			if !found {
				cfg.Mappings = append(cfg.Mappings, Mapping{SNI: it.SNI, Upstream: up})
			}
			applyCount++
		} else if it.Type == "http" && it.Host != "" {
			cfg.HTTPEnabled = true
			hostFound := false
			for i := range cfg.HTTPHosts {
				if strings.EqualFold(cfg.HTTPHosts[i].Host, it.Host) {
					hostFound = true
					replaced := false
					for j := range cfg.HTTPHosts[i].Paths {
						if cfg.HTTPHosts[i].Paths[j].PathPrefix == it.Path {
							cfg.HTTPHosts[i].Paths[j].Upstream = up
							replaced = true
							break
						}
					}
					if !replaced {
						cfg.HTTPHosts[i].Paths = append(cfg.HTTPHosts[i].Paths, HTTPPath{PathPrefix: it.Path, Upstream: up})
					}
					break
				}
			}
			if !hostFound {
				cfg.HTTPHosts = append(cfg.HTTPHosts, HTTPHost{Host: it.Host, Paths: []HTTPPath{{PathPrefix: it.Path, Upstream: up}}})
			}
			applyCount++
		}
	}

	if err := saveConfig(cfg); err != nil {
		configMutex.Unlock()
		http.Error(w, err.Error(), 500)
		return
	}
	configMutex.Unlock()

	if applyCount == 0 {
		http.Error(w, "no applicable entries found", 400)
		return
	}
	if err := applyAndReload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
