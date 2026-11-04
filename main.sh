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
value_flag=''
for arg in "$@"; do
    if [ -n "$value_flag" ]; then
        case "$arg" in
            -*)
                log ERROR "测试参数缺少值: $value_flag (下一项为选项: $arg)"
                exit 1
                ;;
        esac
        value_flag=''
        continue
    fi
    case "$arg" in
        -file*|--file*|-outfile*|--outfile*)
            log ERROR "批量管线不允许覆盖 -file / -outfile"
            exit 1
            ;;
        -max|--max|-speedtest|--speedtest|-url|--url|-delay|--delay|-countries|--countries|-speedLimit|--speedLimit|-ipapi-url|--ipapi-url|-asn-db|--asn-db|-country-db|--country-db|-asn-url|--asn-url|-country-url|--country-url)
            value_flag=$arg
            ;;
        -max=*|--max=*|-speedtest=*|--speedtest=*|-url=*|--url=*|-delay=*|--delay=*|-countries=*|--countries=*|-speedLimit=*|--speedLimit=*|-tls|--tls|-tls=*|--tls=*|-fast|--fast|-fast=*|--fast=*|-ips|--ips|-ips=*|--ips=*|-download-db|--download-db|-download-db=*|--download-db=*|-ipapi-url=*|--ipapi-url=*|-asn-db=*|--asn-db=*|-country-db=*|--country-db=*|-asn-url=*|--asn-url=*|-country-url=*|--country-url=*)
            ;;
        -*)
            log ERROR "不支持的测试参数或位置参数标记: $arg"
            exit 1
            ;;
        *)
            log ERROR "不支持位置参数: $arg"
            exit 1
            ;;
    esac
done
if [ -n "$value_flag" ]; then
    log ERROR "测试参数缺少值: $value_flag"
    exit 1
fi

if [ ! -f locations.json ]; then
    run_step data.js node data.js
fi
run_step ip_init.js node ip_init.js
run_step iptest.go go run iptest.go -file ip.txt -outfile ip_tq.csv -max 20 -speedtest 5 "$@"
run_step ip_tq.js node ip_tq.js ip_tq.csv
log INFO "所有脚本执行完毕"
