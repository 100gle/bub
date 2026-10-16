from __future__ import annotations

import asyncio
import io
import json
from typing import Any, cast

import pytest
from bub_desktop.channel import ApprovalGate, DesktopChannel, Emitter
from bub_desktop.plugin import BubDesktopPlugin

from bub.channels.admission import TurnSnapshot
from bub.channels.message import ChannelMessage
from bub.hooks.interception import ToolCall
from bub.streaming import StreamEvent


async def _noop(message: ChannelMessage) -> None:
    return None


def _channel(**kwargs: Any) -> tuple[DesktopChannel, io.StringIO]:
    buffer = io.StringIO()
    emitter = Emitter(buffer)

    async def on_receive(message: ChannelMessage) -> None:
        return None

    channel = DesktopChannel(on_receive=on_receive, emitter=emitter, **kwargs)
    return channel, buffer


def _lines(buffer: io.StringIO) -> list[dict[str, Any]]:
    return [json.loads(line) for line in buffer.getvalue().splitlines() if line.strip()]


@pytest.mark.asyncio
async def test_emitter_writes_one_json_object_per_line() -> None:
    buffer = io.StringIO()
    emitter = Emitter(buffer)
    await emitter.emit({"type": "a", "value": 1})
    await emitter.emit({"type": "b", "value": "中文"})

    assert _lines(buffer) == [{"type": "a", "value": 1}, {"type": "b", "value": "中文"}]


@pytest.mark.asyncio
async def test_stream_events_forwards_and_emits_each_event() -> None:
    channel, buffer = _channel()

    async def events():
        yield StreamEvent("reasoning", {"delta": "thinking"})
        yield StreamEvent("text", {"delta": "hello"})
        yield StreamEvent("final", {"text": "hello"})

    message = ChannelMessage(session_id="s1", channel="desktop", content="hi")
    seen = [event async for event in channel.stream_events(message, events())]

    assert [event.kind for event in seen] == ["reasoning", "text", "final"]
    emitted = _lines(buffer)
    assert emitted[0] == {"type": "generating", "session_id": "s1", "active": True}
    assert emitted[1] == {"type": "stream", "session_id": "s1", "kind": "reasoning", "data": {"delta": "thinking"}}
    assert emitted[2] == {"type": "stream", "session_id": "s1", "kind": "text", "data": {"delta": "hello"}}
    assert emitted[-1] == {"type": "generating", "session_id": "s1", "active": False}


@pytest.mark.asyncio
async def test_send_builds_channel_message_routed_back_to_desktop() -> None:
    captured: list[ChannelMessage] = []
    buffer = io.StringIO()

    async def on_receive(message: ChannelMessage) -> None:
        captured.append(message)

    channel = DesktopChannel(on_receive=on_receive, emitter=Emitter(buffer))
    await channel._handle({"id": "r1", "type": "send", "session_id": "x", "text": "hello"})

    assert len(captured) == 1
    message = captured[0]
    assert message.session_id == "x"
    assert message.channel == "desktop"
    assert message.output_channel == "desktop"
    assert message.content == "hello"
    assert message.kind == "normal"
    assert _lines(buffer) == [{"type": "result", "id": "r1", "ok": True}]


@pytest.mark.asyncio
async def test_send_marks_prefixed_input_as_command() -> None:
    captured: list[ChannelMessage] = []

    async def on_receive(message: ChannelMessage) -> None:
        captured.append(message)

    channel = DesktopChannel(on_receive=on_receive, emitter=Emitter(io.StringIO()))
    await channel._handle({"id": "r2", "type": "send", "text": ",help"})

    assert captured and captured[0].kind == "command"


@pytest.mark.asyncio
async def test_stop_asks_the_router_to_quit_the_session() -> None:
    quit_calls: list[str] = []

    class FakeFramework:
        async def quit_via_channel_router(self, session_id: str) -> None:
            quit_calls.append(session_id)

    buffer = io.StringIO()

    async def on_receive(message: ChannelMessage) -> None:
        return None

    channel = DesktopChannel(on_receive=on_receive, emitter=Emitter(buffer), framework=FakeFramework())
    await channel._handle({"id": "r3", "type": "stop", "session_id": "y"})

    assert quit_calls == ["y"]
    assert _lines(buffer) == [{"type": "result", "id": "r3", "ok": True}]


@pytest.mark.asyncio
async def test_unknown_request_type_reports_error() -> None:
    channel, buffer = _channel()
    await channel._handle({"id": "r4", "type": "nope"})

    emitted = _lines(buffer)
    assert emitted[0]["ok"] is False
    assert "unknown request type" in emitted[0]["error"]


