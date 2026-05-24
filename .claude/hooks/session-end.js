#!/usr/bin/env node
/**
 * Stop Hook (Session End) - Persist session summary for cross-session continuity
 *
 * Standalone adaptation (no ECC dependencies).
 *
 * Runs on Stop events (after each response). Extracts a meaningful summary
 * from the session transcript (via stdin JSON transcript_path) and writes/updates
 * a session file under ~/.claude/sessions/.
 */

'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');
const { execFileSync } = require('child_process');

const PROJECT_SLUG = 'plantry';
const SUMMARY_START_MARKER = '<!-- SESSION_START -->';
const SUMMARY_END_MARKER = '<!-- SESSION_END -->';
const SESSION_SEPARATOR = '\n---\n';

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

function readFile(filePath) {
  try {
    return fs.readFileSync(filePath, 'utf8');
  } catch {
    return null;
  }
}

function writeFile(filePath, content) {
  try {
    fs.writeFileSync(filePath, content, 'utf8');
    return true;
  } catch {
    return false;
  }
}

function getDateString() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function getTimeString() {
  const d = new Date();
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
}

function stripAnsi(str) {
  return String(str || '').replace(
    // eslint-disable-next-line no-control-regex
    /[][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><~]/g,
    ''
  );
}

function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function runGitBranch() {
  try {
    const output = execFileSync('git', ['rev-parse', '--abbrev-ref', 'HEAD'], {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe']
    }).trim();
    return { success: true, output };
  } catch {
    return { success: false, output: '' };
  }
}

// ---------------------------------------------------------------------------
// Transcript parsing
// ---------------------------------------------------------------------------

/**
 * Extract a meaningful summary from the session transcript JSONL.
 * Pulls out user messages, tools used, and files modified.
 */
function extractSessionSummary(transcriptPath) {
  const content = readFile(transcriptPath);
  if (!content) return null;

  const lines = content.split('\n').filter(Boolean);
  const userMessages = [];
  const toolsUsed = new Set();
  const filesModified = new Set();
  let parseErrors = 0;

  for (const line of lines) {
    try {
      const entry = JSON.parse(line);

      // Collect user messages
      if (entry.type === 'user' || entry.role === 'user' || entry.message?.role === 'user') {
        const rawContent = entry.message?.content ?? entry.content;
        const text = typeof rawContent === 'string'
          ? rawContent
          : Array.isArray(rawContent)
            ? rawContent.map(c => (c && c.text) || '').join(' ')
            : '';
        const cleaned = stripAnsi(text).trim();
        if (cleaned) {
          userMessages.push(cleaned.slice(0, 200));
        }
      }

      // Collect tool names and modified files (direct tool_use entries)
      if (entry.type === 'tool_use' || entry.tool_name) {
        const toolName = entry.tool_name || entry.name || '';
        if (toolName) toolsUsed.add(toolName);

        const filePath = entry.tool_input?.file_path || entry.input?.file_path || '';
        if (filePath && (toolName === 'Edit' || toolName === 'Write')) {
          filesModified.add(filePath);
        }
      }

      // Extract tool uses from assistant message content blocks
      if (entry.type === 'assistant' && Array.isArray(entry.message?.content)) {
        for (const block of entry.message.content) {
          if (block.type === 'tool_use') {
            const toolName = block.name || '';
            if (toolName) toolsUsed.add(toolName);

            const filePath = block.input?.file_path || '';
            if (filePath && (toolName === 'Edit' || toolName === 'Write')) {
              filesModified.add(filePath);
            }
          }
        }
      }
    } catch {
      parseErrors++;
    }
  }

  if (parseErrors > 0) {
    log(`[SessionEnd] Skipped ${parseErrors}/${lines.length} unparseable transcript lines`);
  }

  if (userMessages.length === 0) return null;

  return {
    userMessages: userMessages.slice(-10),
    toolsUsed: Array.from(toolsUsed).slice(0, 20),
    filesModified: Array.from(filesModified).slice(0, 30),
    totalMessages: userMessages.length
  };
}

// ---------------------------------------------------------------------------
// Session file building
// ---------------------------------------------------------------------------

function getSessionMetadata() {
  const branchResult = runGitBranch();
  return {
    project: PROJECT_SLUG,
    branch: branchResult.success ? branchResult.output : 'unknown',
    worktree: process.cwd()
  };
}

function extractHeaderField(content, label) {
  const match = content.match(new RegExp(`\\*\\*${escapeRegExp(label)}:\\*\\*\\s*(.+)$`, 'm'));
  return match ? match[1].trim() : null;
}

