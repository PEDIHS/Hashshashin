#!/usr/bin/env bash
set -u

BIN="/usr/local/bin/hashshashin"
BENCH="/usr/local/bin/hsh-bench"
CONF="/etc/hashshashin/config.json"
SERVICE="hashshashin"
INSTALL_URL="https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh"

if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  RESET='\033[0m'; BOLD='\033[1m'; RED='\033[38;5;196m'; GREEN='\033[38;5;82m'; YELLOW='\033[38;5;220m'
  CYAN='\033[38;5;45m'; BLUE='\033[38;5;75m'; PURPLE='\033[38;5;141m'; WHITE='\033[38;5;255m'; GRAY='\033[38;5;245m'
else
  RESET=''; BOLD=''; RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; PURPLE=''; WHITE=''; GRAY=''
fi

clear_screen(){ [[ -t 1 ]] && clear 2>/dev/null || true; }
line(){ printf '%b\n' "${GRAY}────────────────────────────────────────────────────────────────${RESET}"; }
pause(){ echo; read -r -p "  برای بازگشت Enter بزنید... " _; }
confirm(){ local a; read -r -p "  $1 [y/N]: " a; [[ "$a" =~ ^[Yy]$ ]]; }
ask(){ local p="$1" d="$2" v; read -r -p "  $p [$d]: " v; printf '%s' "${v:-$d}"; }

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then exec sudo "$0" "$@"; fi
  echo "Hashshashin Manager must run as root." >&2; exit 1
fi

banner(){
  printf '%b\n' "${CYAN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}              ${WHITE}${BOLD}H A S H S H A S H I N${RESET}                     ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}       ${GRAY}Adaptive L3 Tunnel • Data Plane Manager${RESET}             ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
  printf '%b\n' "              ${PURPLE}حشاشین • پنل مدیریت تونل${RESET}"
}

role="-"; mode="-"; transport="udp"; tun_name="hsh0"; tun_cidr="-"; tun_peer="-"; mtu="-"
public_interface="-"; public_ip="-"; foreign_public_ip="-"; iran_public_ip="-"
carrier_listen="-"; carrier_peer="-"; service_ports="-"; kcp_fec="-"; kcp_window="-"
smart_return="false"; smart_probe_port="-"; smart_interval="-"; smart_timeout="-"; smart_thresholds="-"
performance_profile="-"; tun_queues="-"; receive_workers="-"; tx_queue_len="-"; qdisc="-"; socket_buffer="-"

load_summary(){
  role="-"; mode="-"; transport="udp"; tun_name="hsh0"; mtu="-"; smart_return="false"; smart_probe_port="-"; smart_thresholds="-"
  performance_profile="-"; tun_queues="-"; receive_workers="-"; tx_queue_len="-"; qdisc="-"; socket_buffer="-"
  [[ -x "$BIN" && -f "$CONF" ]] || return 0
  while IFS='=' read -r k v; do
    case "$k" in
      role) role="$v";; mode) mode="$v";; transport) transport="$v";; tun_name) tun_name="$v";; tun_cidr) tun_cidr="$v";; tun_peer) tun_peer="$v";; mtu) mtu="$v";;
      public_interface) public_interface="$v";; public_ip) public_ip="$v";; foreign_public_ip) foreign_public_ip="$v";; iran_public_ip) iran_public_ip="$v";;
      carrier_listen) carrier_listen="$v";; carrier_peer) carrier_peer="$v";; service_ports) service_ports="$v";;
      kcp_fec) kcp_fec="$v";; kcp_window) kcp_window="$v";; smart_return) smart_return="$v";; smart_probe_port) smart_probe_port="$v";;
      smart_interval) smart_interval="$v";; smart_timeout) smart_timeout="$v";; smart_thresholds) smart_thresholds="$v";;
      performance_profile) performance_profile="$v";; tun_queues) tun_queues="$v";; receive_workers) receive_workers="$v";;
      tx_queue_len) tx_queue_len="$v";; qdisc) qdisc="$v";; socket_buffer) socket_buffer="$v";;
    esac
  done < <("$BIN" -summary -c "$CONF" 2>/dev/null || true)
}
service_state(){ systemctl is-active "$SERVICE" 2>/dev/null || true; }
service_enabled(){ systemctl is-enabled "$SERVICE" 2>/dev/null || true; }
role_label(){ [[ "$role" == iran ]] && echo "IRAN / Entry" || { [[ "$role" == kharej ]] && echo "KHAREJ / Exit" || echo "$role"; }; }
mode_label(){ [[ "$mode" == full ]] && echo "Full Tunnel" || { [[ "$mode" == direct-return ]] && echo "Direct Return" || echo "$mode"; }; }
transport_label(){ echo "${transport^^}"; }
human_bytes(){ command -v numfmt >/dev/null 2>&1 && numfmt --to=iec-i --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0} B"; }
iface_counter(){ local f="/sys/class/net/${tun_name}/statistics/$1"; [[ -r "$f" ]] && cat "$f" || echo 0; }
actual_qlen(){ local f="/sys/class/net/${tun_name}/tx_queue_len"; [[ -r "$f" ]] && cat "$f" || echo 0; }
actual_queues(){ local d="/sys/class/net/${tun_name}/queues"; [[ -d "$d" ]] || { echo 0; return; }; find "$d" -maxdepth 1 -type d -name 'tx-*' 2>/dev/null | wc -l; }
state_badge(){ case "$1" in active) printf '%b' "${GREEN}● ACTIVE${RESET}";; failed) printf '%b' "${RED}● FAILED${RESET}";; *) printf '%b' "${YELLOW}● ${1^^}${RESET}";; esac; }

