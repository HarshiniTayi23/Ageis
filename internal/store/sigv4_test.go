package store

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Test vectors from AWS's "Signature Calculations for the Authorization
// Header" documentation (Amazon S3 API reference, Examples section).
var exampleCreds = Credentials{
	AccessKey: "AKIAIOSFODNN7EXAMPLE",
	SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
}

var exampleTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

func signature(t *testing.T, req *http.Request, payload string) string {
	t.Helper()
	signV4(req, exampleCreds, "us-east-1", "s3", payload, exampleTime)
	auth := req.Header.Get("Authorization")
	i := strings.Index(auth, "Signature=")
	if i < 0 {
		t.Fatalf("no signature in %q", auth)
	}
	return auth[i+len("Signature="):]
}

func TestSigV4GetObject(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	got := signature(t, req, emptySHA256Hex)
	want := "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
	if !strings.Contains(req.Header.Get("Authorization"), "SignedHeaders=host;range;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("unexpected signed headers: %s", req.Header.Get("Authorization"))
	}
}

func TestSigV4PutObjectWithSpecialCharacter(t *testing.T) {
	req, _ := http.NewRequest("PUT", "https://examplebucket.s3.amazonaws.com/test$file.text", nil)
	req.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	req.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	got := signature(t, req, "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072")
	want := "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"
	if got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestSigV4SubresourceQuery(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/?lifecycle", nil)
	got := signature(t, req, emptySHA256Hex)
	want := "fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"
	if got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestSigV4ListObjects(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", nil)
	got := signature(t, req, emptySHA256Hex)
	want := "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"
	if got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}
