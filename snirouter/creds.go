package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func parseAdminFile(b []byte) (adminCred, bool) {
	txt := string(b)

	// 1) JSON: {"user":"...","pass":"..."}
	var j map[string]string
	if json.Unmarshal(b, &j) == nil {
		u := strings.TrimSpace(j["user"])
		p := strings.TrimSpace(j["pass"])
		if u != "" && p != "" {
			return adminCred{User: u, Pass: p}, true
		}
	}

	// 2) Lines: "Username: X\nPassword: Y"
	var u, p string
	for _, ln := range strings.Split(txt, "\n") {
		ln = strings.TrimSpace(ln)
		low := strings.ToLower(ln)
		if strings.HasPrefix(low, "username:") {
			u = strings.TrimSpace(strings.TrimPrefix(ln, "Username:"))
			u = strings.TrimSpace(strings.TrimPrefix(u, "username:"))
		}
		if strings.HasPrefix(low, "password:") {
			p = strings.TrimSpace(strings.TrimPrefix(ln, "Password:"))
			p = strings.TrimSpace(strings.TrimPrefix(p, "password:"))
		}
	}
	if u != "" && p != "" {
		return adminCred{User: u, Pass: p}, true
	}

	// 3) Single-line: "user:pass"
	if strings.Count(txt, ":") == 1 && !strings.Contains(txt, "\n") {
		parts := strings.SplitN(strings.TrimSpace(txt), ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return adminCred{User: strings.TrimSpace(parts[0]), Pass: strings.TrimSpace(parts[1])}, true
		}
	}

	return adminCred{}, false
}

func ensureAdminCreds() (adminCred, error) {
	if st, err := os.Stat(credsPath); err == nil && !st.IsDir() {
		if b, rerr := os.ReadFile(credsPath); rerr == nil {
			if cr, ok := parseAdminFile(b); ok {
				return cr, nil
			}
		}
		ts := time.Now().Format("20060102-150405")
		_ = os.Rename(credsPath, credsPath+".broken-"+ts)
	}

	cr := adminCred{
		User: "admin-" + randomToken(4),
		Pass: randomToken(14),
	}
	content := []byte(fmt.Sprintf("Panel Path: /%%ADMIN_PATH%%\nUsername: %s\nPassword: %s\n", cr.User, cr.Pass))

	if err := os.MkdirAll(filepath.Dir(credsPath), 0700); err != nil {
		return adminCred{}, err
	}
	if err := os.WriteFile(credsPath, content, 0600); err != nil {
		return adminCred{}, err
	}
	return cr, nil
}

func readAdminCreds() (adminCred, error) {
	b, err := os.ReadFile(credsPath)
	if err != nil {
		return adminCred{}, err
	}
	if cr, ok := parseAdminFile(b); ok {
		return cr, nil
	}
	return adminCred{}, fmt.Errorf("invalid %s format", credsPath)
}
