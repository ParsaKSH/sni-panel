#bin/bash
sudo -i
mkdir -p /opt/snirouter
cd /opt/snirouter
wget https://github.com/ParsaKSH/sni-panel/releases/download/v1.0.0/sni-panel-linux-amd64
mv sni-panel-linux-amd64 sni-panel

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

echo "sni-panel installed!-github.com/ParsaKSH/sni-panel"
echo "panel: http://<server-ip>:8080/<Panel Path>
cat /etc/snirouter/ADMIN.txt
