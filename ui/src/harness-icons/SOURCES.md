# Harness icons

Each file here is a vendor's own mark, copied unmodified from the vendor's published brand assets; the table says where each came from. Each is used only to say which harness a conversation runs on, as the control bar's leftmost element. None is used for mega's own identity: not the favicon, the README header or the About panel. The marks belong to their owners, and their use here implies no endorsement.

The harness table, `ui/src/harnesses.ts`, names each file. Taking one down is deleting its file and its `icon` line; the harness then shows its name in words.

| File | Harness | Source | Taken |
| --- | --- | --- | --- |
| `claude-code.svg` | Claude Code | Anthropic's press kit, `https://www.anthropic.com/press-kit`, which redirects to `https://www-cdn.anthropic.com/ae59ca4ca194dac9c9dc3bc78c5829468cb0e8af.zip` (SHA-256 `c68ac92df86c825f95177e24016fcc9a8863a3fd4ca344fe6f0700b2c1e07151`). In it, `Anthropic media resources/Anthropic logos/Claude logos/3 Claude Spark/SVG/Claude Spark - Clay.svg`: the Claude Spark, a single-colour symbol. The kit's Claude Code logo is a wordmark too wide for the bar. File SHA-256 `6d53db4be375e899c937c26cf16684a80d6e869b1928d72b37748bef2560e219`. | 2026-11-12 |
| `codex.svg` | Codex | OpenAI's Blossom mark in black, `OAI_OpenAI-Blossom_Black.svg`, named as OpenAI's brand kit names it. Michael supplied it on 2026-11-12, after `https://openai.com/brand/` refused automated fetches, so its download was his, not mega's. It replaced an earlier file from SVG Repo, which did not meet the rule. The file is one colour, black, with OpenAI's clear space around the mark, which spans about half its 716-unit box. The harness table marks it `iconMono`, so the bar draws its shape in the text colour, and `iconScale: 2`, so the mark fills the icon. File SHA-256 `75c1e9fffa5e8c437bec1d67197a73992bca45d166c6ff23215185dea8fae92a`. | 2026-11-12 |
