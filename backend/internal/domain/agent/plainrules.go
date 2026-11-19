package agent

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// plainRules rewrites a tool result for the model: every issue object (one
// with a "rule") loses its rule and code and gets a plain "label" and one
// plain "message" instead, and the keys of a "dropped" count ("H9.code")
// become labels. The model repeats what it reads and the user cannot read
// rule codes; the HTTP API keeps its codes. A result without issues is
// returned as it is.
func plainRules(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return raw
	}
	out, err := json.Marshal(plainWalk(v, ""))
	if err != nil {
		return raw
	}
	return out
}

// proseKeys are the result fields that hold the tools' own sentences; names
// of foods and other user data are never rewritten, and neither is the
// report, which is written in kitchen words already.
var proseKeys = map[string]bool{
	"message": true, "why": true, "hint": true, "dry_run_note": true, "no_fix_reason": true, "auto_fix": true,
	"issues_unchecked_reason": true, "evaluation_error": true, "warning": true, "error": true, "reason": true, "what_to_do": true, "note_to_model": true,
}

func plainWalk(v any, key string) any {
	switch x := v.(type) {
	case string:
		if key == "state" || key == "severity" || key == "source" {
			if to, ok := plainValues[x]; ok {
				return to
			}
		}
		if key == "reason" {
			if to, ok := plainReasons[x]; ok {
				return to
			}
		}
		if proseKeys[key] {
			return plainWords(x)
		}
		return x
	case []any:
		for i := range x {
			x[i] = plainWalk(x[i], key)
		}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if k == "dropped" {
				if m, ok := e.(map[string]any); ok {
					out[k] = plainDropped(m)
					continue
				}
			}
			if k == "report" {
				out[k] = e
				continue
			}
			name := plainIdent(k)
			out[name] = plainWalk(e, k)
		}
		if _, ok := out["rule"].(string); ok {
			code, _ := out["code"].(string)
			t := planning.PlainText(code)
			delete(out, "rule")
			delete(out, "code")
			out["label"], out["message"] = plainWords(t.Label), plainWords(t.Message)
		}
		return out
	}
	return v
}

// plainDropped renames the "rule.code" keys of alternatives' dropped counts
// (a key may also be a bare reason such as nutrition_missing).
func plainDropped(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, n := range m {
		name := k
		if _, code, ok := strings.Cut(k, "."); ok {
			name = planning.PlainText(code).Label
		} else if k == "nutrition_missing" {
			name = "missing nutrition data"
		}
		out[name] = n
	}
	return out
}

