#!/usr/bin/env bash
set -u

BIN="/usr/local/bin/hashshashin"
CONF="/etc/hashshashin/config.json"
SERVICE="hashshashin"
INSTALL_URL="https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh"

if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  RESET='\033[0m'; BOLD='\033[1m'; RED='\033[38;5;196m'; GREEN='\033[38;5;82m'
  YELLOW='\033[38;5;220m'; CYAN='\033[38;5;45m'; BLUE='\033[38;5;75m'
  PURPLE='\033[38;5;141m'; WHITE='\033[38;5;255m'; GRAY='\033[38;5;245m'
else
  RESET=''; BOLD=''; RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; PURPLE=''; WHITE=''; GRAY=''
fi

clear_screen(){ [[ -t 1 ]] && clear 2>/dev/null || true; }
line(){ printf '%b\n' "${GRAY}────────────────────────────────────────────────────────────────${RESET}"; }
pause(){ echo; read -r -p "  برای بازگشت Enter را بزنید... " _; }
confirm(){ local a; read -r -p "  $1 [y/N]: " a; [[ "$a" =~ ^[Yy]$ ]]; }

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  if command -v sudo >/dev/null 2>&1; then exec sudo "$0" "$@"; fi
  echo "Hashshashin Manager must run as root." >&2
  exit 1
fi

banner(){
  printf '%b\n' "${CYAN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}              ${WHITE}${BOLD}H A S H S H A S H I N${RESET}                     ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}          ${GRAY}Professional L3 Tunnel Manager${RESET}                  ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
  printf '%b\n' "              ${PURPLE}حشاشین • مدیریت حرفه‌ای تونل${RESET}"
}

role="-"; mode="-"; transport="udp"; tun_name="hsh0"; tun_cidr="-"; tun_peer="-"; mtu="-"
public_interface="-"; public_ip="-"; foreign_public_ip="-"; iran_public_ip="-"
carrier_listen="-"; carrier_peer="-"; service_ports="-"; kcp_fec="-"; kcp_window="-"

load_summary(){
  role="-"; mode="-"; transport="udp"; tun_name="hsh0"; tun_cidr="-"; tun_peer="-"; mtu="-"
  public_interface="-"; public_ip="-"; foreign_public_ip="-"; iran_public_ip="-"
  carrier_listen="-"; carrier_peer="-"; service_ports="-"; kcp_fec="-"; kcp_window="-"
  [[ -x "$BIN" && -f "$CONF" ]] || return 0
  while IFS='=' read -r k v; do
    case "$k" in
      role) role="$v";; mode) mode="$v";; transport) transport="$v";; tun_name) tun_name="$v";; tun_cidr) tun_cidr="$v";; tun_peer) tun_peer="$v";; mtu) mtu="$v";;
      public_interface) public_interface="$v";; public_ip) public_ip="$v";; foreign_public_ip) foreign_public_ip="$v";; iran_public_ip) iran_public_ip="$v";;
      carrier_listen) carrier_listen="$v";; carrier_peer) carrier_peer="$v";; service_ports) service_ports="$v";; kcp_fec) kcp_fec="$v";; kcp_window) kcp_window="$v";;
    esac
  done < <("$BIN" -summary -c "$CONF" 2>/dev/null || true)
}
service_state(){ systemctl is-active "$SERVICE" 2>/dev/null || true; }
service_enabled(){ systemctl is-enabled "$SERVICE" 2>/dev/null || true; }
role_label(){ case "$role" in iran) echo "IRAN / Entry";; kharej) echo "KHAREJ / Exit";; *) echo "$role";; esac; }
mode_label(){ case "$mode" in full) echo "Full Tunnel";; direct-return) echo "Direct Return";; *) echo "$mode";; esac; }
transport_label(){ case "$transport" in udp) echo "UDP";; tcp) echo "TCP";; kcp) echo "KCP";; *) echo "${transport^^}";; esac; }
state_badge(){
  case "$1" in active) printf '%b' "${GREEN}● ACTIVE${RESET}";; failed) printf '%b' "${RED}● FAILED${RESET}";; inactive) printf '%b' "${YELLOW}● STOPPED${RESET}";; *) printf '%b' "${GRAY}● ${1^^}${RESET}";; esac
}
human_bytes(){ command -v numfmt >/dev/null 2>&1 && numfmt --to=iec-i --suffix=B "${1:-0}" 2>/dev/null || echo "${1:-0} B"; }
iface_counter(){ local f="/sys/class/net/${tun_name}/statistics/$1"; [[ -r "$f" ]] && cat "$f" || echo 0; }
carrier_ss_args(){ [[ "$transport" == "tcp" ]] && echo "-H -ltn" || echo "-H -lun"; }

