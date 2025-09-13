package main

import (
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strings"
	"encoding/json"
	"time"
)

func main() {
log.Printf("embed sizes: login=%d, index=%d", len(loginHTML), len(indexHTML))
	mrand.Seed(time.Now().UnixNano())
	if os.Geteuid() != 0 {
		log.Println("please run with sudo user.")
	}
	if err := ensurePaths(); err != nil {
		log.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}


	cr, err := ensureAdminCreds()
	if err != nil {
		log.Fatal(err)
	}
	if b, err := os.ReadFile(credsPath); err == nil {
		nb := strings.ReplaceAll(string(b), "%ADMIN_PATH%", cfg.AdminPath)
		_ = os.WriteFile(credsPath, []byte(nb), 0600)
	}

	base := "/" + strings.Trim(cfg.AdminPath, "/")

	http.HandleFunc(base+"/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		if c, _ := r.Cookie("sni_sess"); c != nil && sessions.valid(c.Value) {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(loginHTML)
	})
	http.HandleFunc(base+"/login/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		_ = r.ParseForm()
		user := r.Form.Get("username")
		pass := r.Form.Get("password")
		if user == "" && pass == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var in struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			user, pass = in.Username, in.Password
		}
		if ac, err := readAdminCreds(); err == nil && user == ac.User && pass == ac.Pass {
			tok := sessions.create(24 * time.Hour)
			setSessionCookie(w, base, tok, 24*time.Hour)
			http.Redirect(w, r, base+"/", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, base+"/login?err=1", http.StatusSeeOther)
	})
	http.HandleFunc(base+"/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, _ := r.Cookie("sni_sess"); c != nil {
			sessions.revoke(c.Value)
		}
		clearSessionCookie(w, base)
		http.Redirect(w, r, base+"/login", http.StatusFound)
	})
	http.HandleFunc(base+"/", requireSession(base, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != base && r.URL.Path != base+"/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	}))

	http.HandleFunc(base+"/api/config", requireSession(base, handleGetConfig))
	http.HandleFunc(base+"/api/default", requireSession(base, handleSetDefault))
	http.HandleFunc(base+"/api/http/default", requireSession(base, handleSetDefaultHTTP))
	http.HandleFunc(base+"/api/stream/mapping", requireSession(base, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleAddMapping(w, r)
			return
		}
		http.Error(w, "method not allowed", 405)
	}))
	http.HandleFunc(base+"/api/stream/mapping/", requireSession(base, makeDeleteStreamHandler(base)))
	http.HandleFunc(base+"/api/http/route", requireSession(base, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleAddHTTPRoute(w, r)
			return
		}
		http.Error(w, "method not allowed", 405)
	}))
	http.HandleFunc(base+"/api/http/route/", requireSession(base, makeDeleteHTTPRouteHandler(base)))
	http.HandleFunc(base+"/api/reload", requireSession(base, handleReload))
	http.HandleFunc(base+"/api/install-nginx", requireSession(base, handleInstallNginx))

	// X-UI APIs
	http.HandleFunc(base+"/api/xui/status", requireSession(base, handleXUIStatus))
	http.HandleFunc(base+"/api/xui/scan", requireSession(base, handleXUIScan))
	http.HandleFunc(base+"/api/xui/apply", requireSession(base, handleXUIApply))

	addr := ":8080"
	log.Printf("Panel at http://<server-ip>%s", base)
	log.Printf("ADMIN creds are stored in %s (not in config.json). User: %s", credsPath, cr.User)
	log.Fatal(http.ListenAndServe(addr, nil))
}