return_path(){
  [[ "$smart_return" == true && "$mode" == direct-return ]] || { echo "-"; return; }
  if [[ "$role" == iran ]]; then echo "RESPONDER"; return; fi
  if ip route show table 168 "$iran_public_ip/32" 2>/dev/null | grep -q "$tun_name"; then echo "TUNNEL"; else echo "DIRECT"; fi
}

header_status(){
  load_summary
  local st ver rx tx path drops
  st="$(service_state)"; ver="$($BIN -version 2>/dev/null | awk '{print $2}' || echo '-')"; rx="$(human_bytes "$(iface_counter rx_bytes)")"; tx="$(human_bytes "$(iface_counter tx_bytes)")"; path="$(return_path)"
  drops=$(( $(iface_counter tx_dropped) + $(iface_counter rx_dropped) ))
  line
  printf '  %-17s %b\n' "Service:" "$(state_badge "$st")"
  printf '  %-17s %-16s %-14s %s\n' "Version:" "$ver" "Role:" "$(role_label)"
  printf '  %-17s %-16s %-14s %s\n' "Mode:" "$(mode_label)" "Carrier:" "$(transport_label)"
  printf '  %-17s %-16s %-14s %s\n' "TUN:" "${tun_name}/${mtu}" "Public IP:" "$public_ip"
  printf '  %-17s %-16s %-14s %s\n' "Performance:" "$performance_profile" "Queues/qlen:" "$(actual_queues)/$(actual_qlen)"
  if (( drops > 0 )); then printf '  %-17s %b\n' "TUN Drops:" "${RED}${drops}${RESET}"; else printf '  %-17s %b\n' "TUN Drops:" "${GREEN}0${RESET}"; fi
  if [[ "$smart_return" == true ]]; then printf '  %-17s %b     %-14s %b\n' "Smart Return:" "${GREEN}ON${RESET}" "Return Path:" "${CYAN}${path}${RESET}"; else printf '  %-17s %b\n' "Smart Return:" "${GRAY}OFF${RESET}"; fi
  printf '  %-17s %-16s %-14s %s\n' "Tunnel RX:" "$rx" "Tunnel TX:" "$tx"
  line
}