async def _wait_for_approval_id(buffer: io.StringIO) -> str:
    for _ in range(400):
        for line in _lines(buffer):
            if line.get("type") == "approval":
                return str(line["approval_id"])
        await asyncio.sleep(0.005)
    raise AssertionError("no approval event was emitted")


@pytest.mark.asyncio
async def test_approval_gate_auto_mode_allows_without_emitting() -> None:
    buffer = io.StringIO()
    gate = ApprovalGate(Emitter(buffer), mode="auto")

    allowed, reason = await gate.request(tool="bash", arguments={"command": "ls"})

    assert (allowed, reason) == (True, None)
    assert _lines(buffer) == []


@pytest.mark.asyncio
async def test_approval_gate_ask_mode_emits_and_resolves() -> None:
    buffer = io.StringIO()
    gate = ApprovalGate(Emitter(buffer), mode="ask")
    channel = DesktopChannel(on_receive=_noop, emitter=Emitter(io.StringIO()), approval_gate=gate)

    task = asyncio.create_task(gate.request(tool="bash", arguments={"command": "ls"}))
    approval_id = await _wait_for_approval_id(buffer)

    request = _lines(buffer)[0]
    assert request == {
        "type": "approval",
        "approval_id": approval_id,
        "tool": "bash",
        "arguments": {"command": "ls"},
    }

    await channel._handle({"id": "r", "type": "approval_response", "approval_id": approval_id, "allow": False})
    allowed, _reason = await task

    assert allowed is False
    resolved = _lines(buffer)[-1]
    assert resolved["type"] == "approval_resolved"
    assert resolved["approval_id"] == approval_id and resolved["allow"] is False


@pytest.mark.asyncio
async def test_approval_mode_request_updates_the_gate() -> None:
    gate = ApprovalGate(Emitter(io.StringIO()), mode="auto")
    buffer = io.StringIO()
    channel = DesktopChannel(on_receive=_noop, emitter=Emitter(buffer), approval_gate=gate)

    await channel._handle({"id": "r", "type": "approval_mode", "mode": "ask"})

    assert gate.mode == "ask"
    assert _lines(buffer) == [{"type": "approval_mode", "id": "r", "mode": "ask"}]


@pytest.mark.asyncio
async def test_plugin_hook_is_inert_in_auto_mode() -> None:
    plugin = BubDesktopPlugin(cast(Any, None))
    plugin.configure(emitter=Emitter(io.StringIO()), session_id="s", chat_id="c", approval_mode="auto")

    call = ToolCall(run_id="r", tool="bash", arguments={"command": "ls"})

    assert await plugin.before_tool_call(call, {}) is None


@pytest.mark.asyncio
async def test_plugin_hook_denies_when_the_user_rejects() -> None:
    buffer = io.StringIO()
    plugin = BubDesktopPlugin(cast(Any, None))
    plugin.configure(emitter=Emitter(buffer), session_id="s", chat_id="c", approval_mode="ask")
    gate = plugin.approval_gate
    assert gate is not None

    call = ToolCall(run_id="r", tool="bash", arguments={"command": "rm -rf /"})
    task = asyncio.create_task(plugin.before_tool_call(call, {}))
    approval_id = await _wait_for_approval_id(buffer)
    gate.resolve(approval_id, False, "not allowed")

    decision = await task

    assert decision is not None and decision.action == "deny"
    assert decision.message == "not allowed"


@pytest.mark.asyncio
async def test_plugin_hook_allows_when_the_user_accepts() -> None:
    buffer = io.StringIO()
    plugin = BubDesktopPlugin(cast(Any, None))
    plugin.configure(emitter=Emitter(buffer), session_id="s", chat_id="c", approval_mode="ask")
    gate = plugin.approval_gate
    assert gate is not None

    call = ToolCall(run_id="r", tool="fs.read", arguments={"path": "README.md"})
    task = asyncio.create_task(plugin.before_tool_call(call, {}))
    approval_id = await _wait_for_approval_id(buffer)
    gate.resolve(approval_id, True)

    assert await task is None


@pytest.mark.asyncio
async def test_admit_message_steers_only_while_generating() -> None:
    channel, _ = _channel()
    idle = TurnSnapshot(session_id="s", is_running=False, running_count=0, pending_count=0)
    busy = TurnSnapshot(session_id="s", is_running=True, running_count=1, pending_count=0)

    assert await channel.admit_message("s", {"content": "hi"}, idle) is None
    decision = await channel.admit_message("s", {"content": "hi"}, busy)
    assert decision is not None and decision.action == "steer"
