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
need_root(){ [[ ${EUID:-$(id -u)} -eq 0 ]] || die "Installer را با root اجرا کنید."; }
valid_port(){ [[ "$1" =~ ^[0-9]+$ ]] && (( "$1" >= 1 && "$1" <= 65535 )); }
valid_ipv4(){ local IFS=. a b c d; [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1; read -r a b c d <<< "$1"; for n in "$a" "$b" "$c" "$d"; do (( n >= 0 && n <= 255 )) || return 1; done; }
default_iface(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}'; }
default_gateway(){ ip -4 route show default | awk 'NR==1{for(i=1;i<=NF;i++)if($i=="via"){print $(i+1);exit}}'; }
default_src(){ ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}'; }
is_local_ip(){ ip -4 addr show dev "$1" | grep -qw "$2"; }
min(){ (( $1 < $2 )) && echo "$1" || echo "$2"; }

banner(){
  clear_screen
  printf '%b\n' "${CYAN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}              ${WHITE}${BOLD}H A S H S H A S H I N${RESET}                     ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}        ${GRAY}Adaptive L3 Tunnel • Smart Return${RESET}                 ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
  printf '%b\n' "              ${PURPLE}حشاشین • نصب و مدیریت حرفه‌ای${RESET}"
  echo
  printf '%b\n' "${GRAY} Full Tunnel • Direct Return • Auto Failover • Multi-Queue TUN • UDP • TCP • KCP/FEC${RESET}"
}

cleanup_on_error(){ local ec=$?; [[ $ec -eq 0 ]] && return; echo; warn "عملیات با کد $ec متوقف شد؛ Config قبلی خودکار حذف نشده است."; }
trap cleanup_on_error EXIT

install_deps(){
  if have apt-get; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y >/dev/null
    apt-get install -y ca-certificates curl git golang-go iproute2 iptables iperf3 kmod procps >/dev/null
  elif have dnf; then
    dnf install -y ca-certificates curl git golang iproute iptables iperf3 kmod procps-ng >/dev/null
  else
    die "Installer رسمی فعلاً apt و dnf را پشتیبانی می‌کند."
  fi
  for c in git go ip iptables systemctl sysctl tc; do have "$c" || die "Missing command: $c"; done
}

build_install(){
  rm -rf "$SRC"
  git clone --quiet --depth 1 --branch "$REF" "$REPO" "$SRC"
  cd "$SRC"
  info "Running Go tests..."
  go test ./...
  info "Building optimized binary..."
  go build -trimpath -ldflags="-s -w -X main.buildRef=$REF" -o "${BIN}.new" .
  chmod 0755 "${BIN}.new"
  mv "${BIN}.new" "$BIN"
  install -d -m 0755 "$(dirname "$MANAGER")"
  install -m 0755 packaging/hashshashin-manager.sh "$MANAGER"
  install -m 0644 packaging/hashshashin.service "$SERVICE"
  install -d -m 0700 "$CONF_DIR"
  if [[ -f tools/hsh-bench.sh ]]; then install -m 0755 tools/hsh-bench.sh /usr/local/bin/hsh-bench; fi
}

uninstall(){
  banner; need_root
  step 1 3 "Stop & cleanup"
  if [[ -x "$BIN" && -f "$CONF" ]]; then systemctl stop hashshashin >/dev/null 2>&1 || true; "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true; fi
  ok "Network state cleaned."
  step 2 3 "Remove runtime"
  systemctl disable --now hashshashin >/dev/null 2>&1 || true
  rm -f "$SERVICE" "$SYSCTL" "$BIN" "$MANAGER" /usr/local/bin/hsh-bench; rm -rf "$SRC"; systemctl daemon-reload >/dev/null 2>&1 || true
  ok "Runtime files removed."
  step 3 3 "Finished"
  printf '%b\n' "${GREEN}${BOLD}  Hashshashin حذف شد.${RESET} ${GRAY}Config در ${CONF_DIR} نگه داشته شد.${RESET}"
  exit 0
}

[[ "$ACTION" == "--uninstall" ]] && uninstall
need_root; banner

if [[ -z "$ACTION" && -x "$BIN" && -f "$CONF" ]]; then
  printf '%b\n' "${GREEN}${BOLD}  Hashshashin از قبل نصب است.${RESET}"; line
  printf '%b\n' "  ${CYAN}1)${RESET} Management Panel"
  printf '%b\n' "  ${BLUE}2)${RESET} Update / Repair ${GRAY}(Config حفظ می‌شود)${RESET}"
  printf '%b\n' "  ${PURPLE}3)${RESET} Reconfigure"
  printf '%b\n' "  ${RED}4)${RESET} Uninstall"
  printf '%b\n' "  ${GRAY}0) Exit${RESET}"
  read -r -p "  انتخاب [0-4]: " c
  case "$c" in 1) exec "$MANAGER";; 2) ACTION="--update";; 3) ACTION="--reconfigure";; 4) uninstall;; *) exit 0;; esac
