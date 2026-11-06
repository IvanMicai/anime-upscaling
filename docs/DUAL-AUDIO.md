# Dual audio

A dual-audio merge joins the **video of one release** with the **audio of
another** into a single `.mkv` with both tracks, keeping the second one in sync.

The typical case: a clean web release in the original language, and an older TV
rip that carries the dub you actually want to hear.

## In the app: the Merge job

**Create Job → Merge.** Pick two folders (each holding one language), or any list
of files, and the app pairs them **by name**:

```
input/Show/EN/ep01.en.mkv    ┐
input/Show/PT/ep01.pt-br.mp4 ┘→  merged/Show/ep01.mkv
```

Same name, different language tag. The tag is the last segment before the
extension and comes from a closed list (`en`, `pt-br`, `ja`, `es-la`, …) —
release names are full of short segments that are not languages (`web`, `x264`,
`v2`), and guessing from shape would pair `show.web.mkv` with `show.bd.mkv`. The
file picker shows the pairing **before** the job runs: what becomes what, what is
already merged, and what is left without a partner and why.

| Setting | What it does |
| --- | --- |
| **Vídeo** | *Automático* keeps the picture with more pixels (bitrate breaks ties). It is a measure, not a judgement — an AI upscale has 4× the pixels of its clean source and may look worse — so the picture can also be chosen by language. |
| **Verificar a sincronia a cada** | The tick: how often the rebuilt track is checked against the video's own audio (1–30 s). Finer finds shorter out-of-sync stretches and takes longer. The window stays 10 s whatever the tick — a shorter one does not hold enough score to lock — so a finer tick means more overlapping checks, and a run of disagreeing checks only counts as evidence once it holds two that do not overlap. |
| **Buracos** | What plays where the other track has no content: the video's own audio (default) or silence. |
| **Faixa de áudio padrão** | Default: the language that did *not* supply the video — the one the merge was done for. |
| **Gravar mesmo sem sincronia garantida** | Off by default: a pair that fails the gate is not written and the log says why. |

### Choosing the picture by looking

*Automático* counts pixels, and pixels are not quality. Each pair in the preview
has a **Comparar** button that shows the same frame from both files in one box
with a draggable divider (double-click zooms), a few seek points plus a free
slider, and the resolution and bitrate of each. *Usar o vídeo de […]* sets the
choice for the job.

"The same frame" is the part that needs care: it is **not at the same
timestamp**. On a real pair, frame `t=100 s` of one file is a Pokéball in
mid-flight; `t=100 s` of the other is a different scene altogether, and the same
Pokéball is at `142.35 s` — and the offset had been `+30.7 s` a minute earlier,
because a cut sits in between. So the second frame is taken where
`GET /api/merge/locate` finds the same moment **by audio**: 30 s of audio around
the point, searched in ±4 min of the other file, about a second of work instead
of the ~40 s a full alignment takes. Near a cut part of a centred window has no
counterpart, so three placements are tried (centred, forward, backward) and the
most confident wins. When nothing matches — a dialogue-only stretch, or content
the other file lacks — the dialog says the two frames may not be the same scene
instead of presenting them as if they were.

Frames are served as **PNG**: they exist to judge picture quality, and JPEG would
add artefacts of its own to exactly what is being judged.

The result lands in **`merged/`**, a stage folder like the others: it is a valid
source for upscale, interpolate, optimize, check and saved pipelines.

### The verdict travels with the file

The sync verdict is written into the file's own container tags, so anything that
lists the file can say how good the sync is without measuring it again. The file
explorer shows it in the tooltip of the Merged column:

```
DUALAUDIO_SYNC         = {"v":1,"status":"ok","residual_ms":1.2,"p95_ms":9.5,
                          "in_place":591,"checked":612,"off_s":0,"coverage":0.954,
                          "segments":9,"gaps":6,"tick_s":2,"video":"en"}
DUALAUDIO_SYNC_SUMMARY = ok — residual 1 ms, 591/612 checks in place (97%), 0 s off, every 2 s
```

