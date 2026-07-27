# Issue #8: MediaInfo vs ffprobe

## Question and decision

This comparison treats both engines on their own merits, then applies FileGot's
actual requirement: expose FileBot-compatible technical bindings, including raw
`media`, `video`, `audio`, `text`, `chapters`, `image`, and `menu` objects.

**Decision: use MediaInfo as the sole metadata backend for issue #8.**

ffprobe is materially stronger for packet/frame analysis, payload hashes,
bounded interval reads, and decoder-level investigation. Those are valuable
diagnostic features, but they are not naming inputs. MediaInfo is materially
stronger for normalized, human-facing technical fields and is the exact semantic
source used by FileBot. A dual backend would add packaging, execution, mapping,
fallback, and test paths without improving the requested naming contract.

## Decision matrix

`High` confidence means the difference is explicit in first-party documentation
or source. `Medium` means the capabilities are verified but no common test corpus
establishes an overall winner.

| Area | MediaInfo | ffprobe | Edge | Confidence |
| --- | --- | --- | --- | --- |
| FileBot parity | FileBot directly projects MediaInfo stream kinds and field names | Requires a translation and normalization layer | **MediaInfo** | High |
| Container / stream coverage | Broad audiovisual, image, subtitle, archive, and professional-format parser coverage | Broad demuxer, protocol, codec, subtitle, data, and attachment coverage | **Unknown / corpus-dependent** | Medium |
| Normalized technical fields | Stable names across containers; extensive codec/profile, commercial-name, HDR, channel, and tag fields | Mostly exposes libavformat/libavcodec's native format, codec, stream, tag, and side-data model | **MediaInfo** for naming | High |
| HDR / Dolby Vision summary | Dedicated `HDR_Format*` family, compatibility and commercial-name fields | Exposes mastering/light metadata and Dolby Vision/HDR side data; can inspect changing per-frame metadata | **MediaInfo** for labels; **ffprobe** for deep/frame data | High |
| Chapters / menus | Native `Menu` kind; FileBot builds `chapters` from it | Native `CHAPTER` sections and program/stream-group sections | Tie for chapters; **MediaInfo** for FileBot menu shape | High |
| Attachments / images / cover art | Native `Image` kind plus cover presence, MIME type, description, and optional Base64 data | Attachment stream type and attached-picture/video distinction | **MediaInfo** for naming-friendly image maps; ffprobe remains capable | High |
| Tags | Large cross-container normalized tag vocabulary | Prints raw container and stream tags, selectable by key | **MediaInfo** for normalized access; **ffprobe** for raw fidelity | High |
| Packet / frame inspection | Summary-level parser; no equivalent documented packet/frame enumeration contract | `-show_packets`, `-show_frames`, counts, frame/subtitle sections, side data | **ffprobe** | High |
| Interval reads | Partial-buffer SDK exists, but no equivalent time/packet interval query was found | `-read_intervals` supports time ranges and packet counts | **ffprobe** | High |
| Hashes / payload dumps | Cover extraction and summary fields; no documented packet/extradata hash equivalent found | Hex/Base64 payload dumps and `-show_data_hash` for packets/extradata | **ffprobe** | High |
| Corruption / decoder diagnostics | Primarily identification and summary; MediaConch is the separate MediaArea validation product | Can open decoders, enumerate frames, apply decoder AVOptions, and emit `ERROR` sections | **ffprobe**, though neither is a full validator | Medium |
| Standards conformance | MediaInfo itself is not MediaConch; MediaConch provides limited-format implementation validation and policy checks | ffprobe reports probe/decode facts and errors, not standards-conformance verdicts | **Neither** | High |
| Machine output | Exhaustive MediaInfo pivot in JSON/MIXML; field names are relatively stable but compatibility is not guaranteed | JSON/XML/flat/INI/CSV writers with explicit nested sections and optional-field controls | Tie; both require tolerant parsing and version fixtures | High |
| Performance | No fair first-party comparative benchmark found | No fair first-party comparative benchmark found | **Unknown: benchmark FileGot's corpus** | Low |
| CLI deployment | Standalone CLI available; library also exposes stream-kind/ordinal/field access | Standalone `ffprobe`; libraries expose much broader C APIs | Tie for subprocess use | High |
| License / redistribution | BSD-style license with attribution; alternative licenses offered | LGPL 2.1+ by default; optional build parts change the result to GPL and official redistribution guidance is longer | **MediaInfo** | High |
| Go integration | `os/exec` + JSON is dependency-free; native library means C ABI/CGO and shipped native binaries | Same subprocess path; native libav* means C ABI/CGO and more API surface | Tie for CLI; **MediaInfo** is the smaller semantic adapter | Medium |

## Primary-source evidence

### FileBot compatibility