fi
case "$ACTION" in ""|--update|--reconfigure) ;; *) die "Unknown option: $ACTION";; esac

step 1 8 "System check & dependencies"
printf '  OS       : %s\n' "$(. /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-Linux}")"
printf '  Kernel   : %s\n' "$(uname -r)"
printf '  Arch     : %s\n' "$(uname -m)"
printf '  CPU      : %s logical cores\n' "$(nproc 2>/dev/null || echo 1)"
install_deps
modprobe tun >/dev/null 2>&1 || true
[[ -c /dev/net/tun ]] || die "/dev/net/tun در دسترس نیست؛ TUN/TAP را در پنل VPS فعال کنید."
ok "System prerequisites ready."

step 2 8 "Download, test & build"
info "Fetching ref: $REF"
build_install
printf '  Binary   : %s\n' "$($BIN -version)"
ok "Core and Manager installed."

cat > "$SYSCTL" <<'SYS'
# Hashshashin forwarding and conservative high-throughput socket ceilings.
net.ipv4.ip_forward=1
net.core.rmem_max=16777216
net.core.wmem_max=16777216
net.core.netdev_max_backlog=16384
SYS
sysctl --system >/dev/null || true
if have timedatectl; then timedatectl set-ntp true >/dev/null 2>&1 || true; fi

if [[ "$ACTION" == "--update" ]]; then
  step 3 8 "Validate existing configuration"
  [[ -f "$CONF" ]] || die "Config پیدا نشد؛ --reconfigure را اجرا کنید."
  "$BIN" -check -c "$CONF"; ok "Config preserved and valid. Missing performance fields use the Turbo defaults automatically."
  step 4 8 "Restart service"
  systemctl daemon-reload; systemctl enable hashshashin >/dev/null 2>&1 || true; systemctl restart hashshashin; sleep 2
  systemctl is-active --quiet hashshashin || { journalctl -u hashshashin -n 60 --no-pager || true; die "Service failed after update."; }
  ok "Service ACTIVE."
  step 5 8 "Local health"
  ip link show hsh0 >/dev/null 2>&1 && ok "hsh0 is up." || warn "hsh0 not visible yet."
  [[ "$(sysctl -n net.ipv4.ip_forward)" == 1 ]] && ok "IPv4 forwarding enabled."
  ip -details link show hsh0 2>/dev/null | grep -E 'qlen|mtu' || true
  step 6 8 "Feature compatibility"
  "$BIN" -summary -c "$CONF" | grep -E '^(mode|transport|smart_return|performance_profile|tun_queues|receive_workers|tx_queue_len|qdisc|socket_buffer)=' || true
  step 7 8 "Management"
  printf '%b\n' "  اجرا کنید: ${CYAN}${BOLD}hashshashin${RESET}"
  printf '%b\n' "  Benchmark: ${CYAN}${BOLD}hsh-bench${RESET}"
  step 8 8 "Update complete"; ok "Hashshashin updated with existing Config."; exit 0
fi

step 3 8 "Select server role"
printf '%b\n' "  ${CYAN}1) IRAN / Entry${RESET}   ${GRAY}Client endpoint${RESET}"
printf '%b\n' "  ${PURPLE}2) KHAREJ / Exit${RESET} ${GRAY}Service endpoint${RESET}"
role_choice="$(ask 'Role' '1')"; [[ "$role_choice" == 2 ]] && role="kharej" || role="iran"

