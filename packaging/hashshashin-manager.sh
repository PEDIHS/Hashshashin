#!/usr/bin/env bash
set -u

BIN="/usr/local/bin/hashshashin"
CONF="/etc/hashshashin/config.json"
SERVICE="hashshashin"
INSTALL_URL="https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh"

if [[ -t 1 && "${TERM:-}" != "dumb" ]]; then
  RESET='\033[0m'; BOLD='\033[1m'; DIM='\033[2m'
  RED='\033[38;5;196m'; GREEN='\033[38;5;82m'; YELLOW='\033[38;5;220m'
  CYAN='\033[38;5;45m'; BLUE='\033[38;5;75m'; PURPLE='\033[38;5;141m'; WHITE='\033[38;5;255m'; GRAY='\033[38;5;245m'
else
  RESET=''; BOLD=''; DIM=''; RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; PURPLE=''; WHITE=''; GRAY=''
fi

clear_screen(){ [[ -t 1 ]] && clear 2>/dev/null || true; }
line(){ printf '%b\n' "${GRAY}────────────────────────────────────────────────────────────────${RESET}"; }
pause(){ echo; read -r -p "  برای بازگشت Enter را بزنید... " _; }
confirm(){ local ans; read -r -p "  $1 [y/N]: " ans; [[ "$ans" =~ ^[Yy]$ ]]; }

need_root(){
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    if command -v sudo >/dev/null 2>&1; then
      exec sudo "$0" "$@"
    fi
    echo "Hashshashin Manager must run as root." >&2
    exit 1
  fi
}
need_root "$@"

banner(){
  printf '%b\n' "${CYAN}${BOLD}╔══════════════════════════════════════════════════════════════╗${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}              ${WHITE}${BOLD}H A S H S H A S H I N${RESET}                     ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}║${RESET}          ${GRAY}Professional L3 Tunnel Manager${RESET}                  ${CYAN}${BOLD}║${RESET}"
  printf '%b\n' "${CYAN}${BOLD}╚══════════════════════════════════════════════════════════════╝${RESET}"
  printf '%b\n' "              ${PURPLE}حشاشین • مدیریت حرفه‌ای تونل${RESET}"
}

role="-"; mode="-"; tun_name="hsh0"; tun_cidr="-"; tun_peer="-"; mtu="-"
public_interface="-"; public_ip="-"; foreign_public_ip="-"; iran_public_ip="-"
carrier_listen="-"; carrier_peer="-"; service_ports="-"

load_summary(){
  role="-"; mode="-"; tun_name="hsh0"; tun_cidr="-"; tun_peer="-"; mtu="-"
  public_interface="-"; public_ip="-"; foreign_public_ip="-"; iran_public_ip="-"
  carrier_listen="-"; carrier_peer="-"; service_ports="-"
  [[ -x "$BIN" && -f "$CONF" ]] || return 0
  while IFS='=' read -r key value; do
    case "$key" in
      role) role="$value";; mode) mode="$value";; tun_name) tun_name="$value";; tun_cidr) tun_cidr="$value";;
      tun_peer) tun_peer="$value";; mtu) mtu="$value";; public_interface) public_interface="$value";;
      public_ip) public_ip="$value";; foreign_public_ip) foreign_public_ip="$value";; iran_public_ip) iran_public_ip="$value";;
      carrier_listen) carrier_listen="$value";; carrier_peer) carrier_peer="$value";; service_ports) service_ports="$value";;
    esac
  done < <("$BIN" -summary -c "$CONF" 2>/dev/null || true)
}

service_state(){ systemctl is-active "$SERVICE" 2>/dev/null || true; }
service_enabled(){ systemctl is-enabled "$SERVICE" 2>/dev/null || true; }
state_badge(){
  case "$1" in
    active) printf '%b' "${GREEN}● ACTIVE${RESET}";;
    activating) printf '%b' "${YELLOW}● STARTING${RESET}";;
    failed) printf '%b' "${RED}● FAILED${RESET}";;
    inactive) printf '%b' "${YELLOW}● STOPPED${RESET}";;
    *) printf '%b' "${GRAY}● ${1^^}${RESET}";;
  esac
}

mode_label(){
  case "$mode" in
    full) echo "Full Tunnel";;
    direct-return) echo "Direct Return";;
    *) echo "$mode";;
  esac
}
role_label(){
  case "$role" in iran) echo "IRAN / Entry";; kharej) echo "KHAREJ / Exit";; *) echo "$role";; esac
}

human_bytes(){
  local n="${1:-0}"
  if command -v numfmt >/dev/null 2>&1; then numfmt --to=iec-i --suffix=B "$n" 2>/dev/null || echo "$n B"; else echo "$n B"; fi
}
iface_counter(){ local f="/sys/class/net/${tun_name}/statistics/$1"; [[ -r "$f" ]] && cat "$f" || echo 0; }

