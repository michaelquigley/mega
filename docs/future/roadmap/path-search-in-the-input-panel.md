---
title: path search in the input panel
state: evaluating
created: 2026-11-10
tags: [feature, spike]
---

Typing `@` in the reply box opens a search over paths in the conversation's checkout; picking one inserts the path into the prompt. The last open item of the operator input panel epic.

## shape

- **In the box.** `@` at the start of a word opens a small list above the caret; further typing filters it; Up, Down and Enter pick; Escape or a space closes it and leaves the text as typed. It works anywhere in a draft, any number of times, and never steals a Ctrl+Enter.
- **What it searches.** Files git tracks plus untracked files that are not ignored, and the directories those imply, relative to the checkout. Ignored files (build output, dependencies) never appear. This is the planning agent's recommendation, so that a file an agent created a minute ago can be pointed at; Michael has not confirmed it, and tracked-only is the alternative.
- **Matching.** Case-insensitive, each typed fragment must appear in order, with basename matches ranked above directory matches and shorter paths above longer. At most twenty results.
- **Where the list comes from.** A new read-only endpoint on the service, `GET /api/projects/{id}/paths?q=`, answered from `git ls-files --cached --others --exclude-standard` run in the checkout and cached briefly. A checkout that is not a git repository falls back to a bounded walk that skips dot-directories. It lists names only, never reads file contents, and never leaves the checkout through a symlink.
- **What is inserted.** The path relative to the checkout. Whether to keep the leading `@` depends on the probe below.

## spike

Whether each harness treats `@path` in a programmatic prompt as a file mention, as their terminals do: send `@fixture.txt` through the Claude shim and through Codex `turn/start` and see whether the file's content is attached or the text arrives literally. If a harness expands it, insert `@path` for that harness; otherwise insert the bare path. Record the observation in the capability report with versions.

## ride-along

From the failed-load fix: when the service answers a workspace read with an error, the never-loaded page says the service could not read the workspace, with the error, instead of "unreachable". "Unreachable" is kept for when nothing answered. The retry is unchanged.

## checks

A service test for the listing against a fixture with tracked, untracked and ignored files and a symlink pointing outside; a server test for the endpoint and its result cap. The browser check with the fake shim: the list opening on `@`, filtering, keyboard picking, Escape leaving the text alone, two mentions in one draft, and the inserted text per harness.