step 4 8 "Tunnel mode & Smart Return"
printf '%b\n' "  ${CYAN}1) Full Tunnel${RESET}    ${GRAY}Upload + Download through tunnel${RESET}"
printf '%b\n' "  ${GREEN}2) Direct Return${RESET}  ${GRAY}Upload tunnel / Download direct${RESET}"
mode_choice="$(ask 'Mode' '2')"; [[ "$mode_choice" == 1 ]] && mode="full" || mode="direct-return"
smart=false; probe_port=9001; probe_interval=5; probe_timeout=2; fail_threshold=3; recover_threshold=3
if [[ "$mode" == "direct-return" ]]; then
  echo
  printf '%b\n' "  ${GREEN}${BOLD}Smart Return${RESET} ${GRAY}اگر مسیر مستقیم دانلود خراب شود، دانلود خودکار به تونل برمی‌گردد.${RESET}"
  if yesno 'Enable automatic Direct → Tunnel → Direct failover?' 'Y'; then
    smart=true
    probe_port="$(ask 'Direct-path health probe UDP port' '9001')"; valid_port "$probe_port" || die "Invalid probe port."
    printf '%b\n' "  ${GRAY}Default: هر 5 ثانیه probe؛ بعد از 3 خطا fallback و بعد از 3 موفقیت recovery.${RESET}"
    if yesno 'Use default Smart Return sensitivity?' 'Y'; then :; else
      probe_interval="$(ask 'Probe interval seconds' '5')"
      probe_timeout="$(ask 'Probe timeout seconds' '2')"
      fail_threshold="$(ask 'Failures before fallback' '3')"
      recover_threshold="$(ask 'Successes before direct recovery' '3')"
    fi
  fi
fi

step 5 8 "Carrier & performance profile"
printf '%b\n' "  ${CYAN}1) UDP${RESET}        ${GRAY}Lowest overhead; preferred on a clean path${RESET}"
printf '%b\n' "  ${BLUE}2) TCP${RESET}        ${GRAY}For UDP-restricted networks${RESET}"
printf '%b\n' "  ${PURPLE}3) KCP/FEC${RESET}    ${GRAY}ARQ + optional loss recovery${RESET}"
transport_choice="$(ask 'Carrier' '1')"
case "$transport_choice" in 2) transport="tcp";; 3) transport="kcp";; *) transport="udp";; esac
kcp_data=0; kcp_parity=0; kcp_nodelay=1; kcp_interval=20; kcp_resend=2; kcp_nc=1; kcp_sndwnd=512; kcp_rcvwnd=512; kcp_mtu=1200; kcp_buffer=4194304
if [[ "$transport" == kcp ]]; then
  printf '%b\n' "  ${GREEN}1) Balanced FEC 10/3${RESET}  ${CYAN}2) No FEC${RESET}  ${YELLOW}3) Strong FEC 10/5${RESET}"
  kp="$(ask 'KCP profile' '1')"; case "$kp" in 2) ;; 3) kcp_data=10; kcp_parity=5;; *) kcp_data=10; kcp_parity=3;; esac
fi

echo
printf '%b\n' "  ${WHITE}${BOLD}Performance Profile${RESET}"
printf '%b\n' "  ${GREEN}1) Turbo${RESET}       ${GRAY}recommended: multi-queue + qlen 4096 + fq_codel${RESET}"
printf '%b\n' "  ${CYAN}2) Balance${RESET}     ${GRAY}small/shared VPS; lower memory/CPU${RESET}"
printf '%b\n' "  ${PURPLE}3) Throughput${RESET}  ${GRAY}bulk bandwidth; deeper queue/buffers${RESET}"
printf '%b\n' "  ${YELLOW}4) Custom${RESET}      ${GRAY}manual queue/worker/buffer values${RESET}"
perf_choice="$(ask 'Performance' '1')"
cpus="$(nproc 2>/dev/null || echo 1)"
qdisc="fq_codel"; stats_interval=10
case "$perf_choice" in
  2) perf_profile="balance"; tun_queues="$(min "$cpus" 2)"; recv_workers="$(min "$cpus" 2)"; tx_queue_len=2048; socket_buffer=4194304;;
  3) perf_profile="throughput"; tun_queues="$(min "$cpus" 8)"; recv_workers="$(min "$cpus" 8)"; tx_queue_len=8192; socket_buffer=16777216;;
  4)
    perf_profile="custom"
    tun_queues="$(ask 'TUN queues (1-16)' "$(min "$cpus" 4)")"
    recv_workers="$(ask 'UDP receive workers (1-16)' "$(min "$cpus" 4)")"
    tx_queue_len="$(ask 'TUN txqueuelen' '4096')"
    socket_buffer="$(ask 'Carrier socket buffer bytes' '8388608')"
    qdisc="$(ask 'Qdisc (fq_codel/fq/none)' 'fq_codel')";;
  *) perf_profile="turbo"; tun_queues="$(min "$cpus" 4)"; recv_workers="$(min "$cpus" 4)"; tx_queue_len=4096; socket_buffer=8388608;;
