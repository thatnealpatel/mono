# srvfs design language

srvfs renders a directory of markdown files in the browser with
opinionated formatting and a file tree, ephemerally — nothing is written
to disk. it inherits the patel.codes design language; this document is the
authority for keeping srvfs consistent when editing it.

the entire ui lives in one go raw-string template (`pageHTML` in
`main.go`): one `<style>` block and one `<script>` block. no build step,
no framework, no external css/js.

## principles

- minimalism: one binary, one positional directory argument, one template.
- monospace everything: PT Mono, including the file tree and chrome.
- lowercase all chrome copy (`files`, `select a file`, `no markdown
  files`). rendered markdown is shown verbatim — never transform content.
- content-first display zone; the file tree is the only chrome.
- colors come only from the palette tokens — never hardcode a hex value.

## palette

defined once on `:root`; identical to patel.codes. never redefine these.

| token         | value     | usage                                   |
|---------------|-----------|-----------------------------------------|
| `--paper`     | `#fafafa` | page and zone background                |
| `--ink`       | `#2c2b28` | headings, emphasis, active, focus line  |
| `--ink-light` | `#6b6963` | body copy, tree entries                 |
| `--ink-faint` | `#a8a49c` | labels, rules, borders, directory names |
| `--accent`    | `#f8d551` | active underline, selection             |

- reference colors only via `var(--token)`.
- derivations must stay token-only, e.g. `color-mix(in srgb,
  var(--token) N%, var(--token))`.
- raw hex is permitted only in canvas js / svg `fill` (none today).

## typography

- single typeface: PT Mono, loaded via google fonts `@import`, set through
  `--font-family`. introduce no other typeface.
- base size 19px, line-height 1.7.
- PT Mono is single-weight: do NOT signal hierarchy with `font-weight`.
  use color (`--ink` > `--ink-light` > `--ink-faint`), size, underline,
  and the accent instead.

## css authority

- all `border-radius` is `0`.
- the inline `<style>` block is the single source of truth. markdown
  element styling is scoped under `#content`; tree styling under `#tree`.
  keep new rules scoped to a zone or a namespaced class.
- when a new component is needed, add a class composing existing tokens;
  do not invent colors or restyle shared elements ad hoc.

## zones

the page has exactly two zones:

- `#tree` — fixed left sidebar; the rooted, nested file tree. lists only
  markdown files and the directories that (transitively) contain them.
  hidden entries (dotfiles) are skipped; directories sort before files.
- `#content` — the display zone; rendered markdown is injected here.

exactly one zone is focused at a time, marked by the `.focused` class and
shown as a thin vertical rule (2px, `--ink`) along its left edge. default
focus is the tree.

## keyboard model

- `tab` toggles focus between the two zones.
- tree focused:
  - `j` / `k` — select next / previous file (clamped at the ends);
    selecting a file loads it into the display zone.
  - `h` / `l` — jump to the first file of the previous / next directory
    group, wrapping around.
- content focused:
  - `j` / `k` — scroll the document down / up.
  - `h` / `l` — unused.

keep bindings vim-flavored and zone-scoped. plain keys only; ignore the
event when a modifier (ctrl / alt / meta) is held.

## markdown rendering

- rendered with `rsc.io/markdown`. the github-flavored feature set is on
  (heading ids, strikethrough, task lists, tables, autolinks, emoji,
  footnotes). smart-punctuation transforms stay OFF — output is faithful
  to the source.
- no latex / mathml. (patel.codes renders math at build time; srvfs
  deliberately does not.)

## architecture constraints

- single `main.go`; one positional argument, the directory to serve.
- ephemeral: render on request, in memory, over http. never write
  generated files to disk.
- `/` serves the shell with the server-rendered tree; selecting a file
  fetches `/api/render?file=` and swaps `#content` client-side. other
  paths serve raw assets from the tree.
- vanilla js only, inline in the template. the template is a go raw
  string: it must contain NO backticks — use single quotes and `+`, never
  template literals.
- security: reject path traversal and hidden path components; skip hidden
  entries when walking the tree.

## editing checklist

- new color? use an existing token, or a `color-mix` of tokens — never hex.
- new chrome text? lowercase it.
- need emphasis? use color or underline, not weight.
- added js? no backticks; keep it framework-free and inline.
- changed rendering? keep it faithful (no source-mutating transforms) and
  latex-free.
- run `go test ./...` — it also verifies the template still parses.