`status` is `ok`, or `review` for a file written with the gate overridden.

## From the command line

`animeup dualaudio` does the same alignment, plus pairing **by audio content**
for releases whose episode numbers do not line up (see below):

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
   segment, with a 40 ms crossfade centered on each join (20 ms on each side).
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
- **Pairing by audio in the app.** The Merge job pairs by name. Pairing by audio
  content (`dualaudio match`) — needed when two releases number their episodes
  differently — is CLI-only: it fingerprints every file first and its result
  wants reviewing, which the app has no screen for yet.
- **Merge as a saved-pipeline step.** A pipeline takes one file through one stage
  at a time; a merge consumes two files. `merged/` is a pipeline *source* instead.

## Resource use

Analysis is CPU-bound FFT work; nothing here touches the GPU. One episode takes
about 40 s and ~1 GB of RAM at the default settings, so `--jobs` multiplies both.
On a host shared with other services, cap the container (`--cpus`) — an unbounded
batch will starve its neighbours.

## Experimental visual diagnostics

When audio has little common music or effects, `dualaudio visual` can look for a
five-second image sequence near a timestamp in another release:

```bash
animeup dualaudio visual --base episode.en.mkv --dub episode.pt-br.mp4 --time 150 --radius 20
```

The JSON response gives `t_a`, `t_b`, normalized `distance`, `margin`, and
`matched`. It decodes one frame per second, downscales to grayscale and compares
cropped difference hashes. A match is rejected when the distance is too high or
a non-adjacent sequence scores similarly. These thresholds are experimental;
opening sequences, static scenes, watermarks, crops and differing edits can
still cause incorrect matches. An unmatched result has `t_b` set to zero.

This command is **read-only**. Its coarse timestamps are diagnostic evidence,
not proof of audio synchronization: it neither feeds the alignment algorithm nor
changes the quality gate. Check corresponding scenes and verify audio separately.

For a read-only map of distinctive scenes across an episode, run:

```bash
animeup dualaudio visual --base episode.en.mkv --dub episode.pt-br.mp4 --timeline
```

This searches every five seconds and refines distinctive three-second sequences
at 12 frames per second. The JSON contains accepted scene matches and two-second
comparison windows using offsets supported by at least three scene matches.
Short intervals between agreeing windows may also be included when every
intermediate sequence visually resembles the proposed match. These windows are
marked `contextual` because a static scene or short edit can remain ambiguous.
An independent grayscale-correlation check compares each midpoint against the
proposed frame and five-second-away controls. Rejected windows are recorded
separately and the audio map will not bridge those ranges. This check samples
one frame per window and does not prove frame-accurate alignment.

For a targeted, slower second look, `dualaudio visual-refine --base ... --dub
... --timeline timeline.json --limit 8 > refined.json` decodes three-second
sequences at 24 fps around rejected candidates. A window is recovered only
when its nearby timing peak is distinctive and beats five-second controls.
The limit is 1–100 per invocation; the resulting JSON remains compatible with
`visual-audio`. Recoveries are marked `fine_refined` and remain experimental:
24-fps sampling and matched imagery do not prove dialogue or source lip sync.

`dualaudio visual-fill --base ... --dub ... --timeline refined.json --limit 100
> filled.json` optionally searches missing two-second windows near existing
anchors using three-second grayscale sequences and distant controls. When the
wider sequence fails, it can try the two-second window itself with stricter
similarity and ambiguity thresholds; an edit just outside the window may
otherwise hide a match. It never
fills explicitly rejected intervals or uses newly recovered windows as anchors.
It also checks a cropped difference hash of the same sampled frames against
both distant controls; the two metrics are not independent source material.
The pass decodes both full videos at 12 fps and marks its additions
`gap_recovered`; they remain contextual, not an automatic merge approval.
Missing windows are inconclusive; they are not proof of a gap. This is not a
continuous edit map and does not identify exact cut boundaries. Sampling
limits timestamp resolution, and the audio may already be out of sync with its
own source video. Do not infer that the audio is synchronized from a visual
match. This mode decodes both videos twice and rejects inputs over two hours.

