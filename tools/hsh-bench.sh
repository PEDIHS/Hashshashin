#!/usr/bin/env bash
set -Eeuo pipefail

PORT_DEFAULT=39001
RUNS_DEFAULT=5
PARALLEL_DEFAULT=8
DURATION_DEFAULT=15
WARMUP_DEFAULT=5
CONF="/etc/hashshashin/config.json"
BIN="/usr/local/bin/hashshashin"

fail(){ echo "[hsh-bench] ERROR: $*" >&2; exit 1; }
need(){ command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"; }
valid_port(){ [[ "$1" =~ ^[0-9]+$ ]] && (( $1 >= 1 && $1 <= 65535 )); }
valid_uint(){ [[ "$1" =~ ^[0-9]+$ ]] && (( $1 >= 1 )); }

usage(){
  cat <<'EOF'
Hashshashin repeatable benchmark

  hsh-bench server [port]
      Start a persistent iperf3 server and verify that the port is listening.

  hsh-bench client HOST [port] [runs] [parallel]
      Warm up, then run repeated forward and reverse tests using parallel TCP
      streams. Defaults: port=39001 runs=5 parallel=8 duration=15s.

The client stores raw iperf3 JSON under /tmp/hashshashin-bench-<timestamp>/
and prints median/min/max plus hsh0 drop counters before and after the run.
EOF
}

iface_stat(){
  local stat="$1" f="/sys/class/net/hsh0/statistics/$1"
  [[ -r "$f" ]] && cat "$f" || echo 0
}

show_host_context(){
  echo "== host context =="
  echo "time=$(date -Is 2>/dev/null || date)"
  echo "hostname=$(hostname)"
  echo "kernel=$(uname -r)"
  echo "cpu_logical=$(nproc 2>/dev/null || echo unknown)"
  if [[ -x "$BIN" ]]; then "$BIN" -version 2>/dev/null || true; fi
  if [[ -x "$BIN" && -f "$CONF" ]]; then
    "$BIN" -summary -c "$CONF" 2>/dev/null | grep -E '^(role|mode|transport|mtu|performance_profile|tun_queues|receive_workers|tx_queue_len|qdisc|socket_buffer|smart_return)=' || true
  fi
  if ip link show hsh0 >/dev/null 2>&1; then
    ip -details link show hsh0 2>/dev/null | head -n 2 || true
    if command -v tc >/dev/null 2>&1; then tc qdisc show dev hsh0 2>/dev/null || true; fi
  fi
  echo "hsh0_tx_dropped=$(iface_stat tx_dropped)"
  echo "hsh0_rx_dropped=$(iface_stat rx_dropped)"
  echo
}

server(){
  need iperf3; need ss
  local port="${1:-$PORT_DEFAULT}"
  valid_port "$port" || fail "invalid port: $port"

  if ss -H -ltn 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]"; then
    echo "[hsh-bench] TCP/$port is already listening; leaving the existing listener untouched."
    echo "[hsh-bench] if this is not iperf3, choose another benchmark port."
  else
    echo "[hsh-bench] starting persistent iperf3 server on TCP/$port"
    iperf3 -s -D -p "$port"
  fi

  for _ in 1 2 3 4 5; do
    if ss -H -ltn 2>/dev/null | grep -Eq "[:.]${port}[[:space:]]"; then
      echo "[hsh-bench] listener ready: TCP/$port"
      echo "[hsh-bench] keep this port allowed only from the benchmark peer while testing."
      return 0
    fi
    sleep 1
  done
  fail "iperf3 did not become ready on TCP/$port"
}

wait_peer(){
  local host="$1" port="$2" attempt
  echo "[hsh-bench] preflight $host:$port"
  for attempt in 1 2 3 4 5; do
    if python3 - "$host" "$port" >/dev/null 2>&1 <<'PY'
import socket, sys
with socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=3):
    pass
PY
    then
      echo "[hsh-bench] peer is reachable (attempt $attempt/5)"
      return 0
    fi
    echo "[hsh-bench] peer not ready (attempt $attempt/5); retrying..." >&2
    sleep "$attempt"
  done
  fail "connection refused/unreachable at $host:$port after 5 attempts; run 'hsh-bench server $port' on the peer and check its firewall"
}

run_one(){
  local host="$1" port="$2" parallel="$3" seconds="$4" reverse="$5" output="$6"
  local -a args=(-c "$host" -p "$port" -P "$parallel" -t "$seconds" -O 3 -J)
  [[ "$reverse" == 1 ]] && args+=(-R)
  iperf3 "${args[@]}" >"$output"
}

