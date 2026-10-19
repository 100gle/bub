"""The desktop channel: a Bub ``Interface`` channel driven over stdio.

This module knows nothing about GPUI. It feeds user input into Bub's pipeline
and serializes inbound model events back out as newline-delimited JSON, so any
front-end can speak the same protocol.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import mimetypes
import sys
from collections.abc import AsyncIterable
from pathlib import Path
from typing import Any, TextIO, cast

from loguru import logger

from bub.builtin.commands import strip_command_prefix
from bub.builtin.settings import load_settings
from bub.channels.admission import AdmitDecision, TurnSnapshot
from bub.channels.base import Interface
from bub.channels.message import ChannelMessage, MediaItem, MediaType
from bub.envelope import Envelope
from bub.streaming import StreamEvent
from bub_desktop import settings as bub_settings

PROTOCOL_VERSION = 2
DEFAULT_SESSION_ID = "desktop:default"
DEFAULT_CHAT_ID = "default"

# Approval modes for tool calls. ``auto`` runs every tool without asking;
# ``ask`` blocks each model tool call until the front-end answers.
APPROVAL_MODES = ("auto", "ask")
DEFAULT_APPROVAL_MODE = "auto"
DEFAULT_APPROVAL_TIMEOUT = 600.0

# The media kinds Bub's message model accepts.
_MEDIA_TYPES = ("image", "audio", "video", "document")


def _read_bytes(path: str) -> bytes:
    return Path(path).read_bytes()


def _media_from_attachments(raw: Any) -> list[MediaItem]:
    """Turn the front-end's queued files into media the runtime can carry."""
    if not isinstance(raw, list):
        return []
    items: list[MediaItem] = []
    for entry in raw:
        if not isinstance(entry, dict):
            continue
        path = str(entry.get("path", ""))
        if not path:
            continue
        media_type = str(entry.get("media_type") or "document")
        if media_type not in _MEDIA_TYPES:
            media_type = "document"
        filename = str(entry.get("name") or Path(path).name)
        mime_type = mimetypes.guess_type(filename)[0] or "application/octet-stream"

        async def fetch(source: str = path) -> bytes:
            return await asyncio.to_thread(_read_bytes, source)

        items.append(
            MediaItem(
                type=cast("MediaType", media_type),
                mime_type=mime_type,
                filename=filename,
                data_fetcher=fetch,
            )
        )
    return items


class Emitter:
    """Serialize newline-delimited JSON events to a stream under a lock."""

    def __init__(self, stream: TextIO | None = None) -> None:
        self._stream = stream if stream is not None else sys.stdout
        self._lock = asyncio.Lock()

    async def emit(self, payload: dict[str, Any]) -> None:
        line = json.dumps(payload, ensure_ascii=False)
        async with self._lock:
            self._stream.write(line + "\n")
            self._stream.flush()


class ApprovalGate:
    """Ask the front-end to allow or deny one tool call before it runs.

    The gate is the only place that couples the agent loop to the UI: a
    ``before_tool_call`` hook awaits :meth:`request`, which emits an ``approval``
    event and resolves when the front-end answers with an ``approval_response``.
    ``auto`` mode short-circuits to allow, so the gate is inert unless the user
    opts into asking.
    """

    def __init__(
        self,
        emitter: Emitter,
        *,
        mode: str = DEFAULT_APPROVAL_MODE,
        timeout: float = DEFAULT_APPROVAL_TIMEOUT,
    ) -> None:
        self._emitter = emitter
        self._mode = mode if mode in APPROVAL_MODES else DEFAULT_APPROVAL_MODE
        self._timeout = timeout
        self._pending: dict[str, asyncio.Future[tuple[bool, str | None]]] = {}
        self._counter = 0

    @property
    def mode(self) -> str:
        return self._mode

    def set_mode(self, mode: Any) -> str:
        """Set the approval mode; unknown values fall back to the default."""
        self._mode = mode if mode in APPROVAL_MODES else DEFAULT_APPROVAL_MODE
        return self._mode

    async def request(self, *, tool: str, arguments: dict[str, Any]) -> tuple[bool, str | None]:
        """Emit an approval request and await the user's decision."""
        if self._mode != "ask":
            return True, None

        self._counter += 1
        approval_id = f"approval-{self._counter}"
        loop = asyncio.get_running_loop()
        future: asyncio.Future[tuple[bool, str | None]] = loop.create_future()
        self._pending[approval_id] = future
        await self._emitter.emit({
            "type": "approval",
            "approval_id": approval_id,
            "tool": tool,
            "arguments": arguments,
        })

        allow: bool = False
        reason: str | None = "cancelled"
        try:
            try:
                allow, reason = await asyncio.wait_for(future, timeout=self._timeout)
            except TimeoutError:
                allow, reason = False, "approval timed out"
        finally:
            self._pending.pop(approval_id, None)
            with contextlib.suppress(Exception):
                await self._emitter.emit({
                    "type": "approval_resolved",
                    "approval_id": approval_id,
                    "allow": allow,
                    "reason": reason,
                })
        return allow, reason

    def resolve(self, approval_id: str, allow: bool, reason: str | None = None) -> bool:
        """Resolve a pending request; returns False when the id is unknown."""
        future = self._pending.get(approval_id)
        if future is None or future.done():
            return False
        future.set_result((allow, reason))
        return True

    def fail_all(self, reason: str) -> None:
        """Resolve every pending request as denied (used when the bridge stops)."""
        for future in self._pending.values():
            if not future.done():
                future.set_result((False, reason))
        self._pending.clear()


