#bin/bash
GREEN="\e[32m"
BOLD_GREEN="\e[1;32m"
YELLOW="\e[33m"
BLUE="\e[34m"
CYAN="\e[36m"
MAGENTA="\e[35m"
WHITE="\e[37m"
RED="\e[31m"
RESET="\e[0m"

mkdir -p /opt/snirouter
cd /opt/snirouter
wget https://github.com/ParsaKSH/sni-panel/releases/download/v1.0.0/sni-panel-linux-amd64
mv sni-panel-linux-amd64 sni-panel
chmod +x sni-panel
tee /etc/systemd/system/sni-panel.service >/dev/null <<'UNIT'
[Unit]
Description=SNI Router Panel (github.com/ParsaKSH)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/snirouter/sni-panel
WorkingDirectory=/opt/snirouter
Restart=always
RestartSec=2

StandardOutput=journal
StandardError=journal
LimitNOFILE=1048576
[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now sni-panel
systemctl restart sni-panel

echo -e "${GREEN}sni-panel installed!-${BLUE}github.com/ParsaKSH/sni-panel"
echo -e "panel: http://<server-ip>:8080/<Panel Path>${RESET}"
cat /etc/snirouter/ADMIN.txt
