// Package detect scores each new snapshot for signs of a ransomware attack by
// diffing it against the last clean snapshot.
//
// A backup system sees every file, every day. Ransomware leaves a
// distinctive fingerprint in that stream. v2 signals (see docs/eval for how
// each one was justified by a measured failure of v1):
//
//  1. Hostile change - a file's content changed AND now looks encrypted:
//     a. entropy jumped to near-random (~8 bits/byte), or
//     b. a text file's entropy jumped sharply (catches base64-wrapped
//     ciphertext, which sits at ~6 bits/byte), or
//     c. its format broke: it was a recognisable JPEG/PDF/ZIP/text and is
//     now unrecognisable bytes. Real re-encodes keep their format; even
//     a 1-byte XOR "cipher", which leaves entropy unchanged, breaks it.
//     Renamed and moved files are paired with their originals first, so
//     "encrypt then rename" can't hide the change.
//  2. Mass deletion - most files gone (wipers).
//  3. Rename storm  - ransomware-style extensions appended.
//  4. Ransom notes  - "HOW_TO_DECRYPT"-style files.
//
// No single weak signal is conclusive (a zip is high-entropy, a bulk edit
// touches every file), so signals are combined into a 0..1 score.
package detect

import (
	"fmt"
	"math"
	"net/http"
	"path"
	"strings"
)

// FileStat is the per-file information the detector needs.
type FileStat struct {
	Path    string  `json:"path"`
	SHA256  string  `json:"sha256"`
	Size    int64   `json:"size"`
	Entropy float64 `json:"entropy"`        // Shannon entropy, bits per byte (0..8)
	Type    string  `json:"type,omitempty"` // content type sniffed from the first bytes
}

// Verdict is the result of analysing one snapshot.
type Verdict struct {
	Score              float64  `json:"score"` // 0..1
	Suspicious         bool     `json:"suspicious"`
	Reasons            []string `json:"reasons,omitempty"`
	ChangedRatio       float64  `json:"changed_ratio"`
	EntropyJumpRatio   float64  `json:"entropy_jump_ratio"` // v1 name kept: share of files with a hostile change
	FormatBroken       int      `json:"format_broken"`      // files whose recognisable format disappeared
	DeletedRatio       float64  `json:"deleted_ratio"`      // files gone with no rename/move partner
	RenamedToBadExt    int      `json:"renamed_to_bad_ext"`
	RansomNotes        int      `json:"ransom_notes"`
	PairedRenamesMoves int      `json:"paired_renames_moves"` // renamed/moved files matched to an original
}

// Thresholds are tunable; defaults are conservative.
type Thresholds struct {
	HighEntropy     float64 // bits/byte considered "looks encrypted"
	EntropyJump     float64 // minimum increase to count as a jump
	TextJump        float64 // minimum increase for text files (base64-wrapped ciphertext)
	SuspiciousScore float64
}

var Default = Thresholds{HighEntropy: 7.5, EntropyJump: 1.5, TextJump: 1.0, SuspiciousScore: 0.5}

var badExtensions = map[string]bool{
	".locked": true, ".encrypted": true, ".enc": true, ".crypt": true,
	".crypted": true, ".lock": true, ".ransom": true, ".pay": true,
	".locky": true, ".cerber": true, ".wncry": true, ".ryk": true,
}

var noteMarkers = []string{"decrypt", "ransom", "recover_files", "how_to_restore", "readme_for_decrypt", "restore_files"}

// Sniff returns the content type of a file from its first bytes (up to 512).
func Sniff(head []byte) string {
	if len(head) == 0 {
		return ""
	}
	t := http.DetectContentType(head)
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = t[:i]
	}
	return t
}

// Entropy returns the Shannon entropy of a byte histogram, in bits per byte.
func Entropy(hist *[256]uint64, total uint64) float64 {
	if total == 0 {
		return 0
	}
	var h float64
	for _, c := range hist {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log2(p)
	}
	return h
}

const unknownType = "application/octet-stream"

func isText(t string) bool { return strings.HasPrefix(t, "text/") }

// hostile reports whether old -> cur looks like encryption rather than editing.
func hostile(old, cur FileStat, t Thresholds) (yes, formatBroken, binaryBroken bool) {
	if old.SHA256 == cur.SHA256 {
		return false, false, false
	}
	if cur.Entropy >= t.HighEntropy && cur.Entropy-old.Entropy >= t.EntropyJump {
		yes = true
	}
	if isText(old.Type) && isText(cur.Type) && cur.Entropy-old.Entropy >= t.TextJump {
		yes = true // e.g. base64-wrapped ciphertext replacing CSV
	}
	if old.Type != "" && old.Type != unknownType && cur.Type != old.Type &&
		(cur.Type == unknownType || !isText(old.Type)) {
		yes, formatBroken = true, true
		// Legitimate tools sometimes turn text into opaque bytes (encrypted
		// exports); they essentially never mass-corrupt binary formats.
		binaryBroken = !isText(old.Type)
	}
	return yes, formatBroken, binaryBroken
}

