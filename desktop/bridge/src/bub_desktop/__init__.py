"""Bub desktop bridge plugin.

The plugin turns Bub into a headless process a desktop front-end can drive: it
registers a :class:`~bub_desktop.channel.DesktopChannel` (a normal Bub
``Interface`` channel) and a hidden ``bub desktop`` command that serves the
newline-delimited JSON protocol over stdio.
"""

from bub_desktop.plugin import BubDesktopPlugin

__all__ = ["BubDesktopPlugin"]