menu(){
  printf '%b\n' "  ${WHITE}${BOLD}مدیریت تونل${RESET}"
  printf '%b\n' "  ${CYAN}1)${RESET} Overview & Smart Path"
  printf '%b\n' "  ${CYAN}2)${RESET} Health Check"
  printf '%b\n' "  ${GREEN}3)${RESET} Restart Tunnel"
  printf '%b\n' "  ${GREEN}4)${RESET} Start Tunnel"
  printf '%b\n' "  ${YELLOW}5)${RESET} Stop Tunnel"
  printf '%b\n' "  ${PURPLE}6)${RESET} Live Logs"
  echo
  printf '%b\n' "  ${WHITE}${BOLD}تنظیمات و نگهداری${RESET}"
  printf '%b\n' "  ${BLUE}7)${RESET} Safe Config View"
  printf '%b\n' "  ${BLUE}8)${RESET} Reconfigure / Mode / Carrier / Performance"
  printf '%b\n' "  ${BLUE}9)${RESET} Update Hashshashin"
  printf '%b\n' "  ${CYAN}10)${RESET} Network Diagnostics"
  printf '%b\n' "  ${YELLOW}11)${RESET} Reset Network State"
  printf '%b\n' "  ${RED}12)${RESET} Uninstall"
  printf '%b\n' "  ${GREEN}13)${RESET} Reproducible Benchmark       ${GRAY}iperf3 multi-stream + drop delta${RESET}"
  printf '%b\n' "  ${GRAY}0) Exit${RESET}"; echo
}

show_overview(){
  clear_screen; banner; header_status
  printf '%b\n' "${BOLD}  Tunnel Overview${RESET}"
  printf '  Role             : %s\n' "$(role_label)"
  printf '  Mode             : %s\n' "$(mode_label)"
  printf '  Carrier          : %s\n' "$(transport_label)"
  printf '  Public interface : %s\n' "$public_interface"
  printf '  Kharej / Iran IP : %s / %s\n' "$foreign_public_ip" "$iran_public_ip"
  printf '  TUN              : %s (%s -> %s)\n' "$tun_name" "$tun_cidr" "$tun_peer"
  printf '  Performance      : %s\n' "$performance_profile"
  printf '  Queues req/seen  : %s / %s\n' "$tun_queues" "$(actual_queues)"
  printf '  RX workers       : %s\n' "$receive_workers"
  printf '  txqueuelen       : configured=%s actual=%s\n' "$tx_queue_len" "$(actual_qlen)"
  printf '  qdisc            : %s\n' "$qdisc"
  printf '  socket buffer    : %s\n' "$socket_buffer"
  printf '  TUN drops        : tx=%s rx=%s\n' "$(iface_counter tx_dropped)" "$(iface_counter rx_dropped)"
  printf '  Carrier listen   : %s\n' "$carrier_listen"
  printf '  Carrier peer     : %s\n' "$carrier_peer"
  [[ "$transport" == kcp ]] && printf '  KCP FEC/window   : %s / %s\n' "$kcp_fec" "$kcp_window"
  printf '  Service ports    : %s\n' "$service_ports"
  if [[ "$smart_return" == true ]]; then
    printf '  Smart probe      : UDP/%s every %ss timeout %ss\n' "$smart_probe_port" "$smart_interval" "$smart_timeout"
    printf '  Fail/Recover     : %s\n' "$smart_thresholds"
    printf '  Current path     : %s\n' "$(return_path)"
  fi
  printf '  Service enabled  : %s\n' "$(service_enabled)"
  pause
}

check_item(){ if [[ "$1" == 1 ]]; then printf '  %b %-29s %s\n' "${GREEN}✔${RESET}" "$2" "${3:-}"; else printf '  %b %-29s %s\n' "${RED}✘${RESET}" "$2" "${3:-}"; fi; }
carrier_health(){
  local port="${carrier_listen##*:}"
  command -v ss >/dev/null 2>&1 || return 1
  if [[ "$transport" == tcp ]]; then
    if [[ "$role" == kharej ]]; then ss -H -ltn 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]"; else ss -H -tn 2>/dev/null | grep -Eq "${foreign_public_ip}:${port}[[:space:]]"; fi
  else
    ss -H -lun 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]"
  fi
}

