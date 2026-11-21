#!/usr/bin/env python3
"""Rebuild chat_start.db from a loop round's start-chat.db.

usage: slim_chat_start.py <rounds/<round>/start-chat.db> chat_start.db

Keeps the plan, library, profile and settings; drops run data (ai_* tables,
plate events and feedback) and the run ids that point at it, then VACUUMs.
"""
import shutil
import sqlite3
import sys

src, dst = sys.argv[1:3]
shutil.copy(src, dst)
db = sqlite3.connect(dst)
db.execute("pragma foreign_keys=off")
for t in ["ai_messages", "ai_conversations", "ai_run_steps", "ai_runs", "ai_changes",
          "ai_prompts", "ai_tool_schemas", "plate_events", "plate_feedback"]:
    db.execute(f"delete from {t}")
for t in ["plates", "cook_sessions"]:
    db.execute(f"update {t} set ai_run_id = NULL")
assert db.execute("pragma foreign_key_check").fetchall() == []
db.commit()
db.execute("vacuum")