header_status(){
  load_summary
  local st ver rx tx
  st="$(service_state)"; ver="$($BIN -version 2>/dev/null | awk '{print $2}' || echo '-')"
  rx="$(human_bytes "$(iface_counter rx_bytes)")"; tx="$(human_bytes "$(iface_counter tx_bytes)")"
  line
  printf '  %-16s %b\n' "وضعیت سرویس:" "$(state_badge "$st")"
  printf '  %-16s %b     %-14s %b\n' "نسخه:" "${WHITE}${ver}${RESET}" "نقش:" "${WHITE}$(role_label)${RESET}"
  printf '  %-16s %b     %-14s %b\n' "حالت:" "${WHITE}$(mode_label)${RESET}" "Carrier:" "${CYAN}$(transport_label)${RESET}"
  printf '  %-16s %b     %-14s %b\n' "Public IP:" "${WHITE}${public_ip}${RESET}" "TUN/MTU:" "${WHITE}${tun_name}/${mtu}${RESET}"
  printf '  %-16s %b\n' "Service Ports:" "${WHITE}${service_ports}${RESET}"
  printf '  %-16s %b     %-14s %b\n' "Tunnel RX:" "${GREEN}${rx}${RESET}" "Tunnel TX:" "${CYAN}${tx}${RESET}"
  line
}

menu(){
  printf '%b\n' "  ${WHITE}${BOLD}مدیریت تونل${RESET}"
  printf '%b\n' "  ${CYAN}1)${RESET}  Overview                    ${GRAY}اطلاعات کامل تونل و carrier${RESET}"
  printf '%b\n' "  ${CYAN}2)${RESET}  Health Check                ${GRAY}سرویس، TUN، routing و carrier${RESET}"
  printf '%b\n' "  ${GREEN}3)${RESET}  Restart Tunnel              ${GRAY}راه‌اندازی مجدد${RESET}"
  printf '%b\n' "  ${GREEN}4)${RESET}  Start Tunnel                ${GRAY}اجرای سرویس${RESET}"
  printf '%b\n' "  ${YELLOW}5)${RESET}  Stop Tunnel                 ${GRAY}توقف سرویس${RESET}"
  printf '%b\n' "  ${PURPLE}6)${RESET}  Live Logs                   ${GRAY}خروج با Ctrl+C${RESET}"
  echo
  printf '%b\n' "  ${WHITE}${BOLD}تنظیمات و نگهداری${RESET}"
  printf '%b\n' "  ${BLUE}7)${RESET}  Safe Config View            ${GRAY}Shared Key مخفی می‌شود${RESET}"
  printf '%b\n' "  ${BLUE}8)${RESET}  Reconfigure / Carrier       ${GRAY}تغییر Mode/Transport/Ports${RESET}"
  printf '%b\n' "  ${BLUE}9)${RESET}  Update Hashshashin          ${GRAY}آپدیت بدون حذف Config${RESET}"
  printf '%b\n' "  ${CYAN}10)${RESET} Network Diagnostics         ${GRAY}rules / routes / firewall / sockets${RESET}"
  printf '%b\n' "  ${YELLOW}11)${RESET} Reset Network State         ${GRAY}پاک‌سازی و بازسازی ruleها${RESET}"
  printf '%b\n' "  ${RED}12)${RESET} Uninstall                   ${GRAY}حذف Hashshashin${RESET}"
  echo
  printf '%b\n' "  ${GRAY}0) Exit${RESET}"
  echo
}

