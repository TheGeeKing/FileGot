# Use MediaInfo for technical metadata

FileGot uses the optional MediaInfo CLI as its sole technical-metadata backend because MediaInfo provides naming-oriented normalized fields and the same raw stream model exposed by FileBot. ffprobe is reserved for a future diagnostics feature if FileGot ever needs packet or frame inspection, payload hashes, interval reads, or decoder analysis; it is not a fallback because backend-dependent values would make naming results unstable.
