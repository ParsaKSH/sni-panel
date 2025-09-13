package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ---------- REGEX PATTERNS ----------
var (
	reHasAnyStream = regexp.MustCompile(`"tlsSettings"|"realitySettings"|"tcpSettings"|"wsSettings"|"httpSettings"|"security"`)
	reInboundNear  = regexp.MustCompile(`inbound-([0-9]{1,5})\s*\{`)
	rePortField    = regexp.MustCompile(`"port"\s*:\s*([0-9]{1,5})`)

	// TLS: tlsSettings.serverName
	reTLSserverName = regexp.MustCompile(`(?s)"tlsSettings"\s*:\s*\{.*?"serverName"\s*:\s*"([^"]+)"`)

	// REALITY: realitySettings.serverNames[0]
	reRealitySN = regexp.MustCompile(`(?s)"realitySettings"\s*:\s*\{.*?"serverNames"\s*:\s*\[\s*"([^"]+)"`)

	// TCP (http header emulation): request.headers.Host / host and request.path
	reTCPHost = regexp.MustCompile(`(?s)"tcpSettings"\s*:\s*\{.*?"request"\s*:\s*\{.*?"headers"\s*:\s*\{.*?(?:"Host"|"host")\s*:\s*(\[[^\]]+\]|"[^"]+")`)
	reTCPPath = regexp.MustCompile(`(?s)"tcpSettings"\s*:\s*\{.*?"request"\s*:\s*\{.*?"path"\s*:\s*(\[[^\]]+\]|"[^"]+")`)

	// WS: wsSettings.headers.Host / host and wsSettings.path
	reWSHost = regexp.MustCompile(`(?s)"wsSettings"\s*:\s*\{.*?"headers"\s*:\s*\{.*?(?:"Host"|"host")\s*:\s*(\[[^\]]+\]|"[^"]+")`)
	reWSPath = regexp.MustCompile(`(?s)"wsSettings"\s*:\s*\{.*?"path"\s*:\s*"([^"]*)"`)
	// HTTP/2 h2c: httpSettings.host / path
	reHTTPHost = regexp.MustCompile(`(?s)"httpSettings"\s*:\s*\{.*?"host"\s*:\s*(\[[^\]]+\]|"[^"]+")`)
	reHTTPPath = regexp.MustCompile(`(?s)"httpSettings"\s*:\s*\{.*?"path"\s*:\s*"([^"]*)"`)
)

func xuiPresent() bool {
	st, err := os.Stat(xuiDBPath)
	return err == nil && !st.IsDir()
}

func extractAllJSONObjects(data []byte, maxSize int) []span {
	var out []span
	inStr := false
	esc := false
	depth := 0
	start := -1
	for i := 0; i < len(data); i++ {
		b := data[i]
		if inStr {
			if esc {
				esc = false
			} else if b == '\\' {
				esc = true
			} else if b == '"' {
				inStr = false
			}
			continue
		}
		if b == '"' {
			inStr = true
			continue
		}
		if b == '{' {
			if depth == 0 {
				start = i
			}
			depth++
			continue
		}
		if b == '}' {
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					if i+1-start <= maxSize {
						out = append(out, span{s: start, e: i + 1})
					}
					start = -1
				}
			}
			continue
		}
	}
	return out
}

