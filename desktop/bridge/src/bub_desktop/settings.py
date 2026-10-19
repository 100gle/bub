"""Bub configuration, read and written on behalf of the desktop front-end.

The settings page is a front end to the same keys Bub itself reads, not a second
set of options. This module is the seam between the two: it reads one effective
snapshot — the values after the config file, the defaults, and the environment
have all been merged — and writes edits back through :mod:`bub.configure`, which
validates them against the registered settings classes before persisting.

A key the environment already supplies is reported as ``overridden``: writing
the config file cannot change it, so the page marks it rather than pretending
the edit took.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from typing import Any

import bub
from bub import configure, ensure_config
from bub.builtin.settings import load_settings
from bub.channels.manager import ChannelSettings
from bub.channels.telegram import TelegramSettings

# Environment variables that shadow a config key. The channel settings share the
# ``BUB_`` prefix with the agent settings; Telegram nests under ``BUB_TELEGRAM_``.
_AGENT_ENV = {
    "model": "BUB_MODEL",
    "command_prefix": "BUB_COMMAND_PREFIX",
    "fallback_models": "BUB_FALLBACK_MODELS",
    "api_key": "BUB_API_KEY",
    "api_base": "BUB_API_BASE",
    "max_tokens": "BUB_MAX_TOKENS",
    "max_steps": "BUB_MAX_STEPS",
    "model_timeout_seconds": "BUB_MODEL_TIMEOUT_SECONDS",
    "enabled_channels": "BUB_ENABLED_CHANNELS",
    "stream_output": "BUB_STREAM_OUTPUT",
    "telegram.token": "BUB_TELEGRAM_TOKEN",
    "telegram.allow_users": "BUB_TELEGRAM_ALLOW_USERS",
    "telegram.allow_chats": "BUB_TELEGRAM_ALLOW_CHATS",
    "telegram.proxy": "BUB_TELEGRAM_PROXY",
}

_PROVIDER_KEY = re.compile(r"^BUB_(.+)_API_(KEY|BASE)$")


def _secret(value: Any) -> str:
    """Flatten a possibly provider-keyed secret to one string for a text field.

    A per-provider ``api_key``/``api_base`` lives as a mapping; a single-line
    field can only show one of them, so the common provider is preferred and a
    mapping is shown as its first entry. Saving writes a plain string back.
    """
    if isinstance(value, dict):
        for provider in ("openai", "openrouter", "anthropic", "google"):
            if provider in value:
                return str(value[provider])
        return str(next(iter(value.values()), ""))
    return "" if value is None else str(value)


def _steps(value: int) -> str:
    """Show the "no limit" sentinel as blank, so the field reads as unset."""
    return "" if value >= sys.maxsize else str(value)


def snapshot(framework: Any) -> dict[str, Any]:
    """One effective view of every key the settings page edits."""
    agent = load_settings()
    channels = ensure_config(ChannelSettings)
    telegram = ensure_config(TelegramSettings)
    return {
        "model": agent.model,
        "command_prefix": agent.command_prefix,
        "fallback_models": list(agent.fallback_models or []),
        "api_key": _secret(agent.api_key),
        "api_base": _secret(agent.api_base),
        "max_tokens": agent.max_tokens,
        "max_steps": _steps(agent.max_steps),
        "model_timeout_seconds": agent.model_timeout_seconds,
        "enabled_channels": channels.enabled_channels,
        "stream_output": channels.stream_output,
        "telegram.token": telegram.token,
        "telegram.allow_users": telegram.allow_users or "",
        "telegram.allow_chats": telegram.allow_chats or "",
        "telegram.proxy": telegram.proxy or "",
    }


def overridden() -> list[str]:
    """Keys the environment supplies, which a config write cannot change."""
    marked = [key for key, env in _AGENT_ENV.items() if env in os.environ]
    for name in os.environ:
        if match := _PROVIDER_KEY.match(name):
            key = "api_key" if match.group(2) == "KEY" else "api_base"
            if key not in marked:
                marked.append(key)
    return marked


def config_file(framework: Any) -> str:
    path = getattr(framework, "config_file", None)
    return str(path) if path is not None else ""


def home(framework: Any) -> str:
    del framework
    return str(bub.home)


def _assign(patch: dict[str, Any], dotted: str, value: Any) -> None:
    """Place ``value`` at a dotted path, creating the nesting it needs."""
    parts = [part for part in dotted.split(".") if part]
    if not parts:
        raise ValueError("settings key must not be empty")
    node = patch
    for part in parts[:-1]:
        child = node.get(part)
        if not isinstance(child, dict):
            child = {}
            node[part] = child
        node = child
    node[parts[-1]] = value


def persist(framework: Any, values: dict[str, Any]) -> None:
    """Merge ``values`` into Bub's config file and write it back.

    Raises whatever :func:`bub.configure.save` raises — a bad value fails
    validation there rather than reaching the running runtime.
    """
    patch: dict[str, Any] = {}
    for dotted, value in values.items():
        _assign(patch, dotted, value)
    data = configure.get_config_data()
    configure.merge(data, patch)
    configure.save(Path(framework.config_file), data)