function buildSessionHeader(today, currentTime, metadata, existingContent = '') {
  const headingMatch = existingContent.match(/^#\s+.+$/m);
  const heading = headingMatch ? headingMatch[0] : `# Session: ${today}`;
  const date = extractHeaderField(existingContent, 'Date') || today;
  const started = extractHeaderField(existingContent, 'Started') || currentTime;

  return [
    heading,
    `**Date:** ${date}`,
    `**Started:** ${started}`,
    `**Last Updated:** ${currentTime}`,
    `**Project:** ${metadata.project}`,
    `**Branch:** ${metadata.branch}`,
    `**Worktree:** ${metadata.worktree}`,
    ''
  ].join('\n');
}

function mergeSessionHeader(content, today, currentTime, metadata) {
  const separatorIndex = content.indexOf(SESSION_SEPARATOR);
  if (separatorIndex === -1) return null;

  const existingHeader = content.slice(0, separatorIndex);
  const body = content.slice(separatorIndex + SESSION_SEPARATOR.length);
  const nextHeader = buildSessionHeader(today, currentTime, metadata, existingHeader);
  return `${nextHeader}${SESSION_SEPARATOR}${body}`;
}

function buildSummarySection(summary) {
  let section = '## Session Summary\n\n';

  section += '### Tasks\n';
  for (const msg of summary.userMessages) {
    section += `- ${msg.replace(/\n/g, ' ').replace(/`/g, '\\`')}\n`;
  }
  section += '\n';

  if (summary.filesModified.length > 0) {
    section += '### Files Modified\n';
    for (const f of summary.filesModified) {
      section += `- ${f}\n`;
    }
    section += '\n';
  }

  if (summary.toolsUsed.length > 0) {
    section += `### Tools Used\n${summary.toolsUsed.join(', ')}\n\n`;
  }

  section += `### Stats\n- Total user messages: ${summary.totalMessages}\n`;
  return section;
}

function buildSummaryBlock(summary) {
  return `${SUMMARY_START_MARKER}\n${buildSummarySection(summary).trim()}\n${SUMMARY_END_MARKER}`;
}

// ---------------------------------------------------------------------------
// Stdin reading
// ---------------------------------------------------------------------------

const MAX_STDIN = 1024 * 1024;
let stdinData = '';
process.stdin.setEncoding('utf8');

process.stdin.on('data', chunk => {
  if (stdinData.length < MAX_STDIN) {
    stdinData += chunk.substring(0, MAX_STDIN - stdinData.length);
  }
});

process.stdin.on('end', () => {
  main().catch(err => {
    console.error('[SessionEnd] Error:', err.message);
    process.exit(0);
  });
});

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  // Parse transcript_path from stdin JSON
  let transcriptPath = null;
  try {
    const input = JSON.parse(stdinData);
    if (input && typeof input.transcript_path === 'string' && input.transcript_path.length > 0) {
      transcriptPath = input.transcript_path;
    }
  } catch {
    // fall through to env var fallback
  }
  if (!transcriptPath) {
    const envPath = process.env.CLAUDE_TRANSCRIPT_PATH;
    if (typeof envPath === 'string' && envPath.length > 0) {
      transcriptPath = envPath;
    }
  }

  const sessionsDir = getSessionsDir();
  ensureDir(sessionsDir);

  const today = getDateString();
  const currentTime = getTimeString();

  // Derive a short ID from the transcript UUID for a stable, unique filename
  let shortId = PROJECT_SLUG;
  if (transcriptPath) {
    const m = path.basename(transcriptPath).match(/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$/i);
    if (m) {
      shortId = m[1].slice(-8).toLowerCase();
    }
  }

  const sessionFile = path.join(sessionsDir, `${today}-${shortId}-session.tmp`);
  const metadata = getSessionMetadata();

  // Extract summary from transcript
  let summary = null;
  if (transcriptPath) {
    if (fs.existsSync(transcriptPath)) {
      summary = extractSessionSummary(transcriptPath);
    } else {
      log(`[SessionEnd] Transcript not found: ${transcriptPath}`);
    }
  }

  if (fs.existsSync(sessionFile)) {
    let updatedContent = readFile(sessionFile) || '';

    if (updatedContent) {
      const merged = mergeSessionHeader(updatedContent, today, currentTime, metadata);
      if (merged) {
        updatedContent = merged;
      }
    }

    if (summary && updatedContent) {
      const summaryBlock = buildSummaryBlock(summary);
      if (updatedContent.includes(SUMMARY_START_MARKER) && updatedContent.includes(SUMMARY_END_MARKER)) {
        updatedContent = updatedContent.replace(
          new RegExp(`${escapeRegExp(SUMMARY_START_MARKER)}[\\s\\S]*?${escapeRegExp(SUMMARY_END_MARKER)}`),
          summaryBlock
        );
      } else {
        updatedContent += `\n\n${summaryBlock}\n`;
      }
    }

    if (updatedContent) {
      writeFile(sessionFile, updatedContent);
    }
    log(`[SessionEnd] Updated session file: ${sessionFile}`);
  } else {
    const summarySection = summary
      ? `${buildSummaryBlock(summary)}\n\n### Notes for Next Session\n-\n\n### Context to Load\n\`\`\`\n[relevant files]\n\`\`\``
      : `## Current State\n\n[Session context goes here]\n\n### Completed\n- [ ]\n\n### In Progress\n- [ ]\n\n### Notes for Next Session\n-\n\n### Context to Load\n\`\`\`\n[relevant files]\n\`\`\``;

    const template = `${buildSessionHeader(today, currentTime, metadata)}${SESSION_SEPARATOR}${summarySection}\n`;
    writeFile(sessionFile, template);
    log(`[SessionEnd] Created session file: ${sessionFile}`);
  }

  process.exit(0);
}