esac

step 6 8 "Network & service configuration"
iface_default="$(default_iface)"; [[ -n "$iface_default" ]] || die "Default interface not detected."
ip_default="$(default_src)"; gateway_default="$(default_gateway)"
iface="$(ask 'Public interface' "$iface_default")"
local_ip="$(ask 'This server public IPv4' "$ip_default")"; valid_ipv4 "$local_ip" || die "Invalid IPv4."
gateway="$(ask 'Public gateway' "$gateway_default")"
transport_port="$(ask "${transport^^} carrier port" '9000')"; valid_port "$transport_port" || die "Invalid carrier port."
ports_raw="$(ask 'Service ports (comma separated)' '443')"
mtu="$(ask 'Tunnel MTU' '1320')"; [[ "$mtu" =~ ^[0-9]+$ ]] && (( mtu >= 900 && mtu <= 1400 )) || die "MTU must be 900..1400."
if [[ "$mode" == direct-return && "$role" == iran ]] && ! is_local_ip "$iface" "$local_ip"; then die "Direct Return requires Iran public IPv4 to be assigned on $iface; upstream NAT/CGNAT-only is not supported."; fi
if [[ "$smart" == true && "$transport" != tcp && "$probe_port" == "$transport_port" ]]; then die "Smart probe UDP port must differ from UDP/KCP carrier port."; fi

ports_json=""; IFS=',' read -r -a parr <<< "$ports_raw"
for raw in "${parr[@]}"; do
  p="${raw//[[:space:]]/}"; [[ -z "$p" ]] && continue; valid_port "$p" || die "Invalid service port: $p"
  [[ "$p" == "$transport_port" ]] && die "Carrier port conflicts with service port: $p"
  [[ "$smart" == true && "$p" == "$probe_port" ]] && die "Smart probe port conflicts with service port: $p"
  [[ -n "$ports_json" ]] && ports_json+=","
  ports_json+="{\"port\":$p,\"protocol\":\"both\"}"
done
[[ -n "$ports_json" ]] || die "At least one service port is required."

step 7 8 "Peer & security"
if [[ "$role" == iran ]]; then
  foreign_ip="$(ask 'Kharej public IPv4' '')"; valid_ipv4 "$foreign_ip" || die "Invalid Kharej IPv4."
  iran_ip="$local_ip"; listen="0.0.0.0:$transport_port"; peer="$foreign_ip:$transport_port"; key="$($BIN -keygen)"; tun_cidr="10.77.0.1/30"; tun_peer="10.77.0.2"; lock=false
  echo; printf '%b\n' "${YELLOW}${BOLD}┌────────────── Shared Key ──────────────┐${RESET}"; printf '%b\n' "${WHITE}${BOLD}  $key${RESET}"; printf '%b\n' "${YELLOW}${BOLD}└─────────────────────────────────────────┘${RESET}"
  printf '%b\n' "${GRAY}  Kharej باید همان Mode / Carrier / Port / Smart settings / Performance profile / Key را داشته باشد.${RESET}"
  read -r -p "  بعد از ذخیره کلید Enter بزنید... " _
else
  iran_ip="$(ask 'Iran public IPv4' '')"; valid_ipv4 "$iran_ip" || die "Invalid Iran IPv4."
  foreign_ip="$local_ip"; listen="0.0.0.0:$transport_port"; peer=""; key="$(ask 'Shared Key from Iran' '')"; [[ -n "$key" ]] || die "Shared Key required."
  tun_cidr="10.77.0.2/30"; tun_peer="10.77.0.1"; if yesno 'Block direct public access to service ports?' 'Y'; then lock=true; else lock=false; fi
fi

if [[ -f "$CONF" ]]; then
  backup="${CONF}.bak.$(date +%Y%m%d-%H%M%S)"; cp -a "$CONF" "$backup"; info "Backup: $backup"
  systemctl stop hashshashin >/dev/null 2>&1 || true; "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true
fi

