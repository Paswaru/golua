# About this fork

This is a fork of [`arnodel/golua`](https://github.com/arnodel/golua) carrying two
sandbox-hardening fixes found while embedding golua in Helios Advance BBS, which runs
untrusted sysop-authored Lua under per-execution CPU, memory and wall-clock ceilings.

Both fixes are offered upstream. Nothing here is embedder-specific: the module path in
`go.mod` is still `github.com/arnodel/golua`, no embedder code or naming appears in the
patched files, and every branch below applies cleanly to upstream `lua5.5`.

## Branches

| Branch | Contents |
| --- | --- |
| `lua5.5` | Unmodified upstream default branch. |
| `hadv-quota-accounting` | One commit: quotas were escapable through `pcall`. |
| `hadv-per-runtime-base-iterators` | One commit: `lib/base` shared two `*rt.GoFunction` objects process-wide, racing their `safetyFlags`. |
| `hadv` | Both of the above merged. This is the branch the embedder pins, by commit. |

The two fix branches are deliberately separate: the defects are independent, and either
can be taken upstream without the other. Read the commit messages for the full analysis
of each — they are written for a reviewer who has not seen the reproduction.

## Severity, briefly

**Quota accounting.** `requireCPU`/`requireMem` recorded consumption only on the branch
where the limit was *not* breached, so a context killed for exceeding its budget reported
consuming nothing, and `PopContext` charged its parent nothing. Since `pcall` runs its
argument in a child context, `while true do pcall(f) end` restarted from a clean budget
on every iteration. Under a 256 KB ceiling a loop requesting 200 MB per iteration ran to
completion. Any embedder relying on golua's quotas to bound untrusted Lua is affected.

**Shared `lib/base` iterators.** `ipairs`'s iterator and `next` were package-level, so
`Load` writing their `safetyFlags` raced every other `rt.Runtime`'s reads of the same
field in `GoCont.RunInThread`. Every writer currently writes the same constant, so no
torn or missing flag set is reachable today — this is race hygiene at a sandbox boundary,
not a live functional bug. It affects any embedder building more than one `rt.Runtime`
per process, whether or not it uses quotas.

## Regression coverage

Neither fix ships with a test in this repository yet; both are pinned by tests in the
embedder, each verified to fail against the unpatched tree before its fix landed. The
quota fix is pinned by a test that runs a 50-iteration `pcall` loop requesting 200 MB an
iteration under a 256 KB ceiling and asserts the script is killed rather than running to
completion; the iterator fix by a `-race` test that constructs runtimes concurrently with
a running script looping over `ipairs`/`pairs`/`next`, whose racing frames are
`GoCont.RunInThread` and `GoFunction.SolemnlyDeclareCompliance`.

Porting equivalent tests into this repository's own suite is outstanding work. An
upstream PR should carry a test written to upstream's conventions rather than a copy of
the embedder's.

## Rebasing onto newer upstream

Re-apply each patch against the new source rather than copying whole files over it — the
patch sites are small and upstream edits around them. `git log lua5.5..hadv` is the
authoritative list of what this fork changes; there are no marker comments to grep for,
because the comments at each site are written to read as ordinary upstream code.

## Licence

Unchanged: Apache-2.0, see `LICENSE`. The fixes are contributed under the same terms.