show_overview(){
  clear_screen; banner; header_status
  printf '%b\n' "${BOLD}  Tunnel Overview${RESET}"
  printf '  Role             : %s\n' "$(role_label)"
  printf '  Mode             : %s\n' "$(mode_label)"
  printf '  Carrier          : %s\n' "$(transport_label)"
  printf '  Public interface : %s\n' "$public_interface"
  printf '  Public IP        : %s\n' "$public_ip"
  printf '  Kharej IP        : %s\n' "$foreign_public_ip"
  printf '  Iran IP          : %s\n' "$iran_public_ip"
  printf '  TUN              : %s (%s -> %s)\n' "$tun_name" "$tun_cidr" "$tun_peer"
  printf '  Carrier listen   : %s\n' "$carrier_listen"
  printf '  Carrier peer     : %s\n' "$carrier_peer"
  [[ "$transport" == "kcp" ]] && printf '  KCP FEC/window   : %s / %s\n' "$kcp_fec" "$kcp_window"
  printf '  Service ports    : %s\n' "$service_ports"
  printf '  Service enabled  : %s\n' "$(service_enabled)"
  local since; since="$(systemctl show "$SERVICE" -p ActiveEnterTimestamp --value 2>/dev/null || true)"
  [[ -n "$since" ]] && printf '  Active since     : %s\n' "$since"
  pause
}

check_item(){ if [[ "$1" == 1 ]]; then printf '  %b %-27s %s\n' "${GREEN}✔${RESET}" "$2" "${3:-}"; else printf '  %b %-27s %s\n' "${RED}✘${RESET}" "$2" "${3:-}"; fi; }
health_check(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Health Check${RESET}"; line
  local st=0 cfg=0 tun=0 fwd=0 carrier=0 routing=0 port ss_args
  [[ "$(service_state)" == active ]] && st=1
  "$BIN" -check -c "$CONF" >/dev/null 2>&1 && cfg=1
  ip link show "$tun_name" >/dev/null 2>&1 && tun=1
  [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo 0)" == 1 ]] && fwd=1
  port="${carrier_listen##*:}"; ss_args="$(carrier_ss_args)"
  if command -v ss >/dev/null 2>&1 && [[ "$port" =~ ^[0-9]+$ ]]; then
    # shellcheck disable=SC2086
    ss $ss_args 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]" && carrier=1
  fi
  if [[ "$role" == iran ]]; then
    ip rule show 2>/dev/null | grep -q 'lookup 166' && routing=1
  elif [[ "$mode" == full ]]; then
    ip rule show 2>/dev/null | grep -q 'lookup 167' && routing=1
  else
    routing=1
  fi
  check_item "$st" "Systemd service" "$(service_state)"
  check_item "$cfg" "Configuration" "$CONF"
  check_item "$tun" "TUN interface" "$tun_name"
  check_item "$fwd" "IPv4 forwarding" "enabled"
  check_item "$carrier" "$(transport_label) carrier socket" "$carrier_listen"
  check_item "$routing" "Policy routing" "role/mode expected state"
  echo
  if [[ "$st$cfg$tun$fwd$carrier$routing" == 111111 ]]; then
    printf '%b\n' "  ${GREEN}${BOLD}Health: PASS${RESET} — بررسی‌های local سالم هستند."
  else
    printf '%b\n' "  ${YELLOW}${BOLD}Health: ATTENTION${RESET} — Diagnostics و Logs را بررسی کنید."
  fi
  printf '%b\n' "  ${GRAY}Health Check محلی است؛ Verify دو VPS همچنان برای مسیر واقعی لازم است.${RESET}"
  pause
}