class DesktopChannel(Interface):
    """Drive one or more Bub sessions from a desktop front-end over stdio."""

    name = "desktop"

    def __init__(
        self,
        *,
        on_receive: Any,
        emitter: Emitter,
        framework: Any = None,
        session_id: str = DEFAULT_SESSION_ID,
        chat_id: str = DEFAULT_CHAT_ID,
        approval_gate: ApprovalGate | None = None,
        input_stream: Any = None,
    ) -> None:
        self._on_receive = on_receive
        self._emitter = emitter
        self._framework = framework
        self._session_id = session_id
        self._chat_id = chat_id
        self._approval_gate = approval_gate
        self._input = input_stream if input_stream is not None else sys.stdin.buffer
        self._stop_event: asyncio.Event | None = None
        self._reader_task: asyncio.Task[None] | None = None
        # The prefix the front-end's command test uses; kept in step with a
        # settings edit so a new prefix works without a restart.
        self._command_prefix = load_settings().command_prefix

    async def start(self, stop_event: asyncio.Event) -> None:
        self._stop_event = stop_event
        self._reader_task = asyncio.create_task(self._read_stdin())

    async def stop(self) -> None:
        if self._approval_gate is not None:
            self._approval_gate.fail_all("desktop bridge stopped")
        if self._reader_task is not None:
            self._reader_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._reader_task
            self._reader_task = None

    async def send(self, message: ChannelMessage) -> None:
        await self._emitter.emit({
            "type": "outbound",
            "session_id": message.session_id,
            "channel": message.channel,
            "kind": message.kind,
            "content": message.content,
        })

    async def stream_events(
        self, message: ChannelMessage, stream: AsyncIterable[StreamEvent]
    ) -> AsyncIterable[StreamEvent]:
        session_id = message.session_id
        await self._emitter.emit({"type": "generating", "session_id": session_id, "active": True})
        try:
            async for event in stream:
                await self._emitter.emit({
                    "type": "stream",
                    "session_id": session_id,
                    "kind": event.kind,
                    "data": event.data,
                })
                yield event
        finally:
            await self._emitter.emit({"type": "generating", "session_id": session_id, "active": False})

    async def admit_message(
        self,
        session_id: str,
        message: Envelope,
        turn: TurnSnapshot,
    ) -> AdmitDecision | None:
        if not turn.is_running:
            return None
        return AdmitDecision("steer", reason="desktop session is already generating")

    async def _route(self, content: str) -> None:
        """Hand a command or message to Bub exactly as an inbound message."""
        message = ChannelMessage(
            session_id=self._session_id,
            channel=self.name,
            chat_id=self._chat_id,
            content=content,
            kind="command" if strip_command_prefix(content, self._command_prefix) is not None else "normal",
            output_channel=self.name,
        )
        await self._on_receive(message)

    async def _read_stdin(self) -> None:
        loop = asyncio.get_running_loop()
        while True:
            raw = await loop.run_in_executor(None, self._input.readline)
            if not raw:
                break
            line = raw.decode("utf-8", "replace").strip()
            if not line:
                continue
            try:
                payload = json.loads(line)
            except json.JSONDecodeError:
                logger.warning("desktop: ignoring malformed input line")
                continue
            if not isinstance(payload, dict):
                continue
            try:
                await self._handle(payload)
            except Exception as exc:
                logger.exception("desktop: request failed")
                await self._emitter.emit({
                    "type": "result",
                    "id": payload.get("id"),
                    "ok": False,
                    "error": str(exc),
                })
        if self._stop_event is not None:
            self._stop_event.set()

    async def _handle(self, payload: dict[str, Any]) -> None:
        request_id = payload.get("id")
        match payload.get("type"):
            case "send":
                await self._handle_send(payload, request_id)
            case "stop":
                await self._handle_stop(payload, request_id)
            case "models":
                await self._handle_models(payload, request_id)
            case "settings":
                await self._handle_settings(payload, request_id)
            case "settings_update":
                await self._handle_settings_update(payload, request_id)
            case "approval_response":
                await self._handle_approval_response(payload, request_id)
            case "approval_mode":
                await self._handle_approval_mode(payload, request_id)
            case "ping":
                await self._emitter.emit({"type": "pong", "id": request_id})
            case other:
                await self._emitter.emit({
                    "type": "result",
                    "id": request_id,
                    "ok": False,
                    "error": f"unknown request type: {other!r}",
                })

    async def _handle_send(self, payload: dict[str, Any], request_id: Any) -> None:
        text = str(payload.get("text", ""))
        session_id = str(payload.get("session_id") or self._session_id)
        is_command = strip_command_prefix(text, self._command_prefix) is not None
        message = ChannelMessage(
            session_id=session_id,
            channel=self.name,
            chat_id=self._chat_id,
            content=text,
            kind="command" if is_command else "normal",
            media=_media_from_attachments(payload.get("attachments")),
            output_channel=self.name,
        )
        await self._on_receive(message)
        await self._emitter.emit({"type": "result", "id": request_id, "ok": True})

    async def _handle_stop(self, payload: dict[str, Any], request_id: Any) -> None:
        session_id = str(payload.get("session_id") or self._session_id)
        if self._framework is not None:
            await self._framework.quit_via_channel_router(session_id)
        await self._emitter.emit({"type": "result", "id": request_id, "ok": True})

    async def _handle_models(self, payload: dict[str, Any], request_id: Any) -> None:
        if self._framework is None:
            await self._emitter.emit({"type": "result", "id": request_id, "ok": False, "error": "no framework"})
            return
        session_id = str(payload.get("session_id") or self._session_id)
        options = await self._framework.get_model_options(session_id=session_id)
        await self._emitter.emit({
            "type": "models",
            "id": request_id,
            "current": options.current_model,
            "models": [{"id": model.id, "name": model.name} for model in options.models],
        })

    async def _handle_settings(self, payload: dict[str, Any], request_id: Any) -> None:
        if self._framework is None:
            await self._emitter.emit({"type": "result", "id": request_id, "ok": False, "error": "no framework"})
            return
        await self._emitter.emit({
            "type": "settings",
            "id": request_id,
            "values": bub_settings.snapshot(self._framework),
            "overridden": bub_settings.overridden(),
            "config_file": bub_settings.config_file(self._framework),
            "home": bub_settings.home(self._framework),
        })

    async def _handle_settings_update(self, payload: dict[str, Any], request_id: Any) -> None:
        values = payload.get("values")
        if not isinstance(values, dict) or not values:
            await self._emitter.emit({"type": "result", "id": request_id, "ok": False, "error": "no values"})
            return
        if self._framework is None:
            await self._emitter.emit({"type": "result", "id": request_id, "ok": False, "error": "no framework"})
            return
        bub_settings.persist(self._framework, values)
        prefix = values.get("command_prefix")
        if isinstance(prefix, str) and prefix:
            self._command_prefix = prefix
        # A new default model also becomes the session's model now, by running
        # Bub's own `,model` command, so the choice is live rather than pending.
        model = values.get("model")
        if isinstance(model, str) and model.strip():
            await self._route(f"{self._command_prefix}model {model.strip()}")
        await self._emitter.emit({"type": "result", "id": request_id, "ok": True})

    async def _handle_approval_response(self, payload: dict[str, Any], request_id: Any) -> None:
        approval_id = str(payload.get("approval_id", ""))
        allow = bool(payload.get("allow", False))
        resolved = self._approval_gate.resolve(approval_id, allow) if self._approval_gate is not None else False
        await self._emitter.emit({"type": "result", "id": request_id, "ok": resolved})

    async def _handle_approval_mode(self, payload: dict[str, Any], request_id: Any) -> None:
        gate = self._approval_gate
        mode = gate.set_mode(payload.get("mode")) if gate is not None else DEFAULT_APPROVAL_MODE
        await self._emitter.emit({"type": "approval_mode", "id": request_id, "mode": mode})
