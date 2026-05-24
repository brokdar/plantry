#!/usr/bin/env node
/**
 * PreCompact Hook - Log compaction timestamp before context is compacted
 *
 * Standalone adaptation (no ECC dependencies).
 *
 * Appends a timestamped entry to ~/.claude/sessions/compaction-log.txt
 * and notes compaction in the most recent active session file.
 */

'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getSessionsDir() {
  return path.join(os.homedir(), '.claude', 'sessions');
}

function log(msg) {
  process.stderr.write(msg + '\n');
}

function ensureDir(dir) {
  try {
    fs.mkdirSync(dir, { recursive: true });
  } catch {
    // ignore
  }
}

function appendFile(filePath, content) {
  try {
    fs.appendFileSync(filePath, content, 'utf8');
    return true;
  } catch {
    return false;
  }
}

function getDateTimeString() {
  const d = new Date();
  const date = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const time = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
  return `${date} ${time}`;
}

function getTimeString() {
  const d = new Date();
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
}

function findMostRecentSessionFile(sessionsDir) {
  try {
    const entries = fs.readdirSync(sessionsDir, { withFileTypes: true });
    let newest = null;
    let newestMtime = 0;

    for (const entry of entries) {
      if (!entry.isFile() || !entry.name.endsWith('-session.tmp')) continue;
      const filePath = path.join(sessionsDir, entry.name);
      try {
        const stat = fs.statSync(filePath);
        if (stat.mtimeMs > newestMtime) {
          newestMtime = stat.mtimeMs;
          newest = filePath;
        }
      } catch {
        // skip
      }
    }
    return newest;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const sessionsDir = getSessionsDir();
  ensureDir(sessionsDir);

  const compactionLog = path.join(sessionsDir, 'compaction-log.txt');
  const timestamp = getDateTimeString();
  appendFile(compactionLog, `[${timestamp}] Context compaction triggered\n`);

  // Note compaction in the active session file if one exists
  const activeSession = findMostRecentSessionFile(sessionsDir);
  if (activeSession) {
    const timeStr = getTimeString();
    appendFile(activeSession, `\n---\n**[Compaction occurred at ${timeStr}]** - Context was summarized\n`);
  }

  log('[PreCompact] Compaction timestamp logged');
  process.exit(0);
}

main().catch(err => {
  console.error('[PreCompact] Error:', err.message);
  process.exit(0);
});
