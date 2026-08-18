#!/usr/bin/env bash
# mlow_file_test.sh — file-in/file-out MLow codec test (Go implementation).
# ffmpeg decodes any audio file (mp3/wav/m4a/...) to 16 kHz mono PCM, which is encoded
# to an MLow .bin and decoded back to a WAV. The .bin format is the shared MLow
# container, so a .bin produced here also decodes with the Rust build's identical
# script (run that repo's scripts/mlow_file_test.sh on the .bin) — the two codecs
# interoperate by file, without referencing each other.
#
#   scripts/mlow_file_test.sh enc <audio-in> <out.bin>
#   scripts/mlow_file_test.sh dec <in.bin>   <out.wav>
#   scripts/mlow_file_test.sh roundtrip <audio-in> <out.wav>
set -euo pipefail

cd "$(dirname "$0")/.."
RATE=16000

command -v ffmpeg >/dev/null 2>&1 || { echo "need ffmpeg (decodes the input audio to PCM)" >&2; exit 1; }

# mlow <meowtool args...> — the local (Go) MLow encode/decode tool. This is the only
# implementation-specific line; the Rust repo's copy of this script swaps just this.
mlow() { go run ./examples/mlow "$@"; }

# audio file -> raw s16le mono 16k on stdout
to_pcm() { ffmpeg -hide_banner -loglevel error -i "$1" -ar "$RATE" -ac 1 -f s16le -; }

cmd_enc() {
  local in="${1:?usage: enc <audio-in> <out.bin>}" out="${2:?missing out.bin}"
  to_pcm "$in" | mlow encode -o "$out"
  echo "encoded $in -> $out" >&2
}

cmd_dec() {
  local in="${1:?usage: dec <in.bin> <out.wav>}" out="${2:?missing out.wav}"
  mlow decode -i "$in" -o "$out"
  echo "decoded $in -> $out" >&2
}

cmd_roundtrip() {
  local in="${1:?usage: roundtrip <audio-in> <out.wav>}" out="${2:?missing out.wav}"
  local bin; bin="$(mktemp -t mlow).bin"
  to_pcm "$in" | mlow encode -o "$bin"
  mlow decode -i "$bin" -o "$out"
  rm -f "$bin"
  echo "roundtrip: $in -> mlow -> $out" >&2
}

case "${1:-}" in
  enc)       shift; cmd_enc "$@" ;;
  dec)       shift; cmd_dec "$@" ;;
  roundtrip) shift; cmd_roundtrip "$@" ;;
  *) sed -n '2,13p' "$0"; exit 2 ;;
esac