// kitchenWords are the planner's words as the chat model and the user read
// them: range, minimum, limit, warning, problem, batch, "not accepted yet".
// They are applied to everything the model reads (results, descriptions, the
// prompt, errors) so no planner word reaches a reply. Identifiers such as
// session_id are not words and stay: they are ids the model passes back.
var kitchenWords = []struct {
	re *regexp.Regexp
	to string
}{
	{regexp.MustCompile(`\bviolations,warnings,soft\b`), "problems,warnings,advice"},
	{regexp.MustCompile(`\b(?i:hard violations?)\b`), "problem"},
	{regexp.MustCompile(`\bviolations\b`), "problems"},
	{regexp.MustCompile(`\bViolations\b`), "Problems"},
	{regexp.MustCompile(`\b(?i:violation)\b`), "problem"},
	{regexp.MustCompile(`\bhard issues\b`), "problems"},
	{regexp.MustCompile(`\bhard issue\b`), "problem"},
	{regexp.MustCompile(`\bhard rules\b`), "required rules"},
	{regexp.MustCompile(`\bHard rules\b`), "Required rules"},
	{regexp.MustCompile(`\bhard\b`), "firm"},
	{regexp.MustCompile(`\bsoft issues\b`), "warnings"},
	{regexp.MustCompile(`\bsoft issue\b`), "warning"},
	{regexp.MustCompile(`\bSoft issues\b`), "Warnings"},
	{regexp.MustCompile(`\bSoft rules\b`), "Warnings"},
	{regexp.MustCompile(`\bsoft rules\b`), "warnings"},
	{regexp.MustCompile(`\b(?i:soft)\b`), "warning"},
	{regexp.MustCompile(`\bfloor_kcal\b`), "minimum_kcal"},
	{regexp.MustCompile(`\b(?:kcal|safety) floors?\b`), "minimum"},
	{regexp.MustCompile(`\bfloors\b`), "minimums"},
	{regexp.MustCompile(`\bFloor\b`), "Minimum"},
	{regexp.MustCompile(`\bfloor\b`), "minimum"},
	{regexp.MustCompile(`\bband_(low|high)`), "range_$1"},
	{regexp.MustCompile(`\bcook_occasions`), "cooking_times"},
	{regexp.MustCompile(`\b(?i:cooking occasions)\b`), "cooking times"},
	{regexp.MustCompile(`\b(?i:cooking occasion)\b`), "cooking time"},
	{regexp.MustCompile(`\bAdd-ons\b`), "Sides"},
	{regexp.MustCompile(`\badd-ons\b`), "sides"},
	{regexp.MustCompile(`\bAdd-on\b`), "Side"},
	{regexp.MustCompile(`\badd-on\b`), "side"},
	{regexp.MustCompile(`\b(?i:hearty minimum)\b`), "least a filling meal holds"},
	{regexp.MustCompile(`\breached_hearty\b`), "reached_filling"},
	{regexp.MustCompile(`\bHearty\b`), "Filling"},
	{regexp.MustCompile(`\bhearty\b`), "filling"},
	{regexp.MustCompile(`\breached_band\b`), "reached_range"},
	{regexp.MustCompile(`\breaches_cap\b`), "reaches_limit"},
	{regexp.MustCompile(`\bbands\b`), "ranges"},
	{regexp.MustCompile(`\bband\b`), "range"},
	{regexp.MustCompile(`\bBand\b`), "Range"},
	{regexp.MustCompile(`\blight caps?\b`), "limit"},
	{regexp.MustCompile(`\bcaps\b`), "limits"},
	{regexp.MustCompile(`\bcap\b`), "limit"},
	{regexp.MustCompile(`\bCap\b`), "Limit"},
	{regexp.MustCompile(`\bdrafts\b`), "pending plans"},
	{regexp.MustCompile(`\bDraft\b`), "Pending:"},
	{regexp.MustCompile(`\bdraft\b`), "pending"},
	{regexp.MustCompile(`\bsessions\b`), "batches"},
	{regexp.MustCompile(`\bSessions\b`), "Batches"},
	{regexp.MustCompile(`\bsession\b`), "batch"},
	{regexp.MustCompile(`\bSession\b`), "Batch"},
}

// passBack are the result fields the model sends back to a tool unchanged
// (ops, light and goal of a way, a cook_at cell, a move target, the cells of
// an away op): they keep ISO dates in modelDates.
var passBack = map[string]bool{"ops": true, "op": true, "light": true, "goal": true, "cook_at": true, "to": true, "cells": true, "pulse": true, "occupant": true}

// modelDates is a tool result as the model reads it: every ISO date outside
// passBack fields becomes "Wed 25 Nov". The run log and the audit keep the
// result as the tool produced it (machine dates); only the copy in the
// conversation changes.
func modelDates(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return raw
	}
	out, err := json.Marshal(datesWalk(v, "", false))
	if err != nil {
		return raw
	}
	return out
}

// datesWalk is the walk of modelDates: days become labels outside passBack
// fields, and the hearty level reads as "filling" everywhere (the model sends
// "filling" back; Execute maps it to the stored "hearty").
func datesWalk(v any, key string, pass bool) any {
	switch x := v.(type) {
	case string:
		if key == "level" || key == "light_level" || key == "intent" {
			if x == "hearty" {
				return "filling"
			}
		}
		if key == "light_meals" {
			x = strings.ReplaceAll(x, "hearty", "filling")
		}
		if pass {
			return x
		}
		return plainDates(x)
	case []any:
		for i := range x {
			x[i] = datesWalk(x[i], key, pass)
		}
	case map[string]any:
		for k, e := range x {
			name := k
			if k == "reached_hearty" {
				name = "reached_filling"
				delete(x, k)
			}
			x[name] = datesWalk(e, name, pass || passBack[k])
		}
	}
	return v
}

