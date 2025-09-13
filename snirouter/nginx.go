package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func generateStreamBlock(c Config) string {
	var mapLines []string
	mapLines = append(mapLines, "        default "+c.DefaultUP+";")
	seenSNI := map[string]struct{}{}
	for _, m := range c.Mappings {
		host := strings.TrimSpace(m.SNI)
		up := strings.TrimSpace(m.Upstream)
		if host == "" || up == "" {
			continue
		}
		key := strings.ToLower(host)
		if _, ok := seenSNI[key]; ok {
			continue
		}
		seenSNI[key] = struct{}{}
		mapLines = append(mapLines, fmt.Sprintf("        %s %s;", host, up))
	}
	return "stream {\n    map $ssl_preread_server_name $backend {\n" +
		strings.Join(mapLines, "\n") + `
    }
    server {
        listen 443 reuseport;
        listen [::]:443 reuseport;
        proxy_pass $backend;
        ssl_preread on;
    }
}
`
}

func baseHTTPCommon() string {
	return `    sendfile on;
    tcp_nopush on;
    types_hash_max_size 2048;
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    access_log /var/log/nginx/access.log;
    error_log /var/log/nginx/error.log;
    gzip on;

`
}

func baseProxyCommon() string {
	return `
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
`
}

func generateHTTPServers(c Config) string {
	if !c.HTTPEnabled {
		return "http {\n" + baseHTTPCommon() + "}\n"
	}
	var b strings.Builder
	common := baseProxyCommon()

	for _, h := range c.HTTPHosts {
		host := strings.TrimSpace(h.Host)
		if host == "" {
			continue
		}
		seenLoc := map[string]struct{}{}
		b.WriteString("server {\n")
		b.WriteString("    listen 80 reuseport;\n")
		b.WriteString("    listen [::]:80 reuseport;\n")
		b.WriteString(fmt.Sprintf("    server_name %s;\n\n", host))

		for _, p := range h.Paths {
			pp := strings.TrimSpace(p.PathPrefix)
			up := strings.TrimSpace(p.Upstream)
			if pp == "" || pp[0] != '/' || up == "" {
				continue
			}
			if _, ok := seenLoc[pp]; ok {
				continue
			}
			seenLoc[pp] = struct{}{}
			b.WriteString(fmt.Sprintf("    location ^~ %s {\n        proxy_pass http://%s;%s    }\n\n", pp, up, common))
		}

		if strings.TrimSpace(h.Fallback) != "" {
			b.WriteString(fmt.Sprintf("    location / {\n        proxy_pass http://%s;%s    }\n", h.Fallback, common))
		} else if strings.TrimSpace(c.DefaultHTTPUP) != "" {
			b.WriteString(fmt.Sprintf("    location / {\n        proxy_pass http://%s;%s    }\n", c.DefaultHTTPUP, common))
		} else {
			b.WriteString("    location / { return 404; }\n")
		}
		b.WriteString("}\n\n")
	}

	b.WriteString("server {\n")
	b.WriteString("    listen 80 default_server reuseport;\n")
	b.WriteString("    listen [::]:80 default_server reuseport;\n")
	b.WriteString("    server_name _;\n")
	if strings.TrimSpace(c.DefaultHTTPUP) != "" {
		b.WriteString(fmt.Sprintf("    location / {\n        proxy_pass http://%s;%s    }\n", c.DefaultHTTPUP, common))
	} else {
		b.WriteString("    return 444;\n")
	}
	b.WriteString("}\n")

	return "http {\n" + baseHTTPCommon() + b.String() + "}\n"
}

func generateNginxConf(c Config) string {
	return `user www-data;
worker_processes auto;
worker_rlimit_nofile 2000000;
pid /run/nginx.pid;
include /etc/nginx/modules-enabled/*.conf;

events { use epoll; worker_connections 131072; multi_accept on; }

` + generateStreamBlock(c) + `
` + generateHTTPServers(c)
}

func writeNginxConf(c Config) error { return writeAtomic(nginxConf, []byte(generateNginxConf(c)), 0644) }

func nginxTest() error {
	var out bytes.Buffer
	cmd := exec.Command("nginx", "-t")
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nginx -t failed: %v\n%s", err, out.String())
	}
	return nil
}

func nginxReload() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return sudoRun("bash", "-lc", "systemctl reload nginx || systemctl restart nginx")
	}
	return sudoRun("nginx", "-s", "reload")
}

func applyAndReload() error {
	configMutex.Lock()
	defer configMutex.Unlock()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := writeNginxConf(cfg); err != nil {
		return err
	}
	if err := nginxTest(); err != nil {
		return err
	}
	return nginxReload()
}
