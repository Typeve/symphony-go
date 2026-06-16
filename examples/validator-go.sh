#!/usr/bin/env bash
set -u

workspace=${SYMPHONY_WORKSPACE:-}
verdict_path=${SYMPHONY_VERDICT_PATH:-}

if [[ -z "$workspace" || -z "$verdict_path" ]]; then
  echo "SYMPHONY_WORKSPACE and SYMPHONY_VERDICT_PATH are required" >&2
  exit 2
fi

mkdir -p "$(dirname "$verdict_path")"
cd "$workspace"

command_text="go test -mod=readonly -count=1 ./..."
status="pass"
summary="Go 测试全部通过。"
command_status="pass"
command_summary="全部通过。"

if ! command -v go >/dev/null 2>&1; then
  status="blocked"
  summary="未找到 Go 工具链，无法完成验证。"
  command_status="blocked"
  command_summary="未找到 Go 工具链。"
elif ! go test -mod=readonly -count=1 ./...; then
  status="fail"
  summary="Go 测试未通过，请根据测试失败信息修复后重试。"
  command_status="fail"
  command_summary="测试未通过。"
fi

cat > "$verdict_path" <<JSON
{
  "status": "$status",
  "summary": "$summary",
  "findings": [],
  "commands": [
    {
      "command": "$command_text",
      "status": "$command_status",
      "summary": "$command_summary"
    }
  ],
  "risks": [],
  "suggested_rework_prompt": ""
}
JSON