health_check(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Health Check${RESET}"; line
  local st=0 cfg=0 tun=0 fwd=0 carrier=0 routing=0 smartok=1 qlenok=0 qdiscok=1 queuesok=1 dropsok=0 aq aqn ql drops
  [[ "$(service_state)" == active ]] && st=1
  "$BIN" -check -c "$CONF" >/dev/null 2>&1 && cfg=1
  ip link show "$tun_name" >/dev/null 2>&1 && tun=1
  [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo 0)" == 1 ]] && fwd=1
  carrier_health && carrier=1
  if [[ "$role" == iran ]]; then ip rule show | grep -q 'lookup 166' && routing=1; elif [[ "$mode" == full || "$smart_return" == true ]]; then ip rule show | grep -q 'lookup 167' && routing=1; else routing=1; fi
  if [[ "$smart_return" == true ]]; then
    if [[ "$role" == iran ]]; then ss -H -lun 2>/dev/null | grep -Eq "[:.]${smart_probe_port}[[:space:]]" || smartok=0
    else ip rule show 2>/dev/null | grep -q 'lookup 168' || smartok=0
    fi
  fi
  ql="$(actual_qlen)"; [[ "$ql" =~ ^[0-9]+$ && "$tx_queue_len" =~ ^[0-9]+$ && "$ql" -ge "$tx_queue_len" ]] && qlenok=1
  if [[ "$qdisc" != "none" ]]; then tc qdisc show dev "$tun_name" 2>/dev/null | grep -qw "$qdisc" || qdiscok=0; fi
  aq="$(actual_queues)"; aqn="${tun_queues:-1}"; [[ "$aqn" =~ ^[0-9]+$ ]] || aqn=1; [[ "$aq" =~ ^[0-9]+$ ]] || aq=0; (( aq >= aqn || aqn <= 1 )) || queuesok=0
  drops=$(( $(iface_counter tx_dropped) + $(iface_counter rx_dropped) )); (( drops == 0 )) && dropsok=1

  check_item "$st" "Systemd service" "$(service_state)"
  check_item "$cfg" "Configuration" "$CONF"
  check_item "$tun" "TUN interface" "$tun_name"
  check_item "$fwd" "IPv4 forwarding" "enabled"
  check_item "$carrier" "$(transport_label) carrier" "$carrier_listen"
  check_item "$routing" "Policy routing" "data/carrier isolation"
  check_item "$qlenok" "TUN txqueuelen" "actual=${ql} configured=${tx_queue_len}"
  check_item "$qdiscok" "TUN qdisc" "$qdisc"
  check_item "$queuesok" "TUN queues" "actual=${aq} requested=${aqn}"
  check_item "$dropsok" "TUN drops" "tx=$(iface_counter tx_dropped) rx=$(iface_counter rx_dropped)"
  [[ "$smart_return" == true ]] && check_item "$smartok" "Smart Return" "path=$(return_path) probe=UDP/${smart_probe_port}"
  echo
  if (( st && cfg && tun && fwd && carrier && routing && qlenok && qdiscok && queuesok && dropsok && smartok )); then printf '%b\n' "  ${GREEN}${BOLD}Health: PASS${RESET}"; else printf '%b\n' "  ${YELLOW}${BOLD}Health: ATTENTION${RESET}"; fi
  printf '%b\n' "  ${GRAY}Drop counters are cumulative since hsh0 was created. Use Benchmark to compare before/after deltas under load.${RESET}"
  pause
}

service_action(){ systemctl "$1" "$SERVICE" && printf '%b\n' "${GREEN}  ✔ انجام شد.${RESET}" || printf '%b\n' "${RED}  ✘ ناموفق.${RESET}"; sleep 1; }
live_logs(){ clear_screen; banner; echo; printf '%b\n' "${PURPLE}  Live Logs — Ctrl+C برای بازگشت${RESET}"; journalctl -u "$SERVICE" -f -n 100; }
show_config(){ clear_screen; banner; line; [[ -f "$CONF" ]] && sed -E 's#"key"[[:space:]]*:[[:space:]]*"[^"]+"#"key": "***REDACTED***"#' "$CONF" || echo "Config missing"; pause; }
remote_installer(){ command -v curl >/dev/null 2>&1 || { echo "curl missing"; pause; return; }; curl -fsSL "$INSTALL_URL" | bash -s -- "$1"; }

