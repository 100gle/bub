#!/usr/bin/env python3
"""A scripted stand-in for the `bub desktop` bridge.

Speaks the same newline-delimited JSON protocol as the real bridge, so the GPUI
front-end can be driven without a model, a network, or a workspace. Point the
app at it to review layout and states:

    BUB_DESKTOP_CMD="python3 tools/fake_bridge.py" FAKE_MODE=demo \
        cargo run

    FAKE_MODE=empty  -> only `ready`, so the shell shows its session-start state
    FAKE_MODE=demo   -> streams a full run per message (the default)

Set FAKE_APPROVAL=1 to have the first run stop on an approval, so the gate and
its Deny/Allow controls are on screen.
"""

import json
import os
import sys
import time

MODE = os.environ.get("FAKE_MODE", "demo")
WORKSPACE = os.environ.get("FAKE_WORKSPACE", os.getcwd())

# (name, description, exposure, deferred) — mirrors what the real bridge
# advertises, including the two tools that are only named until first use.
TOOLS = [
    (
        "bash",
        "Run a shell command. Use background=true to keep it running and fetch output later via bash_output.",
        "command",
        False,
    ),
    ("bash.kill", "Terminate a background shell process.", "command", False),
    (
        "bash.output",
        "Read buffered output from a background shell, with optional offset/limit for incremental polling.",
        "command",
        False,
    ),
    ("code_mode", "Enable or disable code mode for this session.", "command", False),
    (
        "fs_edit",
        "Edit a file by replacing old text with new text. You can specify the line number to start searching from.",
        "code",
        False,
    ),
    ("fs_read", "Read a file, or a line range of one.", "code", False),
    ("fs_write", "Write a file, replacing its contents.", "code", False),
    ("grep", "Search the workspace with a regular expression.", "code", True),
    ("glob", "List files matching a glob pattern.", "code", True),
    ("todo_write", "Record the plan for the current turn as a task list.", "code", False),
]

SKILLS = [
    ("workbuddy-docx", "Create and beautify Word documents.", "builtin"),
    ("workbuddy-pptx", "Create and edit PowerPoint decks.", "builtin"),
    ("workbuddy-sheetagent", "Analyse and edit existing spreadsheets.", "builtin"),
    ("pdf", "Read and produce PDF files.", "builtin"),
    ("commit", "Write a conventional commit for the staged changes.", "project"),
    ("review-pr", "Review a pull request against the repo guidelines.", "project"),
]

COMMANDS = ["/clear", "/compact", "/help", "/model", "/tools", "/plan"]


def emit(payload):
    sys.stdout.write(json.dumps(payload) + "\n")
    sys.stdout.flush()


def ready():
    emit({
        "type": "ready",
        "protocol": 2,
        "workspace": WORKSPACE,
        "model": "openai:gpt-4o",
        "command_prefix": "/",
        "session_id": "desktop:default",
        "approval_mode": "auto",
        "commands": COMMANDS,
        "tools": [{"name": n, "description": d, "exposure": e, "deferred": lazy} for n, d, e, lazy in TOOLS],
        "skills": [{"name": n, "description": d, "source": s} for n, d, s in SKILLS],
    })


def stream(kind, data):
    """One streamed event. The pause between events is what makes the app's
    "following the tail" behaviour visible while it runs."""
    emit({"type": "stream", "kind": kind, "data": data})
    time.sleep(0.08)


def walk():
    """One scripted run: reasoning, two tool calls, prose, usage."""
    emit({"type": "generating", "active": True})

    stream(
        "reasoning",
        {
            "delta": "I'll look at how the CLI wires the deploy command, then add the flag "
            "where the arguments are parsed."
        },
    )

    stream(
        "tool_call",
        {
            "tool_calls": [
                {"name": "bash", "arguments": {"command": "rg -n 'deploy' src/bub | head -20", "background": False}}
            ]
        },
    )
    stream("tool_result", {"tool_results": [{"stdout": "src/bub/__main__.py:118:def deploy(\n"}]})

    stream(
        "text",
        {
            "delta": "# Dry run\n\nThe deploy path already resolves its target from a single "
            "config object, so the flag only has to reach that resolver. "
        },
    )
    stream("text", {"delta": "I'll thread it through rather than branch on it twice.\n\n"})

    stream(
        "tool_call",
        {
            "tool_calls": [
                {
                    "name": "fs_edit",
                    "arguments": {
                        "path": "src/bub/builtin/cli.py",
                        "old_text": "def deploy(root: Path) -> None:",
                        "new_text": "def deploy(root: Path, dry_run: bool = False) -> None:",
                    },
                }
            ]
        },
    )
    stream("tool_result", {"tool_results": [{"ok": True}]})

    stream("text", {"delta": "The flag is in place. Next I'd wire it to `--dry-run`.\n"})

    stream("usage", {"elapsed_seconds": 6.4, "usage": {"total_tokens": 4215}})
    stream("final", {})
    emit({"type": "generating", "active": False})


def awaiting_approval():
    """A run that stops on an approval, so the gate is visible."""
    emit({"type": "generating", "active": True})
    stream("tool_call", {"tool_calls": [{"name": "bash", "arguments": {"command": "git push --force origin gpui"}}]})
    emit({
        "type": "approval",
        "approval_id": "a1",
        "tool": "bash",
        "arguments": {"command": "git push --force origin gpui"},
    })

    for line in sys.stdin:
        try:
            message = json.loads(line)
        except ValueError:
            continue
        if message.get("type") != "approval_response":
            continue
        allowed = bool(message.get("allow"))
        stream("tool_result", {"tool_results": [{"ok": allowed} if allowed else {"error": "denied by the operator"}]})
        stream("text", {"delta": "Understood — leaving the branch as it is.\n"})
        stream("final", {})
        emit({"type": "generating", "active": False})
        return


def main():
    ready()
    turns = 0
    for line in sys.stdin:
        try:
            message = json.loads(line)
        except ValueError:
            continue
        kind = message.get("type")
        if kind == "ping":
            emit({"type": "pong", "id": message.get("id")})
        elif kind == "send":
            turns += 1
            walk()
            if turns == 1 and os.environ.get("FAKE_APPROVAL"):
                awaiting_approval()
        elif kind == "stop":
            emit({"type": "generating", "active": False})


if __name__ == "__main__":
    main()
