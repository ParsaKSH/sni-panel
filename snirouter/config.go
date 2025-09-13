package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

func bootstrapConfig() Config {
	return Config{
		ListenPort443: true,
		DefaultUP:     "127.0.0.1:4433",
		Mappings:      []Mapping{},
		HTTPEnabled:   true,
		DefaultHTTPUP: "127.0.0.1:8081",
		HTTPHosts:     []HTTPHost{},
		AdminPath:     "panel-" + randomToken(6),
	}
}

func loadConfig() (Config, error) {
	var c Config
	b, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c = bootstrapConfig()
			if err := writeAtomic(configPath, mustJSON(c), 0644); err != nil {
				return c, err
			}
			return c, nil
		}
		return c, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		c = bootstrapConfig()
		_ = writeAtomic(configPath, mustJSON(c), 0644)
		return c, nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		ts := time.Now().Format("20060102-150405")
		_ = os.WriteFile("/etc/snirouter/config.broken-"+ts+".json", b, 0600)
		log.Printf("config.json invalid (%v). Recreating defaults", err)
		c = bootstrapConfig()
		_ = writeAtomic(configPath, mustJSON(c), 0644)
		return c, nil
	}
	if strings.TrimSpace(c.AdminPath) == "" {
		c.AdminPath = "panel-" + randomToken(6)
		_ = writeAtomic(configPath, mustJSON(c), 0644)
	}
	return c, nil
}

func saveConfig(c Config) error {
	return writeAtomic(configPath, mustJSON(c), 0644)
}
