package main

import (
	"os"
	"os/exec"
)

func sudoRun(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func installNginx() error {
	if err := sudoRun("bash", "-lc", "apt-get update"); err != nil {
		return err
	}
	return sudoRun("bash", "-lc", "DEBIAN_FRONTEND=noninteractive apt-get install -y nginx-extras && sudo apt-get install nginx-full libnginx-mod-stream libnginx-mod-stream-ssl-preread")
}