cat > "$CONF.tmp" <<JSON
{
  "role": "$role",
  "mode": "$mode",
  "transport": {
    "type": "$transport", "listen": "$listen", "peer": "$peer", "key": "$key",
    "keepalive_seconds": 5, "session_timeout_seconds": 25, "rekey_minutes": 30,
    "kcp": {"data_shards": $kcp_data, "parity_shards": $kcp_parity, "nodelay": $kcp_nodelay, "interval": $kcp_interval, "resend": $kcp_resend, "nc": $kcp_nc, "send_window": $kcp_sndwnd, "receive_window": $kcp_rcvwnd, "mtu": $kcp_mtu, "socket_buffer": $kcp_buffer}
  },
  "smart_return": {"enabled": $smart, "probe_port": $probe_port, "interval_seconds": $probe_interval, "timeout_seconds": $probe_timeout, "fail_threshold": $fail_threshold, "recover_threshold": $recover_threshold},
  "performance": {"profile": "$perf_profile", "tun_queues": $tun_queues, "receive_workers": $recv_workers, "tx_queue_len": $tx_queue_len, "qdisc": "$qdisc", "socket_buffer": $socket_buffer, "stats_interval_seconds": $stats_interval},
  "tun": {"name": "hsh0", "local_cidr": "$tun_cidr", "peer_ip": "$tun_peer", "mtu": $mtu},
  "network": {"public_interface": "$iface", "public_ip": "$local_ip", "public_gateway": "$gateway", "foreign_public_ip": "$foreign_ip", "iran_public_ip": "$iran_ip", "lock_service_ports": $lock},
  "ports": [$ports_json]
}
JSON
chmod 0600 "$CONF.tmp"; "$BIN" -check -c "$CONF.tmp"; mv "$CONF.tmp" "$CONF"; chmod 0600 "$CONF"; ok "Configuration validated."

step 8 8 "Start & verify"
systemctl daemon-reload; systemctl enable hashshashin >/dev/null; systemctl restart hashshashin; sleep 2
if ! systemctl is-active --quiet hashshashin; then systemctl --no-pager --full status hashshashin || true; journalctl -u hashshashin -n 80 --no-pager || true; die "Service failed to start."; fi
ok "Systemd service ACTIVE."; ip link show hsh0 >/dev/null 2>&1 && ok "TUN hsh0 created." || warn "hsh0 not visible yet."; "$BIN" -check -c "$CONF" >/dev/null && ok "Config check passed."

echo
printf '%b\n' "${GREEN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
printf '%b\n' "${GREEN}${BOLD}║${RESET}                 ${WHITE}${BOLD}INSTALLATION COMPLETE${RESET}                    ${GREEN}${BOLD}║${RESET}"
printf '%b\n' "${GREEN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
printf '  Role          : %s\n  Mode          : %s\n  Carrier       : %s/%s\n  Performance   : %s (%s TUN queues, qlen %s, %s)\n  Service Ports : %s\n  TUN           : %s\n' "$role" "$mode" "${transport^^}" "$transport_port" "$perf_profile" "$tun_queues" "$tx_queue_len" "$qdisc" "$ports_raw" "$tun_cidr"
[[ "$transport" == kcp ]] && printf '  KCP FEC       : %s/%s\n' "$kcp_data" "$kcp_parity"
if [[ "$smart" == true ]]; then printf '  Smart Return  : ON (UDP probe %s, fail/recover %s/%s)\n' "$probe_port" "$fail_threshold" "$recover_threshold"; else printf '  Smart Return  : OFF\n'; fi

echo
[[ "$transport" == tcp ]] && printf '%b\n' "${GRAY}  Provider Firewall: TCP/${transport_port} فقط بین ایران و خارج.${RESET}" || printf '%b\n' "${GRAY}  Provider Firewall: UDP/${transport_port} فقط بین ایران و خارج.${RESET}"
[[ "$smart" == true ]] && printf '%b\n' "${GRAY}  Smart Return: UDP/${probe_port} را از Kharej IP به Iran IP اجازه دهید.${RESET}"
printf '%b\n' "  مدیریت: ${CYAN}${BOLD}hashshashin${RESET}"
printf '%b\n' "  تست تکرارپذیر: ${CYAN}${BOLD}hsh-bench${RESET}"
if [[ "$role" == iran ]]; then printf '%b\n' "${YELLOW}${BOLD}  NEXT:${RESET} روی Kharej همان Mode/Carrier/Port/Smart/Performance/Shared Key را وارد کنید."; else printf '%b\n' "${GREEN}${BOLD}  NEXT:${RESET} Health Check را روی هر دو سرور اجرا کنید."; fi

echo; if yesno 'Open Hashshashin Manager now?' 'Y'; then exec "$MANAGER"; fi
