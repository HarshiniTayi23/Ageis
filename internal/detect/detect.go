// Package detect scores each new snapshot for signs of a ransomware attack by
// diffing it against the previous snapshot.
//
// The insight: a backup system sees every file, every day. Ransomware leaves
// a very distinctive statistical fingerprint in that stream:
//
//  1. Entropy jump  - encrypted bytes look random (~8 bits/byte). A .txt or
//     .csv that was ~4.5 bits/byte yesterday and ~7.99 today is a red flag.
//  2. Change burst  - normal days touch a few % of files; an encryptor
//     touches most of them in minutes.
//  3. Rename storms - files deleted and re-created with a new extension
//     (report.xlsx -> report.xlsx.locked).
//  4. Ransom notes  - the same "HOW_TO_DECRYPT" file dropped in many dirs.
//
// No single signal is conclusive (a zip archive is high-entropy, a bulk
// re-format touches every file), so signals are combined into a score.
package detect

import (
	"fmt"
	"math"
	"path"
	"strings"
)

// FileStat is the per-file information the detector needs.
type FileStat struct {
	Path    string  `json:"path"`
	SHA256  string  `json:"sha256"`
	Size    int64   `json:"size"`
	Entropy float64 `json:"entropy"` // Shannon entropy, bits per byte (0..8)
}

// Verdict is the result of analysing one snapshot.
type Verdict struct {
	Score            float64  `json:"score"` // 0..1
	Suspicious       bool     `json:"suspicious"`
	Reasons          []string `json:"reasons,omitempty"`
	ChangedRatio     float64  `json:"changed_ratio"`
	EntropyJumpRatio float64  `json:"entropy_jump_ratio"`
	RenamedToBadExt  int      `json:"renamed_to_bad_ext"`
	RansomNotes      int      `json:"ransom_notes"`
}

// Thresholds are tunable; defaults are conservative.
type Thresholds struct {
	HighEntropy     float64 // bits/byte considered "looks encrypted"
	EntropyJump     float64 // minimum increase to count as a jump
	SuspiciousScore float64
}

var Default = Thresholds{HighEntropy: 7.5, EntropyJump: 1.5, SuspiciousScore: 0.5}

var badExtensions = map[string]bool{
	".locked": true, ".encrypted": true, ".enc": true, ".crypt": true,
	".crypted": true, ".lock": true, ".ransom": true, ".pay": true,
	".locky": true, ".cerber": true, ".wncry": true, ".ryk": true,
}

var noteMarkers = []string{"decrypt", "ransom", "recover_files", "how_to_restore", "readme_for_decrypt", "restore_files"}

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

// Analyze compares the current snapshot with the previous one.
// prev may be nil for the very first snapshot.
func Analyze(prev, cur []FileStat, t Thresholds) Verdict {
	var v Verdict
	prevByPath := make(map[string]FileStat, len(prev))
	for _, f := range prev {
		prevByPath[f.Path] = f
	}
	curByPath := make(map[string]bool, len(cur))

	var changed, jumped, compared int
	for _, f := range cur {
		curByPath[f.Path] = true
		base := strings.ToLower(path.Base(f.Path))
		for _, m := range noteMarkers {
			if strings.Contains(base, m) {
				v.RansomNotes++
				break
			}
		}
		old, existed := prevByPath[f.Path]
		if !existed {
			// New file: is it "old path + bad extension"? (rename storm)
			ext := strings.ToLower(path.Ext(f.Path))
			if badExtensions[ext] {
				orig, wasThere := prevByPath[strings.TrimSuffix(f.Path, path.Ext(f.Path))]
				if wasThere || prev == nil {
					v.RenamedToBadExt++
				}
				// Compare the renamed file with the file it replaced, so
				// "encrypt then rename" families still register an
				// entropy jump.
				if wasThere {
					compared++
					if f.Entropy >= t.HighEntropy && f.Entropy-orig.Entropy >= t.EntropyJump {
						jumped++
					}
				}
			}
			continue
		}
		compared++
		if old.SHA256 != f.SHA256 {
			changed++
			if f.Entropy >= t.HighEntropy && f.Entropy-old.Entropy >= t.EntropyJump {
				jumped++
			}
		}
	}
	deleted := 0
	for p := range prevByPath {
		if !curByPath[p] {
			deleted++
		}
	}

	if len(prev) > 0 {
		v.ChangedRatio = float64(changed+deleted) / float64(len(prev))
	}
	if compared > 0 {
		v.EntropyJumpRatio = float64(jumped) / float64(compared)
	}
	renameRatio := 0.0
	if len(prev) > 0 {
		renameRatio = float64(v.RenamedToBadExt) / float64(len(prev))
	}

	// Weighted score. Entropy jumps are the strongest single signal because
	// legitimate edits to text/CSV/source files almost never make them
	// look random.
	score := 0.45*clamp(v.EntropyJumpRatio/0.3) +
		0.20*clamp(v.ChangedRatio/0.6) +
		0.25*clamp(renameRatio/0.3) +
		0.10*clamp(float64(v.RansomNotes)/2)
	v.Score = math.Round(score*1000) / 1000

	if v.EntropyJumpRatio > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%.0f%% of existing files jumped to near-random entropy", 100*v.EntropyJumpRatio))
	}
	if v.ChangedRatio >= 0.3 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%.0f%% of files changed or deleted since last snapshot", 100*v.ChangedRatio))
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