header_status(){
  load_summary
  local st ver rx tx
  st="$(service_state)"; ver="$($BIN -version 2>/dev/null | awk '{print $2}' || echo '-')"
  rx="$(human_bytes "$(iface_counter rx_bytes)")"; tx="$(human_bytes "$(iface_counter tx_bytes)")"
  line
  printf '  %-16s %b\n' "وضعیت سرویس:" "$(state_badge "$st")"
  printf '  %-16s %b     %-14s %b\n' "نسخه:" "${WHITE}${ver}${RESET}" "نقش:" "${WHITE}$(role_label)${RESET}"
  printf '  %-16s %b     %-14s %b\n' "حالت:" "${WHITE}$(mode_label)${RESET}" "اینترفیس:" "${WHITE}${tun_name}${RESET}"
  printf '  %-16s %b     %-14s %b\n' "Public IP:" "${WHITE}${public_ip}${RESET}" "MTU:" "${WHITE}${mtu}${RESET}"
  printf '  %-16s %b\n' "Service Ports:" "${WHITE}${service_ports}${RESET}"
  printf '  %-16s %b     %-14s %b\n' "Tunnel RX:" "${GREEN}${rx}${RESET}" "Tunnel TX:" "${CYAN}${tx}${RESET}"
  line
}

menu(){
  printf '%b\n' "  ${BOLD}${WHITE}مدیریت تونل${RESET}"
  printf '%b\n' "  ${CYAN}1)${RESET}  وضعیت و اطلاعات کامل        ${GRAY}Overview / routes / endpoints${RESET}"
  printf '%b\n' "  ${CYAN}2)${RESET}  Health Check                ${GRAY}بررسی سرویس، TUN، routing و carrier${RESET}"
  printf '%b\n' "  ${GREEN}3)${RESET}  Restart Tunnel              ${GRAY}راه‌اندازی مجدد سرویس${RESET}"
  printf '%b\n' "  ${GREEN}4)${RESET}  Start Tunnel                ${GRAY}اجرای سرویس${RESET}"
  printf '%b\n' "  ${YELLOW}5)${RESET}  Stop Tunnel                 ${GRAY}توقف سرویس${RESET}"
  printf '%b\n' "  ${PURPLE}6)${RESET}  Live Logs                   ${GRAY}لاگ زنده؛ خروج با Ctrl+C${RESET}"
  echo
  printf '%b\n' "  ${BOLD}${WHITE}تنظیمات و نگهداری${RESET}"
  printf '%b\n' "  ${BLUE}7)${RESET}  نمایش Config امن            ${GRAY}Shared Key مخفی می‌شود${RESET}"
  printf '%b\n' "  ${BLUE}8)${RESET}  Reconfigure / Repair        ${GRAY}اجرای دوباره Wizard نصب${RESET}"
  printf '%b\n' "  ${BLUE}9)${RESET}  Update Hashshashin          ${GRAY}آپدیت Core و Manager بدون حذف Config${RESET}"
  printf '%b\n' "  ${CYAN}10)${RESET} Network Diagnostics         ${GRAY}ip rule / routes / firewall / sockets${RESET}"
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
  printf '  Public interface : %s\n' "$public_interface"
  printf '  Public IP        : %s\n' "$public_ip"
  printf '  Kharej IP        : %s\n' "$foreign_public_ip"
  printf '  Iran IP          : %s\n' "$iran_public_ip"
  printf '  TUN              : %s (%s -> %s)\n' "$tun_name" "$tun_cidr" "$tun_peer"
  printf '  Carrier listen   : %s\n' "$carrier_listen"
  printf '  Carrier peer     : %s\n' "$carrier_peer"
  printf '  Service ports    : %s\n' "$service_ports"
  printf '  Service enabled  : %s\n' "$(service_enabled)"
  local since
  since="$(systemctl show "$SERVICE" -p ActiveEnterTimestamp --value 2>/dev/null || true)"
  [[ -n "$since" ]] && printf '  Active since      : %s\n' "$since"
  pause
}

check_item(){ local ok="$1" label="$2" detail="${3:-}"; if [[ "$ok" == 1 ]]; then printf '  %b %-28s %s\n' "${GREEN}✔${RESET}" "$label" "$detail"; else printf '  %b %-28s %s\n' "${RED}✘${RESET}" "$label" "$detail"; fi; }
health_check(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Health Check${RESET}"; line
  local st=0 cfg=0 tun=0 fwd=0 carrier=0 rules=0 port
  [[ "$(service_state)" == "active" ]] && st=1
  "$BIN" -check -c "$CONF" >/dev/null 2>&1 && cfg=1
  ip link show "$tun_name" >/dev/null 2>&1 && tun=1
  [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo 0)" == "1" ]] && fwd=1
  port="${carrier_listen##*:}"
  if command -v ss >/dev/null 2>&1 && [[ "$port" =~ ^[0-9]+$ ]]; then ss -H -lun 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]" && carrier=1; fi
  ip rule show 2>/dev/null | grep -q 'lookup 166' && rules=1
  check_item "$st" "Systemd service" "$(service_state)"
  check_item "$cfg" "Configuration" "$CONF"
  check_item "$tun" "TUN interface" "$tun_name"
  check_item "$fwd" "IPv4 forwarding" "net.ipv4.ip_forward=1"
  check_item "$carrier" "UDP carrier socket" "$carrier_listen"
  check_item "$rules" "Policy routing" "table 166"
  echo
  if [[ "$st$cfg$tun$fwd$carrier$rules" == "111111" ]]; then
    printf '%b\n' "  ${GREEN}${BOLD}Health: PASS${RESET} — بررسی‌های محلی سالم هستند."
  else
    printf '%b\n' "  ${YELLOW}${BOLD}Health: ATTENTION${RESET} — موارد قرمز را با Diagnostics و Logs بررسی کنید."
  fi
  printf '%b\n' "  ${GRAY}این تست سلامت local است و جای packet capture دو سرور را نمی‌گیرد.${RESET}"
  pause
}

