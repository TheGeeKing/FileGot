# Use MediaInfo for technical metadata

FileGot uses the optional MediaInfo CLI as its sole backend for **technical bindings** and **raw media objects** (naming). MediaInfo is not used to read or write **embedded metadata**.

ffprobe is not a naming fallback: backend-dependent naming values would be unstable. Packet/frame diagnostics remain a possible future ffprobe use. Embedded-metadata tooling is decided separately in ADR 0003.
