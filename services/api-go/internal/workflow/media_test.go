package workflow

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInspectAcceptsSupportedFiles(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		declared string
		body     []byte
		wantType string
		wantName string
	}{
		{name: "jpeg", file: "street.JPG", declared: "image/jpeg", body: []byte{0xFF, 0xD8, 0xFF, 0xD9}, wantType: "image/jpeg", wantName: "media.jpg"},
		{name: "png", file: "street.png", declared: "image/png", body: minimalPNG(), wantType: "image/png", wantName: "media.png"},
		{name: "gif", file: "street.gif", declared: "image/gif", body: minimalGIF(), wantType: "image/gif", wantName: "media.gif"},
		{name: "webp", file: "street.webp", declared: "image/webp", body: minimalWEBP(), wantType: "image/webp", wantName: "media.webp"},
		{name: "mp3", file: "note.mp3", declared: "audio/mpeg", body: append([]byte("ID3"), bytes.Repeat([]byte{0}, 7)...), wantType: "audio/mpeg", wantName: "media.mp3"},
		{name: "m4a", file: "note.m4a", declared: "audio/mp4", body: ftyp("M4A "), wantType: "audio/mp4", wantName: "media.m4a"},
		{name: "mp4", file: "clip.mp4", declared: "video/mp4", body: ftyp("isom"), wantType: "video/mp4", wantName: "media.mp4"},
		{name: "webm", file: "clip.webm", declared: "video/webm", body: []byte{0x1A, 0x45, 0xDF, 0xA3, 'w', 'e', 'b', 'm'}, wantType: "video/webm", wantName: "media.webm"},
		{name: "weba", file: "note.weba", declared: "audio/webm", body: []byte{0x1A, 0x45, 0xDF, 0xA3, 'w', 'e', 'b', 'm'}, wantType: "audio/webm", wantName: "media.weba"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotName, err := InspectUpload(tc.file, tc.declared, tc.body)
			if err != nil {
				t.Fatal(err)
			}
			if gotType != tc.wantType || gotName != tc.wantName {
				t.Fatalf("got %s %s", gotType, gotName)
			}
		})
	}
}

func TestInspectRejectsUnsafeFiles(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	cases := []struct {
		name     string
		file     string
		declared string
		body     []byte
		want     string
	}{
		{name: "spoofed mime", file: "street.jpg", declared: "image/png", body: jpeg, want: "declared type"},
		{name: "mismatched extension", file: "street.png", declared: "image/jpeg", body: jpeg, want: "file name"},
		{name: "malformed jpeg", file: "street.jpg", declared: "image/jpeg", body: []byte{0xFF, 0xD8, 0xFF}, want: "not a complete"},
		{name: "unsupported", file: "notes.txt", declared: "text/plain", body: []byte("not a media file"), want: "not a complete"},
		{name: "executable signature", file: "photo.jpg", declared: "image/jpeg", body: []byte{'M', 'Z', 0xFF, 0xD8}, want: "not supported"},
		{name: "empty", file: "street.jpg", declared: "image/jpeg", body: nil, want: "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := InspectUpload(tc.file, tc.declared, tc.body)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestInspectRejectsOversizedFile(t *testing.T) {
	body := bytes.Repeat([]byte{0xFF}, maxMediaBytes+1)
	_, _, err := InspectUpload("street.jpg", "image/jpeg", body)
	if err == nil || !strings.Contains(err.Error(), "4 MB") {
		t.Fatal(err)
	}
}

func TestUnconfiguredScannerDoesNotClaimClean(t *testing.T) {
	result, err := UnconfiguredScanner{}.Scan(context.Background(), "image/jpeg", []byte{0xFF, 0xD8})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ScanNotScanned {
		t.Fatalf("status = %s", result.Status)
	}
}

func minimalPNG() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89,
		0x00, 0x00, 0x00, 0x0A, 'I', 'D', 'A', 'T', 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4,
		0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xAE, 0x42, 0x60, 0x82,
	}
}

func minimalGIF() []byte {
	return []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
}

func minimalWEBP() []byte {
	return []byte("RIFF\x04\x00\x00\x00WEBP")
}

func ftyp(brand string) []byte {
	body := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p'}
	return append(body, []byte(brand)...)
}
