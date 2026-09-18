# Dual audio

`animeup dualaudio` joins the **video of one release** with the **dubbed audio of
another** into a single `.mkv` with both tracks, keeping the dub in sync.

The typical case: a clean web release in the original language, and an older TV
rip that carries the dub you actually want to hear.

```bash
animeup dualaudio match --base-dir /media/en --dub-dir /media/pt --out mapping.json
animeup dualaudio build --mapping mapping.json --out-dir /media/dual --jobs 2
animeup dualaudio analyze --base ep01.mkv --dub "EP 001.mp4"   # measure, write nothing
```

`base` is the release that supplies the picture (and keeps its own audio).
`dub` supplies the second audio track.

## Why the obvious approach fails

Everything below was measured on 82 episode pairs of one season. Each point
broke a simpler design.

**1. There is no single offset per episode.** The duration difference between
the two releases ran from −209 s to +179 s and *changed sign* between episodes.
Both were 23.976 fps, so this is not PAL speed-up: the opening, eyecatch and
next-episode preview sit in different places. A global `-itsoffset` syncs the
first minute and drifts from there.

**2. The offset changes inside an episode.** One episode, measured:

```
   0 ->   58 s   lag  -0.23 s       667 ->  952 s   lag -10.53 s   <- 11.6 s cut
  58 ->  190 s   lag  +0.05 s       952 -> 1167 s   lag -10.04 s
 190 ->  560 s   lag  +0.58 s      1167 -> 1283 s   lag -10.15 s
 560 ->  667 s   lag  +1.07 s      1283 -> 1342 s   (base only)
```

Steps of 0.03–0.5 s plus one large cut: the signature of a TV rip reassembled at
commercial breaks. Alignment therefore works **per segment**.

**3. Episode numbers cannot be trusted.** The two releases ordered the season
differently (broadcast order vs production order). 28 of 82 numbers disagreed,
by anything from −3 to +6 — and two by **−26**. Pairing by number would have
produced dozens of files carrying *another episode's* dialogue. `match` decides
the pair **from the audio**, comparing every base against every dub: a first
version that only looked ±12 episodes around the number missed those two pairs
and reported them as having no dub at all.

## How it works

Matching two languages works because a dub is normally recorded over the same
music-and-effects master: only the voices differ. The score is what lines up.

1. **Features.** Mono 16 kHz → 24-band log-mel at 100 fps → **positive spectral
   flux** (only what *rises* between frames). Flux sharpens note onsets and sound
   effects, which both tracks share, and mutes the sustained timbre of speech,
   which they do not. Against plain log-mel it lifted window confidence from ~8
   to 20–140 and took a test episode from 45/127 locked windows to 127/127.
2. **Sweep.** A 30 s window every 10 s, cross-correlated (FFT) against the base.
   Each yields a lag and a confidence (peak against the MAD of the curve). A
   window that does not lock near the global prior is retried unbounded — a large
   cut throws it far from where the bounded search looks.
3. **Outliers and the monotonic chain.** A window whose lag disagrees with both
   neighbours is dropped. That is not enough: recurring music (the opening theme,
   a sting) matches elsewhere in the episode with high confidence, and because
   neighbouring windows overlap by 20 s the mistake arrives as 2–4 windows that
   agree with each other. 32 of 74 validation episodes had such a group (lags of
   +506 s, −668 s, +1320 s). A real alignment moves forward on both timelines, so
   only the heaviest **monotonic chain** of window groups is kept.
4. **Solve.** The groups only *propose* lags. Spectral flux is sharp enough that
   a 30 ms lag error drops the score from 0.13 to ~0, so a segment built from a
   tolerant group (80 ms, median) can end up with a lag that fits nothing, and
   its boundary lands anywhere. Instead every candidate lag (10 ms resolution) is
   scored every 0.5 s of video, and a **Viterbi path** picks the sequence that
   best explains the whole episode, paying a toll per switch. A stretch with no
   evidence (dialogue only) pays for no switch and inherits the lag in force.
   Short excursions (A → X → A) are removed; neighbours within 20 ms are fused.
5. **Short segments and holes.** A segment under ~40 s never yields two agreeing
   coarse windows, so its lag is never proposed. A fine sweep (8 s windows)
   around every join finds those, accepting only lags two windows agree on. Where
   the lag *rises*, the base holds content the dub lacks: the dub is continuous,
   so that stretch is left as a **hole** (placed by a change-point search) and
   plays the base audio — it used to be filled with repeated dub audio.
6. **Render.** The dub's PCM is repositioned on the base timeline segment by
   segment, with a 20 ms crossfade at each join.
