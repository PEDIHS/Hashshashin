#!/usr/bin/env bash
set -Eeuo pipefail

REPO="https://github.com/PEDIHS/Hashshashin.git"
REF="${HASHSHASHIN_REF:-main}"
SRC="/opt/hashshashin-src"
BIN="/usr/local/bin/hashshashin"
CONF_DIR="/etc/hashshashin"
CONF="$CONF_DIR/config.json"
SERVICE="/etc/systemd/system/hashshashin.service"
SYSCTL="/etc/sysctl.d/99-hashshashin.conf"

C_RESET='\033[0m'; C_RED='\033[0;31m'; C_GREEN='\033[0;32m'; C_YELLOW='\033[0;33m'; C_CYAN='\033[0;36m'
info(){ echo -e "${C_CYAN}[Hashshashin]${C_RESET} $*"; }
ok(){ echo -e "${C_GREEN}[OK]${C_RESET} $*"; }
warn(){ echo -e "${C_YELLOW}[WARN]${C_RESET} $*"; }
die(){ echo -e "${C_RED}[ERROR]${C_RESET} $*" >&2; exit 1; }
need_root(){ [[ ${EUID:-$(id -u)} -eq 0 ]] || die "Run the installer as root."; }
have(){ command -v "$1" >/dev/null 2>&1; }
ask(){ local p="$1" d="$2" v; read -r -p "$p [$d]: " v; printf '%s' "${v:-$d}"; }
yesno(){ local p="$1" d="$2" v; read -r -p "$p [$d]: " v; v="${v:-$d}"; [[ "$v" =~ ^[Yy]$ ]]; }

cleanup_on_error(){ local ec=$?; [[ $ec -eq 0 ]] && return; warn "Installation stopped with error code $ec. Existing systemd service was not deleted."; }
trap cleanup_on_error EXIT

install_deps(){
  info "Installing required packages..."
  if have apt-get; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y ca-certificates curl git golang-go iproute2 iptables kmod
  elif have dnf; then
    dnf install -y ca-certificates curl git golang iproute iptables kmod
  else
    die "Supported package manager not found. Official installer currently supports apt and dnf based Linux systems."
  fi
  for c in git go ip iptables systemctl; do have "$c" || die "Missing required command: $c"; done
}