func pickFirstString(jsonish string) string {
	s := strings.TrimSpace(jsonish)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		if i := strings.Index(s[1:], `"`); i >= 0 {
			return s[1 : 1+i]
		}
		return strings.Trim(s, `"`)
	}
	if strings.HasPrefix(s, "[") {
		if m := regexp.MustCompile(`^\s*\[\s*"([^"]+)"`).FindStringSubmatch(s); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func findSNI(chunk string) string {
	if m := reTLSserverName.FindStringSubmatch(chunk); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	if m := reRealitySN.FindStringSubmatch(chunk); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func findHTTPHostPath(chunk string) (string, string) {
	if m := reTCPHost.FindStringSubmatch(chunk); len(m) == 2 {
		h := pickFirstString(m[1])
		p := "/"
		if mp := reTCPPath.FindStringSubmatch(chunk); len(mp) == 2 {
			p = pickFirstString(mp[1])
			if p == "" {
				p = "/"
			}
		}
		return strings.TrimSpace(h), p
	}
	if m := reWSHost.FindStringSubmatch(chunk); len(m) == 2 {
		h := pickFirstString(m[1])
		p := "/"
		if mp := reWSPath.FindStringSubmatch(chunk); len(mp) == 2 && strings.TrimSpace(mp[1]) != "" {
			p = mp[1]
		}
		return strings.TrimSpace(h), p
	}
	if m := reHTTPHost.FindStringSubmatch(chunk); len(m) == 2 {
		h := pickFirstString(m[1])
		p := "/"
		if mp := reHTTPPath.FindStringSubmatch(chunk); len(mp) == 2 && strings.TrimSpace(mp[1]) != "" {
			p = mp[1]
		}
		return strings.TrimSpace(h), p
	}
	return "", ""
}

// ---- (A) Forward: object → inbound-PORT within 4KB ----
func portAfterObject(data []byte, sp span) int {
	start := sp.e
	end := clamp(sp.e+4096, 0, len(data))
	sub := data[start:end]
	m := reInboundNear.FindSubmatchIndex(sub)
	if m == nil {
		return 0
	}
	num := 0
	for i := m[2]; i < m[3]; i++ {
		num = num*10 + int(sub[i]-'0')
	}
	return num
}

func scanForward(data []byte) []XUICandidate {
	spans := extractAllJSONObjects(data, 8_000_000)
	seen := map[string]bool{}
	var out []XUICandidate
	nextID := 1

	for _, sp := range spans {
		raw := data[sp.s:sp.e]
		if !reHasAnyStream.Match(raw) {
			continue
		}
		chunk := string(raw)

		port := 0
		if pm := rePortField.FindStringSubmatch(chunk); len(pm) == 2 {
			if n, _ := strconv.Atoi(pm[1]); n > 0 && n <= 65535 {
				port = n
			}
		}
		if port == 0 {
			port = portAfterObject(data, sp)
		}
		if port == 0 {
			continue
		}

		sni := findSNI(chunk)
		host, path := "", ""
		if sni == "" {
			host, path = findHTTPHostPath(chunk)
		}
		if sni == "" && host == "" {
			continue
		}

		var cand XUICandidate
		var key string
		if sni != "" {
			cand = XUICandidate{ID: nextID, Type: "tls", Port: port, SNI: sni}
			key = fmt.Sprintf("tls|%s|%d", strings.ToLower(sni), port)
		} else {
			if path == "" {
				path = "/"
			}
			cand = XUICandidate{ID: nextID, Type: "http", Port: port, Host: host, Path: path}
			key = fmt.Sprintf("http|%s|%s|%d", strings.ToLower(host), path, port)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, cand)
			nextID++
		}
	}
	log.Printf("[xui/text-strict/forward] candidates: %d", len(out))
	return out
}

// ---- (B) Backward: inbound-PORT → nearest object within previous 4KB ----
func nearestObjectBefore(data []byte, idx int) (span, bool) {
	start := clamp(idx-4096, 0, len(data))
	window := data[start:idx]
	sps := extractAllJSONObjects(window, 4096)
	if len(sps) == 0 {
		return span{}, false
	}
	last := sps[len(sps)-1]
	return span{s: start + last.s, e: start + last.e}, true
}

func scanBackward(data []byte) []XUICandidate {
	var out []XUICandidate
	seen := map[string]bool{}
	nextID := 1

	locs := reInboundNear.FindAllSubmatchIndex(data, -1)
	for _, m := range locs {
		port := 0
		for i := m[2]; i < m[3]; i++ {
			port = port*10 + int(data[i]-'0')
		}
		if port <= 0 || port > 65535 {
			continue
		}
		labelPos := m[0]
		sp, ok := nearestObjectBefore(data, labelPos)
		if !ok {
			continue
		}
		raw := data[sp.s:sp.e]
		if !reHasAnyStream.Match(raw) {
			continue
		}
		chunk := string(raw)

		sni := findSNI(chunk)
		host, path := "", ""
		if sni == "" {
			host, path = findHTTPHostPath(chunk)
		}
		if sni == "" && host == "" {
			continue
	}
		var cand XUICandidate
		var key string
		if sni != "" {
			cand = XUICandidate{ID: nextID, Type: "tls", Port: port, SNI: sni}
			key = fmt.Sprintf("tls|%s|%d", strings.ToLower(sni), port)
		} else {
			if path == "" {
				path = "/"
			}
			cand = XUICandidate{ID: nextID, Type: "http", Port: port, Host: host, Path: path}
			key = fmt.Sprintf("http|%s|%s|%d", strings.ToLower(host), path, port)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, cand)
			nextID++
		}
	}

	log.Printf("[xui/text-strict/backward] candidates: %d", len(out))
	return out
}

// ---- Dispatcher: forward ∪ backward ----
func scanXUI() ([]XUICandidate, error) {
	data, err := os.ReadFile(xuiDBPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []XUICandidate{}, nil
		}
		return nil, err
	}
	fwd := scanForward(data)
	bwd := scanBackward(data)

	seen := map[string]bool{}
	var out []XUICandidate
	nextID := 1
	push := func(c XUICandidate) {
		key := ""
		if c.Type == "tls" {
			key = fmt.Sprintf("tls|%s|%d", strings.ToLower(c.SNI), c.Port)
		} else {
			key = fmt.Sprintf("http|%s|%s|%d", strings.ToLower(c.Host), c.Path, c.Port)
		}
		if !seen[key] {
			c.ID = nextID
			out = append(out, c)
			seen[key] = true
			nextID++
		}
	}
	for _, c := range fwd {
		push(c)
	}
	for _, c := range bwd {
		push(c)
	}

	log.Printf("[xui/text-strict] merged: %d", len(out))
	return out, nil
}