service_action(){
  printf '%b\n' "${CYAN}  → systemctl $1 ${SERVICE}${RESET}"
  if systemctl "$1" "$SERVICE"; then printf '%b\n' "${GREEN}  ✔ انجام شد.${RESET}"; else printf '%b\n' "${RED}  ✘ عملیات ناموفق بود.${RESET}"; fi
  sleep 1
}
live_logs(){ clear_screen; banner; echo; printf '%b\n' "${PURPLE}  Live Logs — برای بازگشت Ctrl+C بزنید.${RESET}"; echo; journalctl -u "$SERVICE" -f -n 80; }
show_config(){
  clear_screen; banner; echo; printf '%b\n' "${BOLD}  Configuration (Shared Key redacted)${RESET}"; line
  if [[ -f "$CONF" ]]; then
    sed -E 's#"key"[[:space:]]*:[[:space:]]*"[^"]+"#"key": "***REDACTED***"#' "$CONF"
  else
    printf '%b\n' "${RED}Config not found: ${CONF}${RESET}"
  fi
  pause
}
remote_installer(){
  command -v curl >/dev/null 2>&1 || { printf '%b\n' "${RED}curl نصب نیست.${RESET}"; pause; return; }
  printf '%b\n' "${CYAN}  دریافت installer رسمی از GitHub...${RESET}"
  curl -fsSL "$INSTALL_URL" | bash -s -- "$1"
}
network_diagnostics(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Network Diagnostics${RESET}"; line
  printf '%b\n' "${CYAN}[TUN]${RESET}"; ip -br addr show "$tun_name" 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Policy rules]${RESET}"; ip rule show 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Table 166 / data]${RESET}"; ip route show table 166 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Table 167 / carrier]${RESET}"; ip route show table 167 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Carrier socket: $(transport_label)]${RESET}"
  if [[ "$transport" == "tcp" ]]; then ss -ltnp 2>/dev/null | grep -E "hashshashin|${carrier_listen##*:}" || true; else ss -lunp 2>/dev/null | grep -E "hashshashin|${carrier_listen##*:}" || true; fi
  echo; printf '%b\n' "${CYAN}[Hashshashin firewall chains]${RESET}"
  for t in mangle nat filter; do iptables -t "$t" -S 2>/dev/null | grep 'HSH_' || true; done
  echo; printf '%b\n' "${CYAN}[Recent logs]${RESET}"; journalctl -u "$SERVICE" -n 25 --no-pager 2>/dev/null || true
  pause
}
reset_network(){
  clear_screen; banner; echo
  printf '%b\n' "${YELLOW}سرویس متوقف می‌شود، ruleهای Hashshashin پاک می‌شوند و سپس دوباره ساخته می‌شوند.${RESET}"
  confirm "ادامه می‌دهید؟" || return
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true
  if systemctl start "$SERVICE"; then printf '%b\n' "${GREEN}  ✔ Network state rebuilt.${RESET}"; else printf '%b\n' "${RED}  ✘ سرویس بالا نیامد؛ Logs را بررسی کنید.${RESET}"; fi
  pause
}
uninstall_menu(){
  clear_screen; banner; echo
  printf '%b\n' "${RED}${BOLD}Hashshashin حذف خواهد شد.${RESET}"
  printf '%b\n' "${GRAY}Config برای بازیابی نگه داشته می‌شود.${RESET}"
  confirm "حذف انجام شود؟" || return
  remote_installer --uninstall
  exit 0
}

while true; do
  clear_screen; banner
  [[ -x "$BIN" && -f "$CONF" ]] || { echo; printf '%b\n' "${RED}Installation/config ناقص است؛ installer رسمی را دوباره اجرا کنید.${RESET}"; exit 1; }
  header_status; menu
  read -r -p "  انتخاب شما [0-12]: " choice
  case "$choice" in
    1) show_overview;;
    2) health_check;;
    3) service_action restart;;
    4) service_action start;;
    5) service_action stop;;
    6) live_logs;;
    7) show_config;;
    8) clear_screen; banner; remote_installer --reconfigure; pause;;
    9) clear_screen; banner; remote_installer --update; pause;;
    10) network_diagnostics;;
    11) reset_network;;
    12) uninstall_menu;;
    0|q|Q) clear_screen; exit 0;;
    *) printf '%b\n' "${RED}  انتخاب نامعتبر.${RESET}"; sleep 1;;
  esac
done