service_action(){
  local action="$1"
  printf '%b\n' "${CYAN}  → systemctl ${action} ${SERVICE}${RESET}"
  if systemctl "$action" "$SERVICE"; then printf '%b\n' "${GREEN}  ✔ انجام شد.${RESET}"; else printf '%b\n' "${RED}  ✘ عملیات ناموفق بود.${RESET}"; fi
  sleep 1
}

live_logs(){ clear_screen; banner; echo; printf '%b\n' "${PURPLE}  Live Logs — برای بازگشت Ctrl+C بزنید.${RESET}"; echo; journalctl -u "$SERVICE" -f -n 80; }

show_config(){
  clear_screen; banner; echo; printf '%b\n' "${BOLD}  Configuration (Shared Key redacted)${RESET}"; line
  if [[ -f "$CONF" ]]; then
    sed -E 's/("key"[[:space:]]*:[[:space:]]*")[^"]+/***REDACTED***/' "$CONF"
  else
    printf '%b\n' "${RED}Config not found: ${CONF}${RESET}"
  fi
  pause
}

remote_installer(){
  local mode="$1"
  command -v curl >/dev/null 2>&1 || { printf '%b\n' "${RED}curl نصب نیست.${RESET}"; pause; return; }
  printf '%b\n' "${CYAN}  دریافت installer رسمی از GitHub...${RESET}"
  if [[ -n "$mode" ]]; then curl -fsSL "$INSTALL_URL" | bash -s -- "$mode"; else curl -fsSL "$INSTALL_URL" | bash; fi
}

network_diagnostics(){
  clear_screen; banner; load_summary
  printf '%b\n' "${BOLD}  Network Diagnostics${RESET}"
  line
  printf '%b\n' "${CYAN}[TUN]${RESET}"; ip -br addr show "$tun_name" 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Policy rules]${RESET}"; ip rule show 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Table 166 / data]${RESET}"; ip route show table 166 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Table 167 / carrier]${RESET}"; ip route show table 167 2>&1 || true
  echo; printf '%b\n' "${CYAN}[Carrier socket]${RESET}"; ss -lunp 2>/dev/null | grep -E "hashshashin|${carrier_listen##*:}" || true
  echo; printf '%b\n' "${CYAN}[Hashshashin iptables chains]${RESET}"
  for table in mangle nat filter; do iptables -t "$table" -S 2>/dev/null | grep 'HSH_' || true; done
  echo; printf '%b\n' "${CYAN}[Recent logs]${RESET}"; journalctl -u "$SERVICE" -n 25 --no-pager 2>/dev/null || true
  pause
}

reset_network(){
  clear_screen; banner; echo
  printf '%b\n' "${YELLOW}این عملیات سرویس را متوقف، ruleهای Hashshashin را پاک و دوباره سرویس را اجرا می‌کند.${RESET}"
  if ! confirm "ادامه می‌دهید؟"; then return; fi
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  "$BIN" -cleanup -c "$CONF" >/dev/null 2>&1 || true
  if systemctl start "$SERVICE"; then printf '%b\n' "${GREEN}  ✔ Network state rebuilt.${RESET}"; else printf '%b\n' "${RED}  ✘ سرویس بالا نیامد؛ Logs را بررسی کنید.${RESET}"; fi
  pause
}

uninstall_menu(){
  clear_screen; banner; echo
  printf '%b\n' "${RED}${BOLD}هشدار: Hashshashin حذف خواهد شد.${RESET}"
  printf '%b\n' "${GRAY}فایل config طبق سیاست installer برای بازیابی نگه داشته می‌شود.${RESET}"
  confirm "حذف انجام شود؟" || return
  remote_installer "--uninstall"
  exit 0
}

main_loop(){
  while true; do
    clear_screen; banner
    if [[ ! -x "$BIN" || ! -f "$CONF" ]]; then
      echo; printf '%b\n' "${RED}Hashshashin installation/config is incomplete.${RESET}"
      printf '%b\n' "${GRAY}Official installer را دوباره اجرا کنید.${RESET}"; exit 1
    fi
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
      8) clear_screen; banner; remote_installer "--reconfigure"; pause;;
      9) clear_screen; banner; remote_installer "--update"; pause;;
      10) network_diagnostics;;
      11) reset_network;;
      12) uninstall_menu;;
      0|q|Q) clear_screen; exit 0;;
      *) printf '%b\n' "${RED}  انتخاب نامعتبر.${RESET}"; sleep 1;;
    esac
  done
}

main_loop
