#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/PEDIHS/Hashshashin.git"
REF="${HASHSHASHIN_REF:-main}"
PREFIX="/opt/hashshashin-src"
CONF_DIR="/etc/hashshashin"
BIN="/usr/local/bin/hashshashin"
SERVICE="/etc/systemd/system/hashshashin.service"

need_root(){ [[ $EUID -eq 0 ]] || { echo "Run as root"; exit 1; }; }
need_cmd(){ command -v "$1" >/dev/null 2>&1; }
install_deps(){
  if need_cmd apt-get; then
    apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y git golang-go iproute2 iptables python3 ca-certificates
  elif need_cmd dnf; then
    dnf install -y git golang iproute iptables python3 ca-certificates
  else
    echo "Unsupported package manager. Install git, Go, iproute2, iptables, python3 manually."; exit 1
  fi
}
default_iface(){ ip -4 route show default | awk 'NR==1{print $5}'; }
default_src(){ ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}'; }
ask(){ local prompt="$1" def="$2" out; read -rp "$prompt [$def]: " out; printf '%s' "${out:-$def}"; }

need_root
install_deps
rm -rf "$PREFIX"
git clone --depth 1 --branch "$REF" "$REPO" "$PREFIX"
cd "$PREFIX"
go test ./...
go build -trimpath -ldflags='-s -w' -o "$BIN" .
install -d -m 700 "$CONF_DIR"
install -m 0644 packaging/hashshashin.service "$SERVICE"

printf '\nHashshashin setup\n1) Iran\n2) Kharej\n'
read -rp "Role [1]: " r; r="${r:-1}"; [[ "$r" == "2" ]] && role="kharej" || role="iran"
printf '\n1) Full tunnel\n2) Upload tunnel / Download direct\n'
read -rp "Mode [2]: " m; m="${m:-2}"; [[ "$m" == "1" ]] && mode="full" || mode="direct-return"
iface="$(ask 'Public interface' "$(default_iface)")"
local_ip="$(ask 'This server public IPv4' "$(default_src)")"
listen_port="$(ask 'Hashshashin UDP transport port' '9000')"
ports_raw="$(ask 'Service ports (comma separated)' '443')"

if [[ "$role" == "iran" ]]; then
  foreign_ip="$(ask 'Kharej public IPv4' '')"
  iran_ip="$local_ip"
  listen="0.0.0.0:0"
  peer="$foreign_ip:$listen_port"
  key="$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64 -w0)"
  tun_cidr="10.77.0.1/30"; tun_peer="10.77.0.2"
  echo
  echo "Shared key (copy this to Kharej):"
  echo "$key"
else
  iran_ip="$(ask 'Iran public IPv4' '')"
  foreign_ip="$local_ip"
  listen="0.0.0.0:$listen_port"
  peer=""
  key="$(ask 'Shared key from Iran' '')"
  tun_cidr="10.77.0.2/30"; tun_peer="10.77.0.1"
fi

ROLE="$role" MODE="$mode" LISTEN="$listen" PEER="$peer" KEY="$key" TUN_CIDR="$tun_cidr" TUN_PEER="$tun_peer" IFACE="$iface" LOCAL_IP="$local_ip" FOREIGN_IP="$foreign_ip" IRAN_IP="$iran_ip" PORTS_RAW="$ports_raw" python3 - <<'PY' > "$CONF_DIR/config.json"
import json, os
ports=[]
for x in os.environ['PORTS_RAW'].split(','):
    x=x.strip()
    if x:
        ports.append({'port':int(x),'protocol':'both'})
c={
 'role':os.environ['ROLE'], 'mode':os.environ['MODE'],
 'transport':{'listen':os.environ['LISTEN'],'peer':os.environ['PEER'],'key':os.environ['KEY']},
 'tun':{'name':'hsh0','local_cidr':os.environ['TUN_CIDR'],'peer_ip':os.environ['TUN_PEER'],'mtu':1300},
 'network':{'public_interface':os.environ['IFACE'],'public_ip':os.environ['LOCAL_IP'],'foreign_public_ip':os.environ['FOREIGN_IP'],'iran_public_ip':os.environ['IRAN_IP']},
 'ports':ports
}
print(json.dumps(c,indent=2))
PY
chmod 600 "$CONF_DIR/config.json"
"$BIN" -check -c "$CONF_DIR/config.json"
systemctl daemon-reload
systemctl enable --now hashshashin
sleep 1
systemctl --no-pager --full status hashshashin || true

echo
echo "Installed. Useful commands:"
echo "  systemctl restart hashshashin"
echo "  journalctl -u hashshashin -f"
echo "  $BIN -check -c $CONF_DIR/config.json"
