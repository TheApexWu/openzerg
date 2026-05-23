# OpenAge Engine Demo Path

OpenAge is included as a Git submodule at:

```txt
vendor/openage
```

The official free OpenAge data repo is included as a second submodule at:

```txt
vendor/openage-data
```

Pinned revisions at time of addition:

```txt
openage:      865bd548
openage-data: 7a1beff
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

## Asset choice

We are using the official free OpenAge asset repository:

```txt
https://github.com/SFTtech/openage-data
```

That repo is intended to eventually replace proprietary Genie/AoE media and is licensed under CC-BY-SA/GPL terms. We are **not** using AoE2 assets because the team does not own them locally, and we should not download unofficial copyrighted packs.

## Build caveat

OpenAge is a C++20/Python/Cython/Qt6/OpenGL engine. It is not a drop-in browser dependency.

From OpenAge's README:

- The engine can use original game assets, but does not ship them.
- OpenAge currently notes that gameplay is basically non-functional while engine simulation work is ongoing.
- `openage-data` provides free assets, but may not create a complete playable AoE-like experience by itself.

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
- `vendor/openage-data/README.md`
- `vendor/openage-data/copying.md`