network_diagnostics(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Network Diagnostics${RESET}"; line
  echo "[TUN]"; ip -details link show "$tun_name" 2>&1 || true
  echo; echo "[TUN queues]"; ls -1 "/sys/class/net/${tun_name}/queues" 2>/dev/null || true
  echo; echo "[Qdisc]"; tc -s qdisc show dev "$tun_name" 2>&1 || true
  echo; echo "[TUN drops]"; echo "tx_dropped=$(iface_counter tx_dropped) rx_dropped=$(iface_counter rx_dropped)"
  echo; echo "[Rules]"; ip rule show 2>&1 || true
  echo; echo "[Table 166 / data]"; ip route show table 166 2>&1 || true
  echo; echo "[Table 167 / carrier bypass]"; ip route show table 167 2>&1 || true
  echo; echo "[Table 168 / smart return]"; ip route show table 168 2>&1 || true
  echo; echo "[Sockets]"; ss -lntup 2>/dev/null | grep -E "hashshashin|:${carrier_listen##*:}|:${smart_probe_port}" || true
  echo; echo "[Firewall counters]"; for t in mangle nat filter; do iptables -t "$t" -nvxL 2>/dev/null | grep -E 'HSH_|Chain HSH' || true; done
  echo; echo "[Recent data-plane stats]"; journalctl -u "$SERVICE" -n 200 --no-pager 2>/dev/null | grep 'stats role=' | tail -n 15 || true
  echo; echo "[Recent logs]"; journalctl -u "$SERVICE" -n 40 --no-pager 2>/dev/null || true
  pause
}

benchmark_menu(){
  clear_screen; banner; echo
  [[ -x "$BENCH" ]] || { printf '%b\n' "${RED}hsh-bench نصب نیست؛ Update/Repair را اجرا کنید.${RESET}"; pause; return; }
  printf '%b\n' "${BOLD}  Reproducible Benchmark${RESET}"
  printf '%b\n' "  ${CYAN}1)${RESET} Start persistent iperf3 server"
  printf '%b\n' "  ${GREEN}2)${RESET} Run client benchmark (warmup + repeated forward/reverse)"
  printf '%b\n' "  ${GRAY}0) Back${RESET}"
  local c host port runs parallel
  read -r -p "  انتخاب [0-2]: " c
  case "$c" in
    1) port="$(ask 'iperf3 port' '39001')"; "$BENCH" server "$port"; pause;;
    2)
      host="$(ask 'Benchmark peer IP/host' '')"; [[ -n "$host" ]] || { pause; return; }
      port="$(ask 'iperf3 port' '39001')"; runs="$(ask 'Measured runs' '5')"; parallel="$(ask 'Parallel TCP streams' '8')"
      "$BENCH" client "$host" "$port" "$runs" "$parallel"; pause;;
    *) return;;
  esac
}

reset_network(){ clear_screen; banner; confirm "Network state پاک و دوباره ساخته شود؟" || return; systemctl stop "$SERVICE" >/dev/null 2>&1 || true; "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true; systemctl start "$SERVICE"; pause; }
uninstall_menu(){ clear_screen; banner; confirm "Hashshashin حذف شود؟ Config نگه داشته می‌شود." || return; remote_installer --uninstall; exit 0; }

while true; do
  clear_screen; banner
  [[ -x "$BIN" && -f "$CONF" ]] || { echo "Installation/config ناقص است."; exit 1; }
  header_status; menu; read -r -p "  انتخاب [0-13]: " choice
  case "$choice" in
    1) show_overview;; 2) health_check;; 3) service_action restart;; 4) service_action start;; 5) service_action stop;; 6) live_logs;;
    7) show_config;; 8) clear_screen; banner; remote_installer --reconfigure; pause;; 9) clear_screen; banner; remote_installer --update; pause;;
    10) network_diagnostics;; 11) reset_network;; 12) uninstall_menu;; 13) benchmark_menu;; 0|q|Q) clear_screen; exit 0;; *) sleep 1;;
  esac
done
