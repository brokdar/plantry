package testhelper

import (
	"database/sql"
	"strconv"
	"testing"
)

// ReportNow is "now" of the report fixture: 2026-11-28 12:00 UTC.
const ReportNow = "2026-11-28 12:00:00"

// SeedReport fills a migrated database with a month of Chef activity for the
// `plantry report` tests. The window of the tests is 30 days back from
// ReportNow; one chef_help run and one find lie outside it. The secret and
// the typed text in it must never show up in a report.
func SeedReport(t testing.TB, db *sql.DB) {
	t.Helper()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed report: %v\n%s", err, q)
		}
		id, _ := res.LastInsertId()
		return id
	}
	slot := exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	food := func(name string, trial, archived int) int64 {
		return exec(`INSERT INTO foods (name, kind, role, reference_portions, trial, archived) VALUES (?, 'composed', 'main', 2, ?, ?)`, name, trial, archived)
	}
	cooked, uncooked, removed, plain := food("Dal", 1, 0), food("Soup", 0, 0), food("Stew", 1, 1), food("Chili", 0, 0)
	session := func(f int64, cookedAt any) int64 {
		return exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, cooked_at) VALUES (?, '2026-11-20', ?, 2, ?)`, f, slot, cookedAt)
	}
	sCooked := session(cooked, "2026-11-20 18:00:00")
	session(uncooked, nil)
	sPlain := session(plain, "2026-11-21 18:00:00")
	_ = removed

	run := func(kind, status, startedAt string, durMs any, usd float64, extra ...any) int64 {
		scope, chooser, key := any(nil), any(nil), any(nil)
		if len(extra) > 0 {
			scope = extra[0]
		}
		if len(extra) > 1 {
			chooser = extra[1]
		}
		if status == "error" {
			key = "error.ai.provider_error"
		}
		return exec(`INSERT INTO ai_runs (kind, status, started_at, duration_ms, cost_usd, scope_json, chooser, error_key, provider, model, instruction)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'fake', 'fake-model', 'TYPEDTEXT')`, kind, status, startedAt, durMs, usd, scope, chooser, key)
	}
	help := func(startedAt string, dur int, status string) int64 {
		return run("chef_help", status, startedAt, dur, 0.01, `{"session_id":`+strconv.FormatInt(sCooked, 10)+`,"kind":"explain_step"}`)
	}
	h1, h2 := help("2026-11-10 10:00:00", 2000, "ok"), help("2026-11-11 10:00:00", 4000, "ok")
	help("2026-11-12 10:00:00", 12000, "ok")
	help("2026-11-13 10:00:00", 1000, "error")
	help("2026-09-01 10:00:00", 99000, "ok") // outside the window
	exec(`INSERT INTO chef_help_verdicts (run_id, food_id, kind, verdict, created_at, updated_at) VALUES (?, ?, 'explain_step', 'helped', '2026-11-10 10:01:00', '2026-11-10 10:01:00')`, h1, cooked)
	exec(`INSERT INTO chef_help_verdicts (run_id, food_id, kind, verdict, created_at, updated_at) VALUES (?, ?, 'explain_step', 'not_helped', '2026-11-11 10:01:00', '2026-11-11 10:01:00')`, h2, cooked)
	run("chef_find", "ok", "2026-11-14 09:00:00", 30000, 4.2)
	run("untracked", "ok", "2026-11-15 09:00:00", nil, 0)
	ic1 := run("chef_import_check", "ok", "2026-11-16 09:00:00", 5000, 0.02)
	run("chef_import_check", "ok", "2026-11-17 09:00:00", 6000, 0.02)
	exec(`INSERT INTO import_check_outcomes (run_id, food_id, kept, undone, touch, flags, created_at) VALUES (?, ?, '["times","keep"]', '["role"]', 'added', 2, '2026-11-16 09:05:00')`, ic1, plain)

	gen := run("generate", "ok", "2026-11-18 09:00:00", 8000, 0, nil, "heuristic")
	step := func(runID, seq int64, phase, issues string) {
		exec(`INSERT INTO ai_run_steps (run_id, seq, type, name, args_json, result_json) VALUES (?, ?, 'tool_call', 'plan.evaluation', ?, ?)`,
			runID, seq, `{"phase":"`+phase+`"}`, `{"issues":`+issues+`}`)
	}
	step(gen, 1, "before", `[{"code":"missing_plate"},{"code":"missing_plate"},{"code":"missing_plate"}]`)
	step(gen, 2, "after", `[{"code":"kcal_below_floor"},{"code":"kcal_below_floor"},{"code":"too_many"}]`)
	run("regenerate", "ok", "2026-11-19 09:00:00", 7000, 0, nil, "llm")

	suggestion := func(status string, reason, foodID any, createdAt string) {
		exec(`INSERT INTO recipe_suggestions (mode, source_url, draft_json, annotations_json, grounded, status, reason, food_id, created_at, state)
			VALUES ('find', 'https://example.test/r', '{}', '{}', 1, ?, ?, ?, ?, ?)`, status, reason, foodID, createdAt,
			map[string]any{"accepted": "in_library", "rejected": nil}[status])
	}
	suggestion("accepted", nil, cooked, "2026-11-12 08:00:00")
	suggestion("accepted", nil, uncooked, "2026-11-13 08:00:00")
	suggestion("rejected", "too_long", nil, "2026-11-14 08:00:00")
	suggestion("rejected", "taste", nil, "2026-09-01 08:00:00") // outside the window

	plate := func(date string, f, sess int64, rating string) {
		p := exec(`INSERT INTO plates (slot_id, date, created_at, updated_at) VALUES (?, ?, ?, ?)`, slot, date, date+" 08:00:00", date+" 08:00:00")
		exec(`INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id) VALUES (?, ?, 1, ?)`, p, f, sess)
		exec(`INSERT INTO plate_feedback (plate_id, status, rated_at) VALUES (?, ?, ?)`, p, rating, date+" 20:00:00")
	}
	plate("2026-11-20", cooked, sCooked, "loved")
	plate("2026-11-21", plain, sPlain, "disliked")

	exec(`INSERT INTO chef_tips (kind, dimension, change, state, window_start, created_at, updated_at) VALUES ('swap', 'protein', '{}', 'applied', '2026-11-16', '2026-11-16 09:00:00', '2026-11-16 09:00:00')`)
	exec(`INSERT INTO chef_tips (kind, dimension, change, state, window_start, created_at, updated_at) VALUES ('swap', 'protein', '{}', 'dismissed', '2026-11-16', '2026-11-17 09:00:00', '2026-11-17 09:00:00')`)
	for i := 0; i < 3; i++ {
		exec(`INSERT INTO chef_jobs (kind, status, error, created_at, updated_at) VALUES ('chef_find', 'failed', 'boom', '2026-11-20 09:00:00', '2026-11-20 09:00:00')`)
	}
	exec(`INSERT INTO chef_jobs (kind, status, created_at, updated_at) VALUES ('chef_attributes', 'done', '2026-11-20 09:00:00', '2026-11-20 09:00:00')`)

	exec(`INSERT INTO app_settings (key, value, encrypted) VALUES ('chef.enabled', 'true', 0)`)
	exec(`INSERT INTO app_settings (key, value, encrypted) VALUES ('ai.model', 'fake-model', 0)`)
	exec(`INSERT INTO app_settings (key, value, encrypted) VALUES ('ai.api_key', 'SECRETKEYVALUE', 1)`)
}