summarize(){
  local dir="$1" runs="$2" parallel="$3" duration="$4" tx_before="$5" tx_after="$6" rx_before="$7" rx_after="$8"
  python3 - "$dir" "$runs" "$parallel" "$duration" "$tx_before" "$tx_after" "$rx_before" "$rx_after" <<'PY'
import json, os, statistics, sys
path, runs, parallel, duration, tx0, tx1, rx0, rx1 = sys.argv[1:]
runs, parallel, duration = map(int, (runs, parallel, duration))
tx0, tx1, rx0, rx1 = map(int, (tx0, tx1, rx0, rx1))

def bps(file):
    with open(file, 'r', encoding='utf-8') as f:
        j = json.load(f)
    err = j.get('error')
    if err:
        raise RuntimeError(f"{os.path.basename(file)}: {err}")
    end = j.get('end', {})
    for key in ('sum_received', 'sum_sent', 'sum'):
        v = end.get(key, {}).get('bits_per_second')
        if isinstance(v, (int, float)):
            return float(v) / 1_000_000
    raise RuntimeError(f"{os.path.basename(file)}: bits_per_second missing")

def stats(values):
    return statistics.median(values), min(values), max(values), statistics.pstdev(values) if len(values) > 1 else 0.0

forward = [bps(os.path.join(path, f'run-{i:02d}-forward.json')) for i in range(1, runs+1)]
reverse = [bps(os.path.join(path, f'run-{i:02d}-reverse.json')) for i in range(1, runs+1)]
fm, fmin, fmax, fsd = stats(forward)
rm, rmin, rmax, rsd = stats(reverse)
print('\n== benchmark summary ==')
print(f'runs={runs} parallel_streams={parallel} duration_each={duration}s')
print('forward_mbit=' + ', '.join(f'{x:.1f}' for x in forward))
print(f'forward median={fm:.1f} min={fmin:.1f} max={fmax:.1f} stdev={fsd:.1f} Mbit/s')
print('reverse_mbit=' + ', '.join(f'{x:.1f}' for x in reverse))
print(f'reverse median={rm:.1f} min={rmin:.1f} max={rmax:.1f} stdev={rsd:.1f} Mbit/s')
print(f'hsh0_tx_dropped_delta={max(0, tx1-tx0)}')
print(f'hsh0_rx_dropped_delta={max(0, rx1-rx0)}')
spread_f = ((fmax-fmin)/fm*100) if fm else 0
spread_r = ((rmax-rmin)/rm*100) if rm else 0
print(f'forward_spread={spread_f:.1f}% reverse_spread={spread_r:.1f}%')
if spread_f > 25 or spread_r > 25:
    print('WARNING: run-to-run spread >25%; result is not stable enough for a single headline number.')
if tx1 > tx0 or rx1 > rx0:
    print('WARNING: hsh0 drops increased during the test; the data-plane/queue is still under pressure.')
print(f'raw_results={path}')
PY
}

client(){
  need iperf3; need python3; need ip
  local host="${1:-}" port="${2:-$PORT_DEFAULT}" runs="${3:-$RUNS_DEFAULT}" parallel="${4:-$PARALLEL_DEFAULT}"
  [[ -n "$host" ]] || fail "HOST is required"
  valid_port "$port" || fail "invalid port: $port"
  valid_uint "$runs" || fail "runs must be >=1"
  valid_uint "$parallel" || fail "parallel must be >=1"
  (( parallel <= 64 )) || fail "parallel must be <=64"

  wait_peer "$host" "$port"
  local stamp dir tx0 tx1 rx0 rx1 i
  stamp="$(date +%Y%m%d-%H%M%S)"
  dir="/tmp/hashshashin-bench-${stamp}"
  mkdir -p "$dir"

  show_host_context | tee "$dir/context.txt"
  tx0="$(iface_stat tx_dropped)"; rx0="$(iface_stat rx_dropped)"

  echo "[hsh-bench] warmup forward ${WARMUP_DEFAULT}s + reverse ${WARMUP_DEFAULT}s (not counted)"
  run_one "$host" "$port" "$parallel" "$WARMUP_DEFAULT" 0 "$dir/warmup-forward.json"
  run_one "$host" "$port" "$parallel" "$WARMUP_DEFAULT" 1 "$dir/warmup-reverse.json"

  for ((i=1; i<=runs; i++)); do
    printf '[hsh-bench] measured run %d/%d forward...\n' "$i" "$runs"
    run_one "$host" "$port" "$parallel" "$DURATION_DEFAULT" 0 "$(printf '%s/run-%02d-forward.json' "$dir" "$i")"
    sleep 2
    printf '[hsh-bench] measured run %d/%d reverse...\n' "$i" "$runs"
    run_one "$host" "$port" "$parallel" "$DURATION_DEFAULT" 1 "$(printf '%s/run-%02d-reverse.json' "$dir" "$i")"
    sleep 2
  done

  tx1="$(iface_stat tx_dropped)"; rx1="$(iface_stat rx_dropped)"
  summarize "$dir" "$runs" "$parallel" "$DURATION_DEFAULT" "$tx0" "$tx1" "$rx0" "$rx1" | tee "$dir/summary.txt"
}

case "${1:-}" in
  server) shift; server "$@";;
  client) shift; client "$@";;
  -h|--help|help|"") usage;;
  *) fail "unknown command: $1 (use --help)";;
esac