// fillingToHearty maps the model's level "filling" back to the stored
// "hearty" in a tool input.
func fillingToHearty(raw json.RawMessage) json.RawMessage {
	if !bytes.Contains(raw, []byte(`"filling"`)) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return raw
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				if s, ok := e.(string); ok && k == "level" && s == "filling" {
					x[k] = "hearty"
					continue
				}
				walk(e)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

var isoDay = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

// plainDates replaces every ISO date in a text with the day as people say it
// ("Wed 25 Nov"), weekday included: the model repeats the dates it reads and
// computes weekdays wrongly. Other helpers of the package (the guard's texts)
// call it too.
func plainDates(s string) string {
	return isoDay.ReplaceAllStringFunc(s, func(d string) string {
		if t, err := time.Parse(time.DateOnly, d); err == nil {
			return dayLabel(t)
		}
		return d
	})
}

// plainWords rewrites a text with kitchenWords.
func plainWords(s string) string {
	s = plainDates(s)
	for _, w := range kitchenWords {
		s = w.re.ReplaceAllString(s, w.to)
	}
	return s
}

// plainKeys renames the model-facing result keys that carry planner words.
var plainKeys = map[string]string{
	"sessions": "batches", "draft": "pending", "floor_kcal": "minimum_kcal",
	"band_low": "range_low", "band_high": "range_high", "reached_band": "reached_range", "reaches_cap": "reaches_limit", "violations": "problems", "soft": "advice",
}

// plainReasons are the reason values of an extras issue, as words.
var plainReasons = map[string]string{
	"light_meal": "a side on a light meal", "duplicates_main": "already in the main", "over_cap": "over the usual serving",
	"day_limit": "too many sides that day", "repeated": "the same side on too many meals",
}

var identParts = []struct {
	re *regexp.Regexp
	to string
}{
	{regexp.MustCompile(`(^|_)band(_|$)`), "${1}range${2}"},
	{regexp.MustCompile(`(^|_)floor(_|$)`), "${1}minimum${2}"},
	{regexp.MustCompile(`(^|_)cap(_|$)`), "${1}limit${2}"},
	{regexp.MustCompile(`^cook_occasions`), "cooking_times"},
}

// plainIdent renames a result key: the exact renames of plainKeys, else the
// planner words inside it (below_band, floor_kcal, band_low_pct,
// cook_occasions_day).
func plainIdent(k string) string {
	if to, ok := plainKeys[k]; ok {
		return to
	}
	for _, p := range identParts {
		k = p.re.ReplaceAllString(k, p.to)
	}
	return k
}

// plainValues renames enum values that carry planner words.
var plainValues = map[string]string{"draft": "pending", "violation": "problem", "soft": "warning", "session": "batch"}

// plainError keeps an error's identity and says it in kitchen words.
type plainError struct{ err error }

func (e plainError) Error() string { return plainWords(e.err.Error()) }
func (e plainError) Unwrap() error { return e.err }

// dropAfterWeek removes the week ledger from the "after" block of a result
// that carries a report: the report holds what the change did to the week, and
// the model recited the rest (weekly fat of an unrelated 40 %) as news.
func dropAfterWeek(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj["report"] == nil || obj["after"] == nil {
		return raw
	}
	var after map[string]json.RawMessage
	if json.Unmarshal(obj["after"], &after) != nil || after["week"] == nil {
		return raw
	}
	delete(after, "week")
	b, err := json.Marshal(after)
	if err != nil {
		return raw
	}
	obj["after"] = b
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}
