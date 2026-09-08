#!/usr/bin/env bash
set -Eeuo pipefail

REPO="https://github.com/PEDIHS/Hashshashin.git"
REF="${HASHSHASHIN_REF:-main}"
SRC="/opt/hashshashin-src"
BIN="/usr/local/bin/hashshashin"
MANAGER="/usr/local/libexec/hashshashin-manager"
CONF_DIR="/etc/hashshashin"
CONF="$CONF_DIR/config.json"
SERVICE="/etc/systemd/system/hashshashin.service"
SYSCTL="/etc/sysctl.d/99-hashshashin.conf"
ACTION="${1:-}"

if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  RESET='\033[0m'; BOLD='\033[1m'; DIM='\033[2m'
  RED='\033[38;5;196m'; GREEN='\033[38;5;82m'; YELLOW='\033[38;5;220m'
  CYAN='\033[38;5;45m'; BLUE='\033[38;5;75m'; PURPLE='\033[38;5;141m'; WHITE='\033[38;5;255m'; GRAY='\033[38;5;245m'
else
  RESET=''; BOLD=''; DIM=''; RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; PURPLE=''; WHITE=''; GRAY=''
fi

clear_screen(){ [[ -t 1 ]] && clear 2>/dev/null || true; }
line(){ printf '%b\n' "${GRAY}────────────────────────────────────────────────────────────────${RESET}"; }
info(){ printf '%b\n' "${CYAN}  →${RESET} $*"; }
ok(){ printf '%b\n' "${GREEN}  ✔${RESET} $*"; }
warn(){ printf '%b\n' "${YELLOW}  !${RESET} $*"; }
die(){ printf '%b\n' "${RED}  ✘ $*${RESET}" >&2; exit 1; }
step(){ echo; printf '%b\n' "${CYAN}${BOLD}[$1/$2] $3${RESET}"; line; }
have(){ command -v "$1" >/dev/null 2>&1; }
ask(){ local p="$1" d="$2" v; read -r -p "  $p [$d]: " v; printf '%s' "${v:-$d}"; }
yesno(){ local p="$1" d="$2" v; read -r -p "  $p [$d]: " v; v="${v:-$d}"; [[ "$v" =~ ^[Yy]$ ]]; }

banner(){
  clear_screen
  printf '%b\n' "${CYAN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}              ${WHITE}${BOLD}H A S H S H A S H I N${RESET}                     ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}             ${GRAY}Professional L3 Tunnel${RESET}                      ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
  printf '%b\n' "              ${PURPLE}حشاشین • نصب و مدیریت تونل${RESET}"
  echo
  printf '%b\n' "${GRAY} Full Tunnel • Direct Return • Encrypted UDP Carrier • Linux TUN${RESET}"
}

need_root(){ [[ ${EUID:-$(id -u)} -eq 0 ]] || die "Installer را با root اجرا کنید."; }
cleanup_on_error(){ local ec=$?; [[ $ec -eq 0 ]] && return; echo; warn "نصب با کد $ec متوقف شد. Config قبلی به‌صورت خودکار حذف نشده است."; }
trap cleanup_on_error EXIT

valid_ipv4(){
  local ip="$1" IFS=. a b c d
  [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
  read -r a b c d <<< "$ip"
  for n in "$a" "$b" "$c" "$d"; do (( n >= 0 && n <= 255 )) || return 1; done
}
valid_port(){ [[ "$1" =~ ^[0-9]+$ ]] && (( "$1" >= 1 && "$1" <= 65535 )); }
default_iface(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}'; }
default_gateway(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="via"){print $(i+1);exit}}'; }
default_src(){ ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}'; }
is_local_ip(){ ip -4 addr show dev "$1" | grep -qw "$2"; }

uninstall(){
  banner; need_root
  step 1 3 "Stopping tunnel and cleaning network state"
  if [[ -x "$BIN" && -f "$CONF" ]]; then
    systemctl stop hashshashin >/dev/null 2>&1 || true
    "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true
  fi
  ok "Tunnel stopped and Hashshashin rules cleaned."

  step 2 3 "Removing runtime files"
  systemctl disable --now hashshashin >/dev/null 2>&1 || true
  rm -f "$SERVICE" "$SYSCTL" "$BIN" "$MANAGER"
  rm -rf "$SRC"
  systemctl daemon-reload >/dev/null 2>&1 || true
  ok "Binary, manager, service and source cache removed."

  step 3 3 "Finished"
  printf '%b\n' "${GREEN}${BOLD}  Hashshashin حذف شد.${RESET}"
  printf '%b\n' "${GRAY}  Config برای جلوگیری از حذف ناخواسته کلید در ${CONF_DIR} نگه داشته شد.${RESET}"
  exit 0
}

