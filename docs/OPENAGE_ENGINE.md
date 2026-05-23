# OpenAge Engine Demo Path

OpenAge is included as a Git submodule at:

```txt
vendor/openage
```

Pinned revision at time of addition:

```txt
865bd548
```

## Why submodule, not copied source

We keep OpenAge as a submodule so the OpenZerg repo stays small and clean. The engine can be updated or removed independently without vendoring a large upstream codebase into our history.

## Checkout instructions

After cloning OpenZerg:

```bash
git submodule update --init --recursive
```

If the submodule already exists and you want to update it later:

```bash
git -C vendor/openage pull
```

Then commit the changed submodule pointer intentionally.

## Build caveat

OpenAge is a C++20/Python/Cython/Qt6/OpenGL engine. It is not a drop-in browser dependency and it does not ship copyrighted Age of Empires assets.

From OpenAge's README:

- The engine uses original game assets but does not ship them.
- To play, you need original AoE/AoE2/Definitive Edition assets and conversion.
- OpenAge currently notes that gameplay is basically non-functional while engine simulation work is ongoing.

So our realistic demo path is:

1. Keep `prototypes/openage-evolution.html` as the browser-ready pitch/demo surface.
2. Use `vendor/openage` as an optional experimental engine track.
3. If time allows, build a separate engine-backed scene or capture using OpenAge tooling/assets.
4. Do not make the hackathon pitch depend on OpenAge compiling/running live.

## Upstream docs

- `vendor/openage/README.md`
- `vendor/openage/doc/building.md`
- `vendor/openage/doc/media_convert.md`
- `vendor/openage/doc/build_instructions/docker.md`
