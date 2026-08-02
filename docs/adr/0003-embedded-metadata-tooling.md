# Embedded metadata tooling by container

Embedded metadata is a different job from technical bindings. Tools are chosen per container so write and compare stay on the same format family:

- **MKV** — write with `mkvpropedit`, read/compare with `mkvextract`
- **MP4 / MOV / M4V** — write with `ffmpeg` (stream-copy to a temp file that atomically replaces the original), read/compare with `ffprobe`

MediaInfo stays out of this path; it only supplies naming-oriented technical metadata (ADR 0001). Writes leave no leftover backup media files; undoing embedded-metadata changes is tracked in #25.
