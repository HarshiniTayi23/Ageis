# Detector evaluation

How good is the ransomware detector, really? These files answer that with numbers, not with the scenarios that happen to pass.

| File | What it is |
|---|---|
| [detector-v1-baseline.md](detector-v1-baseline.md) | The original rule-based detector, measured before any changes |
| [detector-report.md](detector-report.md) | The current detector. Regenerate with `make eval` |

## Method
- `cmd/aegis-eval` builds a realistic file share: about 75% CSV/text, about 25% JPEGs with real file signatures. It snapshots the share, applies a scenario, and scores the result exactly as a backup would, using real files and real byte entropy.
- **16 tuning scenarios × 20 seeds, plus 6 held-out scenarios × 20 seeds = 440 labelled snapshots.**
- Scenarios include deliberately **hard** cases on both sides:
  - Legitimate events that look like attacks: zipping records, password-protected exports.
  - Attacks designed to evade rules: base64-wrapped ciphertext, XOR, slow encryption, encrypting only already-compressed files.
- **Held-out scenarios were written before any tuning and were never used to tune.** The gap between tuning and held-out scores estimates how much the detector was fitted to cases its author had already seen.

## Results

| | Tuning recall | Tuning false alarms | **Held-out recall** | Held-out false alarms |
|---|---|---|---|---|
| v1 (original) | 38% | 0% | 33% | 0% |
| v2 first attempt | 75% | **9%** ❌ | 68% | 0% |
| **v2 (current)** | **87%** | **0%** | **68%** | **0%** |

### What changed from v1 to v2, and which failure justified each change

| v1 failure (from the baseline report) | v2 change |
|---|---|
| Wiper (90% deleted): 0/20 caught | Mass deletion is a strong signal on its own |
| Encrypted + renamed to an unknown extension: 0/20 | Pair new files with deleted originals (appended extension or same name in a new folder) before comparing |
| Only JPEGs encrypted (already high entropy): 0/20 | Sniff each file's format; a recognisable JPEG/PDF/ZIP turning into unknown bytes is hostile |
| Base64-wrapped ciphertext (about 6 bits/byte): 0/20 | Separate, lower entropy-jump threshold for text files |

### The first v2 attempt broke the counter-metric
Treating *any* broken format as a strong signal flagged a **legitimate password-protected export** in 15 of 20 runs: a 9% false-alarm rate against the PRD's 2% limit. The fix came from domain reasoning, not curve-fitting. Legitimate tools do turn text into opaque bytes, as encrypted exports do. They essentially never mass-corrupt binary formats. So only broken *binary* formats count as a strong signal alone.

## What it still misses, and why that's the case for M3
- **Slow encryption** (10% per backup): each snapshot looks almost normal. Catching it needs *history across many snapshots*, not one comparison. That's the M3 statistical detector.
- **XOR "encryption"** (held-out): XOR with a 1-byte key keeps text looking like text, and entropy doesn't move. Only the JPEGs' broken signatures show, and 25% of files isn't enough. Deliberately **not** fixed, because it's held-out.

## Caveats, stated plainly
- The scenarios are synthetic and were written by the same person who wrote the detector. Real-world recall is unknown until it's measured on real incident data.
- 20 seeds per scenario gives rough percentages, not precise ones.
- CI runs `aegis-eval -gate` on every push. It fails if tuning recall drops below 85% or any false-alarm rate rises above 2%. Held-out recall is reported but **not gated**, because gating it would invite tuning against it.