default_iface(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}'; }
default_gateway(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="via"){print $(i+1);exit}}'; }
default_src(){ ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}'; }
is_local_ip(){ ip -4 addr show dev "$1" | grep -qw "$2"; }
valid_ipv4(){ [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; }
valid_port(){ [[ "$1" =~ ^[0-9]+$ ]] && (( "$1" >= 1 && "$1" <= 65535 )); }

uninstall(){
  need_root
  info "Stopping Hashshashin..."
  if [[ -x "$BIN" && -f "$CONF" ]]; then "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true; fi
  systemctl disable --now hashshashin >/dev/null 2>&1 || true
  rm -f "$SERVICE" "$SYSCTL" "$BIN"
  rm -rf "$SRC"
  systemctl daemon-reload || true
  ok "Hashshashin removed. Configuration is kept in $CONF_DIR for safety."
  exit 0
}

[[ "${1:-}" == "--uninstall" ]] && uninstall
need_root
install_deps

modprobe tun || true
[[ -c /dev/net/tun ]] || die "/dev/net/tun is unavailable. Enable TUN/TAP in the VPS/provider panel first."

info "Downloading source ref '$REF'..."
rm -rf "$SRC"
git clone --depth 1 --branch "$REF" "$REPO" "$SRC"
cd "$SRC"
info "Running source tests..."
go test ./...
info "Building Hashshashin..."
go build -trimpath -ldflags="-s -w -X main.buildRef=$REF" -o "$BIN" .
chmod 0755 "$BIN"
"$BIN" -version

install -d -m 0700 "$CONF_DIR"
install -m 0644 packaging/hashshashin.service "$SERVICE"
cat > "$SYSCTL" <<'SYS'
# Hashshashin requires IPv4 forwarding for transparent L3 forwarding.
net.ipv4.ip_forward=1
SYS
sysctl --system >/dev/null || true
if have timedatectl; then timedatectl set-ntp true >/dev/null 2>&1 || true; fi

iface_default="$(default_iface)"; [[ -n "$iface_default" ]] || die "Could not detect the default public interface."
ip_default="$(default_src)"; gateway_default="$(default_gateway)"

echo
printf '%b\n' "${C_CYAN}Hashshashin v0.1 setup wizard${C_RESET}"
echo "1) Iran (entry server)"
echo "2) Kharej (service server)"
role_choice="$(ask 'Role' '1')"
[[ "$role_choice" == "2" ]] && role="kharej" || role="iran"

echo
echo "1) Full tunnel        (upload + download through Hashshashin)"
echo "2) Direct return      (upload tunnel + download direct Kharej -> Iran)"
mode_choice="$(ask 'Mode' '2')"
[[ "$mode_choice" == "1" ]] && mode="full" || mode="direct-return"

iface="$(ask 'Public interface' "$iface_default")"
local_ip="$(ask 'This server public IPv4 (must be assigned to this server for direct-return)' "$ip_default")"
valid_ipv4 "$local_ip" || die "Invalid IPv4: $local_ip"
gateway="$(ask 'Public gateway' "$gateway_default")"
transport_port="$(ask 'Hashshashin UDP carrier port' '9000')"
valid_port "$transport_port" || die "Invalid carrier port."
ports_raw="$(ask 'Service ports, comma-separated' '443')"
mtu="$(ask 'Tunnel MTU' '1320')"
[[ "$mtu" =~ ^[0-9]+$ ]] && (( mtu >= 900 && mtu <= 1400 )) || die "MTU must be between 900 and 1400."

if [[ "$mode" == "direct-return" && "$role" == "iran" ]] && ! is_local_ip "$iface" "$local_ip"; then
  die "Direct-return requires the Iran public IPv4 to be assigned locally on $iface. NAT/CGNAT-only public IPs are not supported in v0.1."
fi

ports_json=""
IFS=',' read -r -a parr <<< "$ports_raw"
for raw in "${parr[@]}"; do
  p="${raw//[[:space:]]/}"
  [[ -z "$p" ]] && continue
  valid_port "$p" || die "Invalid service port: $p"
  [[ -n "$ports_json" ]] && ports_json+=","
  ports_json+="{\"port\":$p,\"protocol\":\"both\"}"
done
[[ -n "$ports_json" ]] || die "At least one service port is required."

if [[ "$role" == "iran" ]]; then
  foreign_ip="$(ask 'Kharej public IPv4' '')"
  valid_ipv4 "$foreign_ip" || die "Invalid Kharej IPv4."
  iran_ip="$local_ip"
  listen="0.0.0.0:$transport_port"
  peer="$foreign_ip:$transport_port"
  key="$($BIN -keygen)"
  tun_cidr="10.77.0.1/30"; tun_peer="10.77.0.2"
  lock=false
  echo
  echo -e "${C_YELLOW}Shared key — copy this exact value to the Kharej installer:${C_RESET}"
  echo "$key"
  echo
  read -r -p "Press Enter after saving the key..." _
else
  iran_ip="$(ask 'Iran public IPv4' '')"
  valid_ipv4 "$iran_ip" || die "Invalid Iran IPv4."
  foreign_ip="$local_ip"
  listen="0.0.0.0:$transport_port"
  peer=""
  key="$(ask 'Shared key generated on Iran' '')"
  [[ -n "$key" ]] || die "Shared key is required."
  tun_cidr="10.77.0.2/30"; tun_peer="10.77.0.1"
  if yesno 'Block direct public access to configured service ports on Kharej?' 'Y'; then lock=true; else lock=false; fi
fi

cat > "$CONF.tmp" <<JSON
{
  "role": "$role",
  "mode": "$mode",
  "transport": {
    "listen": "$listen",
    "peer": "$peer",
    "key": "$key",
    "keepalive_seconds": 5,
    "session_timeout_seconds": 25,
    "rekey_minutes": 30
  },
  "tun": {
    "name": "hsh0",
    "local_cidr": "$tun_cidr",
    "peer_ip": "$tun_peer",
    "mtu": $mtu
  },
  "network": {
    "public_interface": "$iface",
    "public_ip": "$local_ip",
    "public_gateway": "$gateway",
    "foreign_public_ip": "$foreign_ip",
    "iran_public_ip": "$iran_ip",
    "lock_service_ports": $lock
  },
  "ports": [$ports_json]
}
JSON
chmod 0600 "$CONF.tmp"
"$BIN" -check -c "$CONF.tmp"
mv "$CONF.tmp" "$CONF"
chmod 0600 "$CONF"

systemctl daemon-reload
systemctl enable hashshashin >/dev/null
systemctl restart hashshashin
sleep 2

if systemctl is-active --quiet hashshashin; then
  ok "Hashshashin is running."
else
  systemctl --no-pager --full status hashshashin || true
  journalctl -u hashshashin -n 50 --no-pager || true
  die "Service failed to start. See logs above."
fi

echo
ok "Installation completed."
echo "Config: $CONF"
echo "Logs:   journalctl -u hashshashin -f"
echo "Check:  hashshashin -check -c $CONF"
echo "Status: systemctl status hashshashin"
echo "Remove: bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall"
if [[ "$role" == "iran" ]]; then
  echo
  info "Install Kharej with the SAME mode, carrier port, service ports and shared key. Iran will keep retrying the handshake until Kharej is online."
fi