7. **Gaps.** A stretch that exists only in the base (credits, preview, a scene
   the dub's rip cut) plays the **base audio**. `--gap-fill silence` mutes it
   instead, but a minute of silence reads as a playback fault, while the original
   language says at once that this part was never dubbed.
8. **Validation, in a closed loop.** The *rebuilt* track is correlated against
   the base again in 10 s windows. Three things are measured: the **residual** of
   confident windows; the **hit share** — windows whose peak lands within 60 ms of
   zero, with no confidence floor, because a stretch carrying the wrong audio
   locks nowhere and would otherwise never enter the median; and **seconds of
   consistent error** — three neighbouring confident windows that disagree with
   zero and agree with each other. Validation also *measures what is missing*: a
   confident residual of +0.14 s says the right lag there is the segment's lag
   + 0.14, so it is fed back as a candidate and the solver runs again, while it
   improves. Windows inside a gap never count — base audio matches itself.
9. **Mux.** Base video and audio are stream-copied; the dub is encoded to AAC;
   subtitles and chapters are kept; tracks are tagged with language and title.

## The quality gate

Every episode is graded `ok`, `review` or `fail`, and **only `ok` is written**
(unless `--force`). An episode is `review` when the median residual exceeds
50 ms, the 95th percentile exceeds 150 ms, more than 8 s play at a consistently
wrong offset, fewer than 75 % of validation windows land in place, coverage falls
below 90 %, fewer than half the sweep windows locked, or rate drift is suspected.
A held-back episode lists the timestamps that failed, so there is somewhere to
look.

This is the point of the design, and it was earned the hard way. An earlier gate
looked only at the residual of windows that locked, and passed an episode whose
second half played the dub from the wrong place — nothing locks over wrong audio,
so it never reached the median. What caught it, and then 14 more episodes with
19–72 s at a neighbour's offset, was checking the **finished files**: correlate
the two audio tracks of the output against each other and expect zero everywhere.
Keep doing that after any change to the aligner:

```bash
animeup dualaudio analyze --base out.dual.mkv --dub out.dual.mkv --dub-stream 1
```

On the validation season (82 episodes): all 82 paired by audio, 69 passed the
gate, and **66 of those also passed the finished-file check**. The 16 held back
are episodes where the two releases share too little score to verify (long
dialogue-only stretches), one pair covering a fraction of the base timeline, and
three that the gate passed but the finished-file check did not — which is the
argument for running it.

`match` applies the same idea to pairing. A pair is accepted when the best
candidate beats the runner-up by `--min-margin` (default 1.6×; genuine pairs
typically clear 3×), **or** when it is a *mutual* best match — the dub's own best
base is this base — with at least 1.15×. Measured on the full 82×82 matrix:
margin alone accepted 75 pairs, all correct, and refused 7 whose top candidate
was right but whose runner-up (an episode scored from the same music library)
came close; adding mutuality accepted 82 of 82, still with none wrong. Mutuality
is what keeps an episode with no counterpart refused: its least-bad dub already
belongs, more strongly, to another base. `mapping.json` is plain JSON so a
refused pair can be filled in by hand.

## Traps worth knowing

- **AAC encoder delay.** ffmpeg's native AAC encoder delays its output by 1024
  samples (21.3 ms) and records it as `initial_padding`, which the Matroska muxer
  does **not** compensate for. The dub came out a constant 20 ms late. Dropping
  1024 samples before encoding cancels it exactly. This only holds for the native
  encoder — change it and measure again:
  `animeup dualaudio analyze --base out.mkv --dub out.mkv --dub-stream 1`
  should report a lag of 0.00 s.
- **Shared structure in test audio.** A synthetic "voice" with a fixed 0.7 Hz
  envelope gave both test tracks the same 1.43 s periodicity, and the aligner
  locked onto it. Real voices share no envelope; test fixtures must not either.
- **Temp file extension.** Output is written to `<name>.part` and renamed, so a
  killed job never leaves a half file where a finished one is expected — but
  ffmpeg picks its muxer from the extension, so `-f matroska` is mandatory.

## Not implemented

- **Rate drift.** Sources running at different speeds (a 24 fps master conformed
  to 23.976) need resampling, not constant offsets. It is *detected* and graded
  `review`; no such pair was available to validate a resampling path against.
- **Dub-only content.** Audio that exists only in the dub has no picture to go
  with and is dropped. If a dub carries much of it, swap the roles of `base` and
  `dub`.
- **Pipeline operation / UI.** This is a CLI command. Saved pipelines process one
  file through one stage folder at a time, while dual audio takes two
  directories and a season-wide pairing step that benefits from review. Wiring it
  in needs decisions this change does not make: where the dub source lives, a
  stage folder for the output, how resume treats it, and a UI to inspect and
  correct `mapping.json`.

## Resource use

Analysis is CPU-bound FFT work; nothing here touches the GPU. One episode takes
about 40 s and ~1 GB of RAM at the default settings, so `--jobs` multiplies both.
On a host shared with other services, cap the container (`--cpus`) — an unbounded
batch will starve its neighbours.
