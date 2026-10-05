# SDL fluid simulation through Joker FFI

This example renders an incompressible RGB dye simulation with SDL2 loaded at runtime. Every SDL call is declared in `fluid.joke` through `joker.ffi`; the small Go host owns the allocation-free solver and byte buffers.

![SDL fluid simulation](../../../docs/images/sdl-fluid.png)

## Run

Install an SDL2 runtime library. Linux screenshot testing also needs Xvfb; macOS and Windows use their native display.

```sh
make sdl-fluid
.cache/tmp/sdl-fluid -frames 0 -library /absolute/path/to/SDL2
```

Typical paths include `/usr/lib/x86_64-linux-gnu/libSDL2-2.0.so.0`, `/opt/homebrew/lib/libSDL2.dylib`, or an absolute path to `SDL2.dll`. Library and process architecture must match. The Linux default path is a convenience for the tested amd64 host, not a portable discovery mechanism.

Close the window to stop. A finite render saves the final SDL framebuffer:

```sh
make sdl-fluid-screenshot
```

The stored screenshot used a 96x96 grid, 240 fixed steps of 1/60 second and a 768x768 software-rendered window. Xvfb supplied the display. Native macOS/Windows fluid execution is not yet qualified. The host locks the process main thread for SDL video calls; it does not dispatch them from background workers.

## Structure

* `fluid.joke`: fixed ABI signatures, SDL window/renderer/texture creation, frame upload/render, event polling, screenshot readback and teardown.
* `main.go`: script runner, arguments, CPU/heap capture, simulation buffers and PNG writer. The `fluid.example` namespace is private to this example.
* `internal/fluid`: stable-fluids diffusion, pressure projection, semi-Lagrangian advection and deterministic coloured forcing. Persistent arrays are reused; steady-state solver/image conversion tests check zero allocations.

The script passes buffers directly to SDL, avoiding one Joker object per pixel and per-pixel FFI calls. The solver uses 18 iterative relaxation passes; profiling attributes most render CPU time to this linear solver. It is a visual simulation, not a validated computational-fluid-dynamics model. See [FFI contracts and limitations](../../../docs/FFI.md).

## Tests

```sh
make ffi-check
```

All Go test runs capture CPU/heap profiles and reports. Solver tests check finite/bounded fields, nontrivial colour output and numerical reproducibility using tolerances; the screenshot's exact bytes are not the acceptance gate. The render target captures profiles separately from any future uninstrumented frame-time comparison.
