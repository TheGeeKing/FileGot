# FileGot roadmap

GitHub Issues are the source of truth for feature scope and acceptance
criteria. This document records the approved order, dependencies, and
cross-feature decisions without duplicating each specification.

Every blocking `Depends on` entry appears in the issue description and
GitHub's native dependency graph. These independently completable features do
not use a parent or sub-issue hierarchy.

## Deferred features

| Issue | Capability | Goal | Depends on | Status |
| --- | --- | --- | --- | --- |
| [#1](https://github.com/TheGeeKing/FileGot/issues/1) | Multi-episode files | Match and safely name one file containing several episodes | — | Ready |
| [#2](https://github.com/TheGeeKing/FileGot/issues/2) | Library organization | Move matched media into fixed movie, series, and season layouts | — | Ready |
| [#3](https://github.com/TheGeeKing/FileGot/issues/3) | Subtitles and sidecars | Move conservatively associated companion files with their video | #2 | Blocked |
| [#4](https://github.com/TheGeeKing/FileGot/issues/4) | Headless CLI | Preview and explicitly apply core workflows without Fyne | #2 | Blocked |
| [#5](https://github.com/TheGeeKing/FileGot/issues/5) | Watch folders | Organize stable files through a polling CLI worker | #4 | Blocked |
| [#6](https://github.com/TheGeeKing/FileGot/issues/6) | Disk integrity | Create and verify portable manifests for corruption detection | #4 for CLI only | Standby |
| [#7](https://github.com/TheGeeKing/FileGot/issues/7) | TMDB artwork | Preview posters and optionally save non-destructive library artwork | #2 | Blocked |
| [#8](https://github.com/TheGeeKing/FileGot/issues/8) | Technical metadata | Expose optional `ffprobe` details and quality naming tokens | — | Ready |
| [#9](https://github.com/TheGeeKing/FileGot/issues/9) | TVmaze provider | Add a credential-free TV provider and the first provider seam | — | Ready |
| [#10](https://github.com/TheGeeKing/FileGot/issues/10) | TheTVDB provider | Add user-configured movie and TV metadata | #9 | Blocked |
| [#11](https://github.com/TheGeeKing/FileGot/issues/11) | AniDB provider | Add anime matching and absolute episode numbering | #9; reuses #1 | Blocked |
| [#12](https://github.com/TheGeeKing/FileGot/issues/12) | Advanced templates | Add safe conditional filename templates | — | Ready |
| [#13](https://github.com/TheGeeKing/FileGot/issues/13) | Visual naming editor | Compose advanced templates with Scratch-like blocks | #12 | Blocked |

## Dependency order

- Start independently with #1, #2, #8, #9, or #12.
- After #2, work on #3, #4, or #7.
- After #4, work on #5; #6 remains on standby until explicitly activated.
- After #9, work on #10 or #11.
- After #12, work on #13.

Closing a blocker does not automatically schedule its dependants. Move only
the selected next issue into the ready queue.

## Agreed boundaries

- All filesystem changes retain full-batch preflight, overwrite refusal,
  versioned journaling, rollback, startup recovery, and durable undo.
- Library organization initially supports same-filesystem moves only.
  Cross-volume copy, verification, and deletion require a separate design.
- Companion matching is conservative and never guesses between multiple
  possible videos.
- Artwork is optional, never overwrites existing files, and cannot invalidate
  an otherwise successful media operation.
- Checksums primarily detect silent disk corruption. They remain explicit and
  do not automatically gate Rename or Organize.
- Technical inspection uses an optional `ffprobe` process. FileGot does not
  bundle binaries, transcode, or edit streams.
- The TV provider interface is introduced by #9, when it has two real
  implementations. The movie interface is introduced by #10.
- Providers remain explicit selections. FileGot does not silently merge or
  fall back across metadata sources.
- Existing simple naming patterns remain the default. Advanced templates are
  constrained `text/template` expressions with no I/O or arbitrary code.
- The visual naming editor compiles into the advanced template engine rather
  than creating another formatting implementation.

## Still out of scope

- Subtitle downloading or embedded subtitle editing
- Cross-volume organization
- Transcoding, remuxing, or stream modification
- Cloud and remote filesystem libraries
- Music, photo, and generic-file renaming
- Arbitrary scripting, executable plugins, or programmable directory layouts
