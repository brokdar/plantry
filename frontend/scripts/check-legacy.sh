#!/usr/bin/env bash
# Phase 2 §0.2 gate 6, frontend half: none of these patterns may appear in
# src/ or e2e/ (.ts, .tsx, .css). The locale JSON is covered by
# src/lib/i18n/keys.test.ts instead.
set -euo pipefail
cd "$(dirname "$0")/.."

patterns=(
  'LegacyPortions|QuantityFromLegacyPortions|plate-workflow-rework'
  'cook_count|last_cooked|MarkCooked|MostCooked|Forgotten|foods/insights|useInsights'
  'fill_empty|replace_all|aiFill|RevertBanner|AiFilledBadge|ChatMode'
  'WeekID|week_id|weekKeys|week_starts_on|fixed_weekday'
  'ReadOnlyPlannerGrid|MobilePlannerGrid|PlannerGrid|SlotSheet|ComponentTraySheet|components/planner|components/calendar'
  '"/components|"/ingredients|"/import|"/presets|"/day/|"/archive|"/calendar'
  'LEGACY_SETTINGS_TABS|\?tab='
  'surface-container|on-surface|outline-variant|primary-container|on-primary|secondary-container|tertiary'
  'gradient-primary|editorial-shadow|glass-header|btn-shimmer|ai-accent|macro-|chart-'
  'PortionStepper|seedTemplate|/api/templates'
  'plan\.generate_prompt|openWith\(t\("plan\.generate'
  'locked_by|is_locked|isLocked'
  'components/chat|ChatPanel|ToolCallBlock|chat-tool-call|useChatUI|chat-ui|chat-stream|chatEvents|plate_changed'
)

status=0
for p in "${patterns[@]}"; do
  rc=0
  grep -rnE --include='*.ts' --include='*.tsx' --include='*.css' -e "$p" src e2e || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "legacy pattern found: $p" >&2
    status=1
  elif [ "$rc" -gt 1 ]; then
    echo "grep failed ($rc) for: $p" >&2
    status=1
  fi
done
exit "$status"
