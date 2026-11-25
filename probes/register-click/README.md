# The register dialog's typed path

Why the `dialogs` suite's step "secret registered" sometimes timed out, on a first full `make checks` on 2026-11-23 and twice on 2026-11-24, and then passed alone (card `the-dialogs-suite-crashes-on-a-first-full-run`).

`probe.mjs [TRIALS]` runs against a scratch mega through the checks' own library, on the checks' port, with its scratch in `<checks root>/register-click-probe`, against the committed fakes. Run it from the repository's root as the checks are run, after `make checks` has built the binary:

```sh
MEGA_CHECKS_DIR=/tmp/mega-checks MEGA_CHECK_BIN=/tmp/mega-checks/mega node probes/register-click/probe.mjs 40
```

It has four parts:

- **forced.** It does `fill`'s two steps by hand, with the dialog's first listing held. It selects the field, lets the listing go so that the prefill lands, then inserts the path. It says what the field held and what the service answered.
- **shift.** The listing of a folder outside the projects root is held while the mouse is pressed on Register, then let go before the release.
- **timing.** Once the field holds the projects root, as the suite now waits for, it says how long after the field's `input` event, which starts the listing's 80 ms wait, a Register click is pressed, and how long after the press it is released.
- **compare.** A controlled comparison, not a replay of the suite. It makes two registrations back to back, a folder in the root and then a path outside it, each typed in full: once the moment its dialog opens, and once after waiting for the field to hold the projects root. The suite picked its folder in the root through the list and typed only the outside path at once.

## 2026-11-24, at `0769a20` plus the working tree

- **The cause.** The dialog fills its field with the projects root (`~/Repos/`) when its first listing arrives, unless something has been typed by then. Playwright's `fill` selects the field's text and then inserts the new text, in two steps. When the root lands between them, the typed path is appended to it, and the service refuses `~/Repos//…/secret` as not accessible, with 400. The dialog stays open with the error and nothing is registered, so the suite timed out 20 seconds later. Each of the three crashes logged that 400 as a console error. The suite's first open waits for the root listing before typing; the second open, for the secret, did not.
- **Forced, every run.** The field holds the typed path after `~/Repos/`, and the service refuses it with 400, `~/Repos//…/Private/secret0`.
- **Compare, run by run.** Typing at once, the rate came and went with conditions on the machine: 10, 8 and 4 of 80 registrations refused in the first runs of the evening, 0 of 80 in five later runs (one of them with all 16 CPUs busy), and 1 of 20 in the last. Every one refused was sent with the typed path after `~/Repos/`. Waiting for the root first: none refused in any run.
- **Ruled out: a click lost to the listing moving the button.** The note that replaces "Looking…" moves Register down 11 px. Register is 28 px tall, so the point pressed stays on the button, and the held click registered. The press also comes 28 to 50 ms after the field's `input` event, before the listing's 80 ms wait has run, with the release 0.5 to 2.2 ms after the press; under load, up to 72 ms and 5.8 ms.
- **Ruled out: load alone.** Ten runs of the suite alone with all 16 CPUs kept busy passed. So did the first full run after the reboot.
- **Ruled out: the registration waiting under the service lock.** The background resolution a registration starts takes its slot inside its own goroutine, not under `s.mu`.

`checks/dialogs.mjs` now waits, in `openRegister`, for the field to hold the projects root before anything is typed.
