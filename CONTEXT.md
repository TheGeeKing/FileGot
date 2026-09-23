# FileGot

FileGot matches local media files to authoritative metadata and proposes safe names before changing the filesystem.

## Language

**Expected episode**:
An episode imported from a metadata provider with a proposed name but not yet associated with a local video file. It exists only in the current FileGot session.
_Avoid_: Placeholder file, imported file

**Episode pairing**:
The association of a local video file with one expected episode, made automatically from a unique season-and-episode identifier or selected manually.
_Avoid_: Metadata match

**Exact-title episode pairing**:
An episode pairing fallback that uses a complete expected episode title found in a normalized local filename. It applies only when the title identifies exactly one unpaired expected episode and does not use approximate similarity.
_Avoid_: Fuzzy match

**Leading-number episode pairing**:
An episode pairing fallback that treats the first numeric filename segment as an episode number. It applies only when that number identifies exactly one unpaired expected episode and exactly one local file.
_Avoid_: Release code parsing

**Technical binding**:
A naming value derived from the structure or encoded streams of a local media file rather than from an external title provider.
_Avoid_: Probe token

**Raw media object**:
A FileBot-compatible map of MediaInfo fields for a general, video, audio, text, image, or menu stream.
_Avoid_: Probe output

**Embedded metadata**:
Descriptive title-provider fields written into a media container when the user opts in. Distinct from technical bindings and from raw media objects.
_Avoid_: Tags, MediaInfo fields, technical metadata, technical binding
