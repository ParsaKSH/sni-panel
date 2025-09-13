package main

import "sync"

var (
	configPath = "/etc/snirouter/config.json"
	credsPath  = "/etc/snirouter/ADMIN.txt"
	cachePath  = "/etc/snirouter/cache.json"
	nginxConf  = "/etc/nginx/nginx.conf"
	xuiDBPath  = "/etc/x-ui/x-ui.db"

	configMutex sync.Mutex
)
