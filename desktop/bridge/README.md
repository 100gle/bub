# bub-desktop

Desktop bridge plugin for [Bub](https://bub.build). It exposes Bub to the GPUI
app under `../` over a newline-delimited JSON protocol on stdio.

Install it editable into the same environment as `bub`:

```sh
uv pip install -e desktop/bridge
```

Then `bub desktop` serves the bridge (hidden command) and the GPUI app spawns it
as a child process.
