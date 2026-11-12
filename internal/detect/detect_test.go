package detect

import (
	"fmt"
	"testing"
)

func corpus(n int) []FileStat {
	var out []FileStat
	for i := 0; i < n; i++ {
		f := FileStat{Path: fmt.Sprintf("d/f%02d.csv", i), SHA256: fmt.Sprintf("a%d", i), Size: 1000, Entropy: 4.5, Type: "text/plain"}
		if i%4 == 0 {
			f.Path, f.Entropy, f.Type = fmt.Sprintf("d/p%02d.jpg", i), 7.9, "image/jpeg"
		}
		out = append(out, f)
	}
	return out
}

func TestWiperFlaggedWithoutEncryption(t *testing.T) {
	prev := corpus(40)
	v := Analyze(prev, prev[:3], Default)
	if !v.Suspicious || v.DeletedRatio < 0.9 {
		t.Fatalf("wiper not flagged: %+v", v)
	}
}

func TestEncryptThenRenameIsPairedWithOriginal(t *testing.T) {
	prev := corpus(40)
	var cur []FileStat
	for i, f := range prev {
		if i < 28 { // 70% encrypted and renamed to an unknown extension
			cur = append(cur, FileStat{Path: f.Path + ".x7k2q", SHA256: "enc" + f.SHA256, Size: f.Size, Entropy: 7.99, Type: "application/octet-stream"})
		} else {
			cur = append(cur, f)
		}
	}
	v := Analyze(prev, cur, Default)
	if v.PairedRenamesMoves != 28 || !v.Suspicious {
		t.Fatalf("rename pairing failed: %+v", v)
	}
}

func TestBrokenBinaryFormatsCaughtWithoutEntropyJump(t *testing.T) {
	prev := corpus(40)
	var cur []FileStat
	for _, f := range prev {
		if f.Type == "image/jpeg" { // already high entropy; only the format breaks
			f = FileStat{Path: f.Path, SHA256: "enc" + f.SHA256, Size: f.Size, Entropy: 7.99, Type: "application/octet-stream"}
		}
		cur = append(cur, f)
	}
	v := Analyze(prev, cur, Default)
	if !v.Suspicious || v.FormatBroken != 10 {
		t.Fatalf("media-only encryption missed: %+v", v)
	}
}

func TestMovedUnchangedFilesAreNotSuspicious(t *testing.T) {
	prev := corpus(40)
	var cur []FileStat
	for _, f := range prev {
		f.Path = "archive/" + f.Path[2:]
		cur = append(cur, f)
	}
	v := Analyze(prev, cur, Default)
	if v.Suspicious || v.DeletedRatio != 0 {
		t.Fatalf("plain move flagged: %+v", v)
	}
}

func TestSniff(t *testing.T) {
	cases := map[string]string{
		"\xFF\xD8\xFF\xE0\x00\x10JFIF": "image/jpeg",
		"id,name\n1,ravi\n":            "text/plain",
		"%PDF-1.7\n":                   "application/pdf",
		"\x00\x01\x02\x03\x9f\x88":     "application/octet-stream",
	}
	for in, want := range cases {
		if got := Sniff([]byte(in)); got != want {
			t.Errorf("Sniff(%q) = %q, want %q", in, got, want)
		}
	}
}