[[ "$ACTION" == "--uninstall" ]] && uninstall
need_root
banner

existing_menu(){
  [[ -x "$BIN" && -f "$CONF" ]] || return 0
  printf '%b\n' "${GREEN}${BOLD}  Hashshashin روی این سرور نصب است.${RESET}"
  if systemctl is-active --quiet hashshashin; then printf '%b\n' "  Status: ${GREEN}● ACTIVE${RESET}"; else printf '%b\n' "  Status: ${YELLOW}● $(systemctl is-active hashshashin 2>/dev/null || echo inactive)${RESET}"; fi
  printf '  Version: %s\n' "$($BIN -version 2>/dev/null || echo unknown)"
  line
  printf '%b\n' "  ${CYAN}1)${RESET} Open Management Panel"
  printf '%b\n' "  ${BLUE}2)${RESET} Update / Repair        ${GRAY}(Config حفظ می‌شود)${RESET}"
  printf '%b\n' "  ${PURPLE}3)${RESET} Reconfigure            ${GRAY}(Wizard تنظیمات دوباره اجرا می‌شود)${RESET}"
  printf '%b\n' "  ${RED}4)${RESET} Uninstall"
  printf '%b\n' "  ${GRAY}0) Exit${RESET}"
  echo
  local c
  read -r -p "  انتخاب شما [0-4]: " c
  case "$c" in
    1)
      if [[ -x "$MANAGER" ]]; then exec "$MANAGER"; fi
      warn "Management Panel هنوز نصب نیست؛ ابتدا Update / Repair اجرا می‌شود."
      ACTION="--update"
      ;;
    2) ACTION="--update";;
    3) ACTION="--reconfigure";;
    4) uninstall;;
    0) exit 0;;
    *) die "انتخاب نامعتبر.";;
  esac
}

if [[ -z "$ACTION" ]]; then existing_menu; fi
case "$ACTION" in ""|--update|--reconfigure) ;; *) die "Unknown installer option: $ACTION";; esac

install_deps(){
  if have apt-get; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y >/dev/null
    apt-get install -y ca-certificates curl git golang-go iproute2 iptables kmod procps >/dev/null
  elif have dnf; then
    dnf install -y ca-certificates curl git golang iproute iptables kmod procps-ng >/dev/null
  else
    die "Installer رسمی فعلاً سیستم‌های apt و dnf را پشتیبانی می‌کند."
  fi
  for c in git go ip iptables systemctl sysctl; do have "$c" || die "Missing required command: $c"; done
}

step 1 7 "System check & dependencies"
printf '  OS        : %s\n' "$(. /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-Linux}")"
printf '  Kernel    : %s\n' "$(uname -r)"
printf '  Arch      : %s\n' "$(uname -m)"
info "Installing/checking required packages..."
install_deps
modprobe tun >/dev/null 2>&1 || true
[[ -c /dev/net/tun ]] || die "/dev/net/tun در دسترس نیست. TUN/TAP را در پنل VPS فعال کنید."
ok "Linux networking prerequisites are available."

step 2 7 "Download, test & build"
info "Fetching Hashshashin ref: $REF"
rm -rf "$SRC"
git clone --quiet --depth 1 --branch "$REF" "$REPO" "$SRC"
cd "$SRC"
info "Running source tests..."
go test ./...
info "Building optimized binary..."
go build -trimpath -ldflags="-s -w -X main.buildRef=$REF" -o "$BIN" .
chmod 0755 "$BIN"
install -d -m 0755 "$(dirname "$MANAGER")"
install -m 0755 packaging/hashshashin-manager.sh "$MANAGER"
install -m 0644 packaging/hashshashin.service "$SERVICE"
install -d -m 0700 "$CONF_DIR"
printf '%b\n' "  Binary    : ${WHITE}$($BIN -version)${RESET}"
printf '%b\n' "  Manager   : ${WHITE}${MANAGER}${RESET}"
ok "Core and management panel installed."

