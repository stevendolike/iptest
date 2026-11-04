#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$root"

log() {
    printf '[%s] [%s] %s\n' "$(date +'%Y-%m-%d %H:%M:%S')" "$1" "$2"
}

run_step() {
    local name=$1
    shift
    log INFO "开始执行 $name"
    if "$@"; then
        log INFO "$name 执行成功"
    else
        local code=$?
        log ERROR "$name 执行失败"
        return "$code"
    fi
}

# Reserve the pipeline's filenames: otherwise a custom output could leave an
# old ip_tq.csv behind for the final stage to consume.
for arg in "$@"; do
    case "$arg" in
        -file*|--file*|-outfile*|--outfile*)
            log ERROR "批量管线不允许覆盖 -file / -outfile"
            exit 1
            ;;
    esac
done

if [ ! -f locations.json ]; then
    run_step data.js node data.js
fi
run_step ip_init.js node ip_init.js
run_step iptest.go go run iptest.go -file ip.txt -outfile ip_tq.csv -max 20 -speedtest 5 "$@"
run_step ip_tq.js node ip_tq.js ip_tq.csv
log INFO "所有脚本执行完毕"
