"""Plugin wiring: channel registration, the desktop tool-approval hook, and the
``bub desktop`` command."""

from __future__ import annotations

import asyncio
import os
import sys
from pathlib import Path
from typing import Any

import typer
from loguru import logger

from bub.builtin.settings import load_settings
from bub.channels.manager import ChannelManager
from bub.framework import BubFramework
from bub.hooks import hookimpl
from bub.hooks.interception import ToolCall, ToolCallDecision
from bub.tools import REGISTRY
from bub_desktop.channel import (
    DEFAULT_APPROVAL_MODE,
    DEFAULT_CHAT_ID,
    DEFAULT_SESSION_ID,
    PROTOCOL_VERSION,
    ApprovalGate,
    DesktopChannel,
    Emitter,
)

PLUGIN_NAME = "bub-desktop"


class BubDesktopPlugin:
    """Expose Bub to a desktop front-end over the stdio bridge.

    The channel is only provided once :meth:`configure` has been called (by the
    ``bub desktop`` command), so loading the plugin never starts a bridge.
    """

    def __init__(self, framework: BubFramework) -> None:
        self.framework = framework
        self._emitter: Emitter | None = None
        self._session_id = DEFAULT_SESSION_ID
        self._chat_id = DEFAULT_CHAT_ID
        self._approval_gate: ApprovalGate | None = None
        self.channel: DesktopChannel | None = None

    @property
    def approval_gate(self) -> ApprovalGate | None:
        return self._approval_gate

    def configure(
        self,
        *,
        emitter: Emitter,
        session_id: str,
        chat_id: str,
        approval_mode: str = DEFAULT_APPROVAL_MODE,
    ) -> None:
        self._emitter = emitter
        self._session_id = session_id
        self._chat_id = chat_id
        self._approval_gate = ApprovalGate(emitter, mode=approval_mode)

    @hookimpl
    def provide_channels(self, message_handler: Any) -> list[DesktopChannel]:
        if self._emitter is None:
            return []
        self.channel = DesktopChannel(
            on_receive=message_handler,
            emitter=self._emitter,
            framework=self.framework,
            session_id=self._session_id,
            chat_id=self._chat_id,
            approval_gate=self._approval_gate,
        )
        return [self.channel]

    @hookimpl
    async def before_tool_call(self, call: ToolCall, state: Any) -> ToolCallDecision | None:
        """Ask the desktop front-end before a model tool call runs, in ``ask`` mode."""
        gate = self._approval_gate
        if gate is None or gate.mode != "ask":
            return None
        allowed, reason = await gate.request(tool=call.tool, arguments=call.arguments)
        if allowed:
            return None
        return ToolCallDecision.deny(reason or f"Denied by the user: {call.tool}")

    @hookimpl
    def register_cli_commands(self, app: typer.Typer) -> None:
        app.command("desktop", hidden=True)(desktop)


def desktop(
    ctx: typer.Context,
    session_id: str = typer.Option(DEFAULT_SESSION_ID, "--session-id", help="Session id for the bridge"),
    chat_id: str = typer.Option(DEFAULT_CHAT_ID, "--chat-id", help="Chat id for the bridge"),
) -> None:
    """Serve the desktop bridge (newline-delimited JSON over stdin/stdout)."""

    framework = ctx.ensure_object(BubFramework)
    plugin = framework.plugin_manager.get_plugin(PLUGIN_NAME)
    if not isinstance(plugin, BubDesktopPlugin):
        raise typer.BadParameter("desktop plugin is not loaded")

    emitter = Emitter(sys.stdout)
    approval_mode = os.environ.get("BUB_DESKTOP_APPROVE", DEFAULT_APPROVAL_MODE)
    plugin.configure(
        emitter=emitter,
        session_id=session_id,
        chat_id=chat_id,
        approval_mode=approval_mode,
    )
    asyncio.run(
        run_desktop_bridge(
            framework,
            emitter,
            plugin.approval_gate,
            session_id=session_id,
        )
    )


def _tool_catalog() -> list[dict[str, Any]]:
    """Describe every registered tool for the front-end's capability panel."""
    return [
        {
            "name": tool.name,
            "description": tool.description,
            "exposure": tool.exposure,
            "deferred": tool.deferred,
        }
        for tool in sorted(REGISTRY.values(), key=lambda item: item.name)
    ]


def _skill_catalog(workspace: Path) -> list[dict[str, Any]]:
    """Describe the skills discoverable from the current workspace."""
    from bub.skills import discover_skills

    try:
        skills = discover_skills(workspace)
    except Exception:
        logger.exception("desktop: skill discovery failed")
        return []
    return [{"name": skill.name, "description": skill.description, "source": skill.source} for skill in skills]


async def run_desktop_bridge(
    framework: BubFramework,
    emitter: Emitter,
    approval_gate: ApprovalGate | None = None,
    *,
    session_id: str = DEFAULT_SESSION_ID,
) -> None:
    """Run the stdio bridge until stdin closes or the session stops."""
    manager = ChannelManager(framework, enabled_channels=[DesktopChannel.name], stream_output=True)
    settings = load_settings()
    commands = sorted(name for name, tool in REGISTRY.items() if tool.exposure == "command")
    await emitter.emit({
        "type": "ready",
        "protocol": PROTOCOL_VERSION,
        "workspace": str(framework.workspace),
        "model": settings.model,
        "command_prefix": settings.command_prefix,
        "session_id": session_id,
        "commands": [f"{settings.command_prefix}{name}" for name in commands],
        "tools": _tool_catalog(),
        "skills": _skill_catalog(framework.workspace),
        "approval_mode": approval_gate.mode if approval_gate is not None else DEFAULT_APPROVAL_MODE,
    })
    await manager.listen_and_run()