To investigate whether the audio supports a correction within each visually
matched region, save the timeline and run the separate read-only analysis:

```bash
animeup dualaudio visual --base episode.en.mkv --dub episode.pt-br.mp4 --timeline > timeline.json
animeup dualaudio visual-audio --base episode.en.mkv --dub episode.pt-br.mp4 --timeline timeline.json
```

The report groups nearby windows with similar visual offsets and estimates an
audio correction only when at least three non-overlapping audio windows agree.
The candidate applies that correction only within 30 seconds of an agreeing
audio window; elsewhere it retains the visual offset as an unverified hypothesis.
Only directly observed visual windows are mapped; unobserved intervals remain
gaps even when the offsets of their neighbors agree.
Use the same base and dub used to create the timeline: the JSON does not bind
itself to the source files.
It reports gaps, the candidate map, audio evidence and validation statistics.
The separate `source_validation` compares the in-memory reconstruction to the
original dubbed audio at the candidate map's positions. This checks whether
audio was placed as the map specifies, not whether the map or the dub's own
lip sync is correct; it also does not verify a muxed or encoded output file.
The report gives mapped and examined seconds separately because short segments,
gaps and joins are excluded from the source check.

Add `--stereo` to `visual-audio` to experimentally extract and compare left and
right audio features separately instead of downmixing first. This can retain
shared sounds that partly cancel in a mono mix, but different channel layouts,
swapped channels or unrelated mixes can still give misleading results. The
source-audio reconstruction check also compares channels separately in this
mode. It remains `review` and does not change the normal merge gate.

With `--stereo --holdout`, corrections are estimated on non-overlapping
10-second windows; different windows are reserved for evaluation, with
padding between reference searches and exclusion of overlapping dubbed source
samples. Feature normalization still uses whole-track statistics, and this is
not an independently annotated sync verdict. In this mode `off_sec` counts
only confidently off sampled seconds and does not imply a continuous run.
`--stereo --repair-audio` instead
tests small corrections on consistently off runs and retains them only if the
same validation set improves without reducing coverage or worsening aggregates.
That repair comparison is not a holdout and cannot be combined with `--holdout`.
The report separately flags mapped intervals where dubbed-audio source time
differs by more than 150 ms from the corresponding visual source time. An
experimental repair is not kept if it increases those flagged intervals. This
flag does not determine which source is correct: the dub may itself be out of
sync, and visual estimates are sampled. Gaps and unobserved images are excluded.

The native AAC/Matroska mux path retains the complete rendered PCM. An
integration test measures the decoded track against its source to detect an
encoder-priming timing shift; this behavior should be rechecked if the encoder
or container changes.
PCM decoding uses stream timestamps to preserve initial audio offsets relative
to the container timeline. A regression test covers an audio stream beginning
one second after video.
Each report is `review`: a soundtrack match can disagree with the dub's own
picture, and the report does not establish whether the original dub is itself
in sync. No media is written and this analysis does not change the merge gate.

### Experimental visual assistance in merge jobs

The opt-in **Experimental visual assistance** setting proposes offsets from
five-second image sequences sampled once per second. A candidate is only added
after the original audio matcher independently refines it near that location.
The solver compares the resulting alignment against the audio-only result and
keeps it only if the existing quality checks improve; the audio quality gate is
never relaxed. Enabling this decodes both videos and may substantially increase
processing time. Videos longer than two hours are rejected by this mode.

Visual matches can be wrong in repeated scenes, cuts, mirrored or heavily edited
releases; the option does not guarantee that a rejected merge will succeed.
API clients can opt in with `merge_visual_assist: true` in `POST /api/jobs`;
`dualaudio analyze --visual-assist` exercises it without producing an output.