FileBot documents `media` as one `[String:String]` map; `video`, `audio`, and
`text` as lists of maps; and `chapters` and `image` as maps:
[FileBot binding reference](https://www.filebot.net/naming.html).

FileBot source proves these are MediaInfo projections rather than a probe-neutral
abstraction:

- `media` → first `StreamKind.General`
- `video` → all `StreamKind.Video`
- `audio` → all `StreamKind.Audio`
- `text` → all `StreamKind.Text`
- `chapters` → chapter-like entries from `StreamKind.Menu`
- `image` → first `StreamKind.Image`
- `menu` → first `StreamKind.Menu`

The same source derives convenience bindings from MediaInfo keys such as
`Format`, `Encoded_Library_Name`, `CodecID/Hint`, `OverallBitRate`, `FrameRate`,
`SamplingRate/String`, `Duration`, and `Language`:
[FileBot `MediaBindingBean`](https://www.filebot.net/docs/api/src-html/net/filebot/format/MediaBindingBean.html).

### MediaInfo strengths

MediaInfo's SDK exposes stream kinds and ordinals plus lookup by parameter name:
[SDK quick start](https://mediaarea.net/en/MediaInfo/Support/SDK/Quick_Start) and
[SDK details](https://mediaarea.net/en/MediaInfo/Support/SDK/More_Info).

Its field dictionary explicitly says MediaInfo normalizes audiovisual
information so users do not depend on the analyzed container's terminology. It
includes:

- dedicated General, Video, Audio, Text, Other, Image, and Menu groups;
- `HDR_Format`, commercial name, version, profile, level, settings, and
  compatibility fields;
- cover presence, description, type, MIME type, and Base64 data;
- detailed channel, codec, bitrate, frame-rate, color, tag, and time-code
  fields.

Source: [MediaInfo fields](https://mediaarea.net/en/MediaInfo/Support/Fields).
Supported parser coverage is listed separately:
[MediaInfo supported formats](https://mediaarea.net/en/MediaInfo/Support/Formats).

MediaInfo's exhaustive pivot is available as JSON or MIXML. Its authors call the
output relatively stable but explicitly do not guarantee cross-version
compatibility, so FileGot must accept missing/new keys and fixture-test the
distributed version:
[MediaInfo output mapping notes](https://mediaarea.net/en/MediaInfo/Support/Fields#Information_about_the_mappings).

MediaInfo and MediaInfoLib use a permissive redistribution license with an
attribution condition and offer alternate licenses:
[MediaInfo license](https://github.com/MediaArea/MediaInfo/blob/master/License.html).

### ffprobe strengths

ffprobe emits machine-readable information about containers and each media
stream. Its output is organized into nested sections, supports JSON, XML, flat,
INI, compact, and CSV writers, and includes container/stream tags:
[ffprobe documentation](https://ffmpeg.org/ffprobe.html).

The same official documentation exposes capabilities beyond MediaInfo's naming
role:

- `-show_packets`, `-show_frames`, `-count_packets`, and `-count_frames`;
- `-read_intervals` by time or packet count;
- packet payload / codec extradata dumps and `-show_data_hash`;
- stream selection, precise entry selection, decoder choice, frame analysis,
  and `ERROR` sections;
- chapters, programs, stream groups, attachments, data streams, and attached
  pictures.

FFmpeg's source exposes frame side-data types for HDR10+ dynamic metadata,
mastering-display metadata, content-light level, Dolby Vision RPU data, and
decoded Dolby Vision metadata:
[FFmpeg side-data source](https://ffmpeg.org/doxygen/8.0/side__data_8c_source.html).
Its public Dolby Vision API describes dynamic metadata blocks that may vary by
frame:
[FFmpeg Dolby Vision metadata API](https://www.ffmpeg.org/doxygen/trunk/dovi__meta_8h.html).

FFmpeg is LGPL 2.1+ by default, while enabling optional GPL components changes
the applicable license; the official page provides a substantial redistribution
checklist:
[FFmpeg legal guidance](https://www.ffmpeg.org/legal.html).

### Conformance is a separate requirement

Neither candidate should be sold as a general conformance validator. MediaArea
provides that as the separate MediaConch product, whose implementation reports
currently validate a limited set of formats and whose policy reports test
user-defined rules:
[MediaConch documentation](https://mediaarea.net/MediaConch/Documentation/HowToUse).
ffprobe's decoder errors and frame inspection are useful diagnostics, but its
documentation does not define a standards-conformance verdict.

## Recommended issue #8 boundary

Perform one MediaInfo read and expose:

1. Raw FileBot-compatible objects: `media`, `video`, `audio`, `text`,
   `chapters`, `image`, and `menu`.
2. FileBot's documented technical convenience bindings derived from those same
   maps: codec, container, resolution, HDR / Dolby Vision labels, bitrate, frame
   rate, channels, languages, duration, and media title.

Missing metadata remains unavailable rather than guessed. Preserve MediaInfo
field names in raw objects; derived values and compatibility aliases belong only
at the top level.

Do **not** add ffprobe as a fallback. A fallback would make identical templates
produce backend-dependent keys and values. Add ffprobe later only as a separate,
explicit diagnostics feature if users ask for packet/frame inspection,
time-bounded analysis, payload hashes, or decode-error scanning.

## Unknowns to resolve during implementation

- Benchmark startup time, peak memory, and scan time on FileGot's actual Windows
  corpus; no fair primary-source comparison was found.
- Choose CLI subprocess versus MediaInfoLib after measuring packaging size and
  call volume. The CLI path uses Go's standard `os/exec` and `encoding/json`;
  native integration adds CGO and native-binary lifecycle work.
- Pin the distributed MediaInfo version and keep representative JSON fixtures
  for MKV, MP4, AVI, TS, audio-only, subtitles, cover art, chapters, HDR10,
  HDR10+, and Dolby Vision.
- Verify exact Dolby Vision samples and profiles needed by users. Both engines
  expose Dolby Vision information, but summary values and depth are not
  equivalent.
