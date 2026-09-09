# About this fork

This is a fork of [`arnodel/golua`](https://github.com/arnodel/golua) carrying three
sandbox-hardening fixes found while embedding golua in Helios Advance BBS, which runs
untrusted sysop-authored Lua under per-execution CPU, memory and wall-clock ceilings.

Nothing here is embedder-specific: the module path in `go.mod` is still
`github.com/arnodel/golua`, no embedder code or naming appears in the patched files, and
every branch below applies cleanly to upstream `lua5.5`. Nothing has been filed against
`arnodel/golua`. The fixes are deliberately kept in a submittable shape, so anyone who
wants to take them upstream can, and the maintainer is welcome to lift any branch.

## Branches

| Branch | Contents |
| --- | --- |
| `lua5.5` | Unmodified upstream default branch. |
| `hadv-quota-accounting` | One commit: quotas were escapable through `pcall`. |
| `hadv-per-runtime-base-iterators` | One commit: `lib/base` wrote the compliance flags of two process-wide `*rt.GoFunction` objects on every `Load`. |
| `hadv-coroutine-end-race` | One commit: a finishing coroutine touched the shared context manager after handing control back. |
| `hadv` | The three above merged. This is the branch the embedder pins, by commit. |

The fix branches are deliberately separate: the defects are independent, and any one can
be taken upstream without the others. Read the commit messages for the full analysis of
each. They are written for a reviewer who has not seen the reproduction.

## Severity, briefly

**Quota accounting.** `requireCPU`/`requireMem` recorded consumption only on the branch
where the limit was not breached, so a context killed for exceeding its budget reported
consuming nothing, and `PopContext` charged its parent nothing. Since `pcall` runs its
argument in a child context, `while true do pcall(f) end` restarted from a clean budget
on every iteration. Under a 256 KB ceiling a loop requesting 200 MB per iteration ran
1778 times before anything stopped it; it now stops after one. Any embedder relying on
golua's quotas to bound untrusted Lua is affected.

**Shared `lib/base` iterators.** `ipairs`'s iterator and `next` are package-level, so
`Load` writing their `safetyFlags` raced every other `rt.Runtime`'s reads of the same
field in `GoCont.RunInThread`. Every writer wrote the same constant, so no wrong flag set
was reachable, which makes this race hygiene at a sandbox boundary rather than a live
functional bug. It affects any embedder building more than one `rt.Runtime` per process.
The flags are now declared once in `init()`, matching what `lib/runtimelib/ctx.go` already
does for its own package-level `GoFunction`s.

**Coroutine end.** `Thread.end` released the coroutine's 2K goroutine stack after
`sendResumeValues` had already woken the resuming thread, so that release ran alongside
the caller on the Runtime's shared `runtimeContextManager`. `ReleaseMem` reads
`hardLimits.Memory` and updates `usedResources.Memory` while `PushContext` writes both.
The mutexes held in `end` guard the two `Thread` structs, not the manager. This affects
any embedder that loads the coroutine library and uses quotas. It is not new: it
reproduces at `f3b8356`, the oldest commit on `lua5.5`.

## Regression coverage

Each fix ships with a test in this repository, verified to fail against the unpatched
tree:

| Fix | Test |
| --- | --- |
| Quota accounting | `lib/runtimelib/lua/quotaescape.quotas.lua` |
| Base iterators | `lib/base/lua/iterators.quotas.lua`, `lib/base/concurrency_test.go` |
| Coroutine end | `lib/coroutine/concurrency_test.go` |

The quota test covers both shapes the fix addresses: a plain `pcall` loop, which escapes
without the recording change, and a nested one, which escapes without the `PopContext`
reorder. Its nested case is bounded rather than `while true`, so a regression fails the
test in seconds instead of hanging it.

The two `concurrency_test.go` files only detect their defect under `-race`. Note that
`go test -race ./...` does not pass on `lua5.5` itself: the coroutine-end race is on that
branch too, and `hadv` is the first tree here where the whole suite is race-clean.

The embedder also pins the quota and iterator fixes with its own tests, which is where
they were first caught.

## Continuous integration

Each fix branch was run through this fork's `Go` workflow before being merged here, via a
draft pull request opened for that purpose and closed once green. That covers the Lua 5.5
conformance suite, macOS and Windows, Go 1.22 and `stable`, and the `GOLUA_POOL` and
`noquotas` build combinations, none of which a local run reaches.

## Rebasing onto newer upstream

Re-apply each patch against the new source rather than copying whole files over it. The
patch sites are small and upstream edits around them. `git log lua5.5..hadv` is the
authoritative list of what this fork changes; there are no marker comments to grep for,
because the comments at each site are written to read as ordinary upstream code.

## Licence

Unchanged: Apache-2.0, see `LICENSE`. The fixes are contributed under the same terms.