cat > "$SYSCTL" <<'SYS'
# Hashshashin requires IPv4 forwarding for transparent L3 forwarding.
net.ipv4.ip_forward=1
SYS
sysctl --system >/dev/null || true
if have timedatectl; then timedatectl set-ntp true >/dev/null 2>&1 || true; fi

if [[ "$ACTION" == "--update" ]]; then
  step 3 7 "Validate existing configuration"
  [[ -f "$CONF" ]] || die "Config پیدا نشد. Installer را با --reconfigure اجرا کنید."
  "$BIN" -check -c "$CONF"
  ok "Existing config is valid and was preserved."

  step 4 7 "Reload service"
  systemctl daemon-reload
  systemctl enable hashshashin >/dev/null 2>&1 || true
  systemctl restart hashshashin
  sleep 2
  systemctl is-active --quiet hashshashin || { journalctl -u hashshashin -n 50 --no-pager || true; die "Service after update failed to start."; }
  ok "Hashshashin restarted successfully."

  step 5 7 "Local health check"
  ip link show hsh0 >/dev/null 2>&1 && ok "TUN interface hsh0 is up." || warn "hsh0 هنوز دیده نمی‌شود؛ Logs را بررسی کنید."
  [[ "$(sysctl -n net.ipv4.ip_forward)" == "1" ]] && ok "IPv4 forwarding enabled." || warn "IPv4 forwarding is disabled."

  step 6 7 "Management command"
  printf '%b\n' "  از این به بعد برای مدیریت فقط اجرا کنید:"
  printf '%b\n' "\n      ${CYAN}${BOLD}hashshashin${RESET}\n"

  step 7 7 "Update complete"
  printf '%b\n' "${GREEN}${BOLD}  ✔ Hashshashin با حفظ Config آپدیت شد.${RESET}"
  exit 0
fi

step 3 7 "Select server role"
printf '%b\n' "  ${CYAN}1) IRAN / Entry${RESET}      ${GRAY}کاربر به این سرور متصل می‌شود${RESET}"
printf '%b\n' "  ${PURPLE}2) KHAREJ / Exit${RESET}    ${GRAY}سرویس اصلی روی این سرور است${RESET}"
role_choice="$(ask 'Role' '1')"
[[ "$role_choice" == "2" ]] && role="kharej" || role="iran"

step 4 7 "Select tunnel mode"
printf '%b\n' "  ${CYAN}1) Full Tunnel${RESET}        ${GRAY}Upload + Download داخل تونل${RESET}"
printf '%b\n' "  ${GREEN}2) Direct Return${RESET}      ${GRAY}Upload داخل تونل / Download مستقیم Kharej → Iran${RESET}"
mode_choice="$(ask 'Mode' '2')"
[[ "$mode_choice" == "1" ]] && mode="full" || mode="direct-return"

step 5 7 "Network & service configuration"
iface_default="$(default_iface)"; [[ -n "$iface_default" ]] || die "Default public interface تشخیص داده نشد."
ip_default="$(default_src)"; gateway_default="$(default_gateway)"
iface="$(ask 'Public interface' "$iface_default")"
local_ip="$(ask 'This server public IPv4' "$ip_default")"
valid_ipv4 "$local_ip" || die "Invalid IPv4: $local_ip"
gateway="$(ask 'Public gateway' "$gateway_default")"
transport_port="$(ask 'Hashshashin UDP carrier port' '9000')"
valid_port "$transport_port" || die "Invalid carrier port."
ports_raw="$(ask 'Service ports (comma separated)' '443')"
mtu="$(ask 'Tunnel MTU' '1320')"
[[ "$mtu" =~ ^[0-9]+$ ]] && (( mtu >= 900 && mtu <= 1400 )) || die "MTU باید بین 900 و 1400 باشد."

if [[ "$mode" == "direct-return" && "$role" == "iran" ]] && ! is_local_ip "$iface" "$local_ip"; then
  die "Direct Return نیاز دارد Public IPv4 ایران واقعاً روی $iface assign شده باشد؛ NAT/CGNAT-only پشتیبانی نمی‌شود."
fi

ports_json=""
IFS=',' read -r -a parr <<< "$ports_raw"
for raw in "${parr[@]}"; do
  p="${raw//[[:space:]]/}"
  [[ -z "$p" ]] && continue
  valid_port "$p" || die "Invalid service port: $p"
  [[ "$p" == "$transport_port" ]] && die "Carrier UDP port نباید با service port یکسان باشد: $p"
  [[ -n "$ports_json" ]] && ports_json+=","
  ports_json+="{\"port\":$p,\"protocol\":\"both\"}"