// Analyze compares the current snapshot with the baseline (last clean one).
// prev may be nil for the very first snapshot.
func Analyze(prev, cur []FileStat, t Thresholds) Verdict {
	var v Verdict
	prevByPath := make(map[string]FileStat, len(prev))
	for _, f := range prev {
		prevByPath[f.Path] = f
	}
	curByPath := make(map[string]bool, len(cur))
	for _, f := range cur {
		curByPath[f.Path] = true
	}

	// Files that vanished from their old path are candidates for pairing
	// with files that appeared under a new path (renamed or moved).
	gone := map[string]FileStat{}
	goneByBase := map[string][]string{}
	for p, f := range prevByPath {
		if !curByPath[p] {
			gone[p] = f
			goneByBase[path.Base(p)] = append(goneByBase[path.Base(p)], p)
		}
	}
	partner := func(newPath string) (FileStat, bool) {
		// 1. extension appended: report.csv -> report.csv.locked
		if ext := path.Ext(newPath); ext != "" {
			if f, ok := gone[strings.TrimSuffix(newPath, ext)]; ok {
				return f, true
			}
		}
		// 2. moved: same file name, different folder (only if unambiguous)
		if c := goneByBase[path.Base(newPath)]; len(c) == 1 {
			if f, ok := gone[c[0]]; ok {
				return f, true
			}
		}
		return FileStat{}, false
	}

	var changed, hostileN, binaryBroken int
	paired := map[string]bool{}
	for _, f := range cur {
		base := strings.ToLower(path.Base(f.Path))
		for _, m := range noteMarkers {
			if strings.Contains(base, m) {
				v.RansomNotes++
				break
			}
		}
		old, existed := prevByPath[f.Path]
		if !existed {
			var ok bool
			if old, ok = partner(f.Path); !ok {
				continue // genuinely new file
			}
			paired[old.Path] = true
			v.PairedRenamesMoves++
			if badExtensions[strings.ToLower(path.Ext(f.Path))] {
				v.RenamedToBadExt++
			}
		}
		if old.SHA256 != f.SHA256 {
			changed++
		}
		if h, broken, bin := hostile(old, f, t); h {
			hostileN++
			if broken {
				v.FormatBroken++
			}
			if bin {
				binaryBroken++
			}
		}
	}
	deleted := 0
	for p := range gone {
		if !paired[p] {
			deleted++
		}
	}

	n := float64(len(prev))
	if n > 0 {
		v.ChangedRatio = float64(changed+deleted) / n
		v.EntropyJumpRatio = float64(hostileN) / n
		v.DeletedRatio = float64(deleted) / n
	}
	renameRatio := 0.0
	if n > 0 {
		renameRatio = float64(v.RenamedToBadExt) / n
	}

	weighted := 0.45*clamp(v.EntropyJumpRatio/0.3) +
		0.20*clamp(v.ChangedRatio/0.6) +
		0.25*clamp(renameRatio/0.3) +
		0.10*clamp(float64(v.RansomNotes)/2)
	// Strong single signals can carry the score on their own.
	strong := 0.0
	if v.DeletedRatio >= 0.8 {
		strong = 0.7 // wiper
	}
	if binaryBroken >= 3 && float64(binaryBroken)/math.Max(1, float64(hostileN)) >= 0.5 && v.EntropyJumpRatio >= 0.2 {
		strong = math.Max(strong, 0.6) // many images/archives/documents turned into noise
	}
	v.Score = math.Round(math.Max(weighted, strong)*1000) / 1000

	if v.EntropyJumpRatio > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%.0f%% of existing files changed into encrypted-looking content", 100*v.EntropyJumpRatio))
	}
	if v.FormatBroken > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%d files lost their file format (e.g. a JPEG or CSV that is now unreadable bytes)", v.FormatBroken))
	}
	if v.ChangedRatio >= 0.3 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%.0f%% of files changed or deleted since last snapshot", 100*v.ChangedRatio))
	}
	if v.DeletedRatio >= 0.5 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%.0f%% of files deleted", 100*v.DeletedRatio))
	}
	if v.RenamedToBadExt > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%d files re-appeared with ransomware-style extensions", v.RenamedToBadExt))
	}
	if v.RansomNotes > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%d probable ransom-note files", v.RansomNotes))
	}
	v.Suspicious = v.Score >= t.SuspiciousScore
	return v
}

func clamp(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < 0 {
		return 0
	}
	return x
}
