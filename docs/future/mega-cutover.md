# Cutover: roundhouse becomes mega

A one-time checklist for Michael to run by hand, drafted 2026-11-11. It moves the live workspace, the checkout and Claude's session store to their new names in one sitting. No agent can do this, because the agents live inside the workspace being moved. Retire this file once the cutover has held for a few days.

Nothing here is destructive until the last step: every move is a rename on one filesystem, and the first step makes copies.

## Before starting

- Phase A is committed: the tree builds a `mega` binary and its tests pass.
- The Phase A probe confirmed that a Claude session resumes by exact id after its store directory is moved to the new path's name. It did, on 2026-11-11 with Claude Code 2.1.318: history intact, same session, no second store created (`probes/claude/moved-store.mjs`, recorded in the capability report).
- No terminal has an agent open in this checkout.

## Steps

1. **Bring the workspace to rest.** In roundhouse, arm "shut down when idle", let every agent finish, answer what is waiting, and shut down from the banner. Confirm nothing is listening:

   ```bash
   ss -ltn | grep 8470 || echo "port free"
   ```

2. **Back up the workspace and its config.**

   ```bash
   cp -a ~/.local/share/roundhouse ~/.local/share/roundhouse.bak-$(date +%F)
   [ -d ~/.config/roundhouse ] && cp -a ~/.config/roundhouse ~/.config/roundhouse.bak-$(date +%F)
   ```

3. **Move the workspace and its config to the new names.**

   ```bash
   mv ~/.local/share/roundhouse ~/.local/share/mega
   [ -d ~/.config/roundhouse ] && mv ~/.config/roundhouse ~/.config/mega
   [ -f ~/.config/mega/roundhouse.yaml ] && mv ~/.config/mega/roundhouse.yaml ~/.config/mega/mega.yaml
   ```

4. **Move the checkout.**

   ```bash
   mv ~/Repos/q/research/roundhouse ~/Repos/q/products/mega
   ```

5. **Point the workspace's own project at the new path.** The pattern is anchored to the end of the line so the two fixture checkouts, whose paths begin with the same text, are left alone. Check the result before going on.

   ```bash
   sed -i 's#^\(\s*checkout: \)/home/michael/Repos/q/research/roundhouse$#\1/home/michael/Repos/q/products/mega#' ~/.local/share/mega/registry.yaml
   grep -n "checkout:" ~/.local/share/mega/registry.yaml
   ```

   Then prove every registered path is a real directory. On the first attempt, 2026-11-11, this project's path was set by hand to `research/mega` instead of `products/mega`; nothing complained until Claude refused to start.

   ```bash
   grep -oP '^\s*checkout: \K.*' ~/.local/share/mega/registry.yaml | while read -r d; do [ -d "$d" ] || echo "MISSING: $d"; done
   ```

6. **Move Claude's session store for the checkout.** This directory holds this project's two conversations and the planning agent's memory.

   ```bash
   mv ~/.claude/projects/-home-michael-Repos-q-research-roundhouse ~/.claude/projects/-home-michael-Repos-q-products-mega
   ```

7. **Tell the other tools about the new path.**

   - ranger: change the `root:` line for this project in `~/.config/ranger/config.yaml` to `~/Repos/q/products/mega`.
   - Codex: change the `[projects."/home/michael/Repos/q/research/roundhouse"]` heading in `~/.codex/config.toml` to the new path. The fixture entry below it stays.
   - Claude's trust is recorded per path, so mega will show the checkout as untrusted until `claude` has been opened there once and the dialog accepted.

8. **Rename the project in the canon.**

   ```bash
   cd ~/Repos/q/products/terminus-canon
   git mv projects/roundhouse projects/mega
   sed -i 's/^  name: roundhouse$/  name: mega/; s/^  repo: roundhouse$/  repo: mega/; s#projects/roundhouse/#projects/mega/#' projects/mega/rubric.yaml
   sed -i 's/^  - roundhouse$/  - mega/' projects/mega/*.md
   terminus rubrics --repo ~/Repos/q/products/mega
   ```

   The last command should list twenty qualities. The planning agent will sweep the quality texts for the old name afterwards.

9. **Publish.** Create the empty repository `michaelquigley/mega` on GitHub, then:

   ```bash
   cd ~/Repos/q/products/mega
   git remote set-url origin git@github.com:michaelquigley/mega.git
   git push -u origin main
   ```

10. **Build and start.**

    ```bash
    cd ~/Repos/q/products/mega
    make
    mega
    ```

    `make` installs `mega` into the Go bin directory beside the old `roundhouse` binary. If `mega` is started before step 3 has run, it refuses and names this document; that guard is deliberate.

## Verify

- Six projects and nine conversations are present, with their histories.
- The project formerly named roundhouse shows the new checkout path. Rename it in the page if wanted.
- In that project, Start one conversation without a prompt, then send it a prompt. The session id in the header is unchanged from before the move.
- The fixture projects still open.
- A terminus review of a small change resolves the `mega` rubric.
- Panel width, the collapsed state, folded projects and the reply box's height start fresh once: the browser keys were renamed with the project. Stars are workspace state and come across.

## If something is wrong

Stop `mega`, then undo in reverse order: move the checkout, the session store, the config and the workspace back to their old names, and restore the registry from the backup. The old `roundhouse` binary is still in the Go bin directory until it is removed, and it runs against the old names.

## After it has held

- Remove the two `.bak` directories and the old binary: `rm "$(go env GOPATH)/bin/roundhouse"`.
- Archive the repository on git.hq.
- Delete this file and the roadmap card, with a line in the journal.
