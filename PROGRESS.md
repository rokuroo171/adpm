# PROGRESS

adpm 0.1.0 · spec: docs/superpowers/specs/2026-10-30-adpm-design.md

## Milestones

| # | Milestone | Done when | Status |
|---|-----------|-----------|--------|
| 1 | Passthrough | fake-binary tests pass, `adpm pacman -Q` runs with no ad | ✅ pass |
| 2 | Player MVP | ad plays at 80x24, 40x20, 20x6; terminal clean after finish, Ctrl+C, SIGTERM, panic | ✅ pass |
| 3 | Controls | key paths behave as specified, cancel exits 130 | ✅ pass |
| 4 | Image tiers | correct tier on kitty, WezTerm, Alacritty, TERM=linux | ✅ pass |
| 5 | Typo mode and init | `pamcan` lockout in bash/zsh/fish, `sudo pacman -S` shows the ad | ⬜ not started |
| 6 | Ads, errors, packaging | first ad ships via `make bake`, corrupted frame yields ADPM-415 | ⬜ not started |
| 7 | Release and docs | a v0.1.0 tag produces artifacts and an AUR update | ⬜ not started |

## Decisions

- `fit` ships as exported `render.Fit` so the player can use it; tests stay in the package.
- Layout tiers: under 20 cols or 6 rows one-line, under 40 or 12 compact, else fullscreen, so 20x6 renders compact.
- Dev ad: 30 synthetic JPEG frames (3s at 10fps, 320x180) in ads-dev/test/frames, loaded only when ADPM_DEV=1.
- Ad title hardcoded to `Synergy™` until the ad.toml loader lands in Milestone 6.
- Ctrl+C in M2 ends the player and proceeds to the install; popup and exit 130 are Milestone 3.
- No skip or report footer text yet, acceptance excludes Milestone 3 controls.
- One-line tier runs outside Bubble Tea (spec: "no TUI"); a shrink inside the TUI clamps back to compact.
- `.gitignore` adds `/adpm` for the Makefile build output next to the spec's three entries.
- Skip unlocks after `clamp(duration × 0.4, 2s, 5s)`; early Enter restarts the ad.
- Ctrl+C opens the reward popup with Continue selected; Skip aborts the pending install and exits 130.
- Report opens its popup, and Enter restarts the ad with a fresh countdown.
- `messages.toml` is embedded for the manager abort messages and popup copy.
- One-line mode reads raw key input and keeps the controls inline without the alt screen.
- `rasterm` encodes Kitty, Sixel, and iTerm2 frames; half-block uses 256 colors unless `COLORTERM` reports truecolor or 24bit.
- The terminal probe checks Kitty graphics and DA1 replies for up to 150ms; `TERM=linux`, `dumb`, and `NO_COLOR` select ASCII.
- Kitty images are cleared before drawing and during cleanup; graphic images are cleared while a popup is open.
- Raster popups overlay a cached, dimmed player grid; playback and its clock stay paused until the popup closes.

## Known issues

- Importing lipgloss/bubbletea makes termenv send an OSC 11 and cursor query at process start; a terminal that never answers stalls every run about 5s. Real terminals answer instantly. Worth silencing when Milestone 4 owns query timeouts.
- SIGTERM during play quits the player, then the install still runs, per the rule that adpm never breaks an install.
- Forced-panic cleanup is covered by a unit test; no env hook to force a live panic exists on purpose.
- One-line popup copy is clipped to the available terminal width.
- Kitty, WezTerm, and Alacritty were represented by response fixtures in tests; the manual emulator matrix remains to run.

## Next step

Milestone 5: add typo lockout, shell hooks, manager aliases, and the user-side sudo wrapper.