done
[[ -n "$ports_json" ]] || die "حداقل یک service port لازم است."

step 6 7 "Peer & security"
if [[ "$role" == "iran" ]]; then
  foreign_ip="$(ask 'Kharej public IPv4' '')"
  valid_ipv4 "$foreign_ip" || die "Invalid Kharej IPv4."
  iran_ip="$local_ip"; listen="0.0.0.0:$transport_port"; peer="$foreign_ip:$transport_port"
  key="$($BIN -keygen)"; tun_cidr="10.77.0.1/30"; tun_peer="10.77.0.2"; lock=false
  echo
  printf '%b\n' "${YELLOW}${BOLD}┌──────────────── Shared Key ────────────────┐${RESET}"
  printf '%b\n' "${WHITE}${BOLD}  $key${RESET}"
  printf '%b\n' "${YELLOW}${BOLD}└─────────────────────────────────────────────┘${RESET}"
  printf '%b\n' "${GRAY}  این کلید را دقیقاً در Installer سرور Kharej وارد کنید.${RESET}"
  read -r -p "  بعد از ذخیره کلید Enter را بزنید... " _
else
  iran_ip="$(ask 'Iran public IPv4' '')"
  valid_ipv4 "$iran_ip" || die "Invalid Iran IPv4."
  foreign_ip="$local_ip"; listen="0.0.0.0:$transport_port"; peer=""
  key="$(ask 'Shared Key generated on Iran' '')"; [[ -n "$key" ]] || die "Shared Key الزامی است."
  tun_cidr="10.77.0.2/30"; tun_peer="10.77.0.1"
  if yesno 'Block direct public access to service ports on Kharej?' 'Y'; then lock=true; else lock=false; fi
fi

if [[ -f "$CONF" ]]; then
  backup="${CONF}.bak.$(date +%Y%m%d-%H%M%S)"
  cp -a "$CONF" "$backup"
  info "Previous config backup: $backup"
  systemctl stop hashshashin >/dev/null 2>&1 || true
  "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true
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
ok "Configuration validated and saved."

step 7 7 "Start & verify"
systemctl daemon-reload
systemctl enable hashshashin >/dev/null
systemctl restart hashshashin
sleep 2

if ! systemctl is-active --quiet hashshashin; then
  systemctl --no-pager --full status hashshashin || true
  journalctl -u hashshashin -n 60 --no-pager || true
  die "Service failed to start."
fi

ok "Systemd service is ACTIVE."
ip link show hsh0 >/dev/null 2>&1 && ok "TUN interface hsh0 created." || warn "hsh0 هنوز دیده نشد؛ handshake/logs را بررسی کنید."
"$BIN" -check -c "$CONF" >/dev/null && ok "Configuration check passed."

echo
printf '%b\n' "${GREEN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
printf '%b\n' "${GREEN}${BOLD}║${RESET}                 ${WHITE}${BOLD}INSTALLATION COMPLETE${RESET}                    ${GREEN}${BOLD}║${RESET}"
printf '%b\n' "${GREEN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
printf '  Role          : %s\n' "$role"
printf '  Mode          : %s\n' "$mode"
printf '  Public IP     : %s\n' "$local_ip"
printf '  Carrier       : UDP/%s\n' "$transport_port"
printf '  Service Ports : %s\n' "$ports_raw"
printf '  TUN           : %s\n' "$tun_cidr"
echo
printf '%b\n' "  برای مدیریت از این به بعد فقط اجرا کنید:"
printf '%b\n' "\n      ${CYAN}${BOLD}hashshashin${RESET}\n"

if [[ "$role" == "iran" ]]; then
  printf '%b\n' "${YELLOW}${BOLD}  NEXT STEP:${RESET} همین installer را روی Kharej اجرا کنید و همان Mode/Port/Shared Key را وارد کنید."
else
  printf '%b\n' "${GREEN}${BOLD}  NEXT STEP:${RESET} روی هر دو سرور `hashshashin` → Health Check را اجرا کنید و سپس مسیر را تست کنید."
fi

echo
if yesno 'Open Hashshashin Manager now?' 'Y'; then exec "$MANAGER"; fi
