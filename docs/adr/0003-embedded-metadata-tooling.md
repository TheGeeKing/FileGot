# Embedded metadata tooling by container

Embedded metadata is a different job from technical bindings. Tools are chosen per container so write and compare stay on the same format family:

- **MKV** — write with `mkvpropedit`; read/compare Tags XML with `mkvextract`; read segment title with `mkvmerge -J`
- **MP4 / MOV / M4V** — write with `ffmpeg` (stream-copy to a temp file that atomically replaces the original; 30s tool timeout), read/compare with `ffprobe` (10s). MKV probe/edit tools also use a 10s timeout. A lighter in-place MP4 writer is tracked separately.

MediaInfo stays out of this path; it only supplies naming-oriented technical metadata (ADR 0001). Writes leave no leftover backup media files; undoing embedded-metadata changes is tracked in #25.
