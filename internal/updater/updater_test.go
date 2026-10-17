package updater

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/ulikunitz/xz"
)

func TestSelectAssetPrefersTarXz(t *testing.T) {
	prefix := "vohive_v1.0.0_linux_amd64"
	assets := []Asset{
		{Name: prefix, BrowserDownloadURL: "raw"},
		{Name: prefix + archiveSuffix, BrowserDownloadURL: "archive"},
		{Name: "vohive_v1.0.0_linux_arm64.tar.xz", BrowserDownloadURL: "other"},
	}
	got, ok := selectAsset(assets, prefix)
	if !ok || got.BrowserDownloadURL != "archive" {
		t.Fatalf("selectAsset = %+v, %v; want archive asset", got, ok)
	}
}

func TestSelectAssetFallsBackToRawBinary(t *testing.T) {
	prefix := "vohive_v1.0.0_linux_amd64"
	got, ok := selectAsset([]Asset{{Name: prefix, BrowserDownloadURL: "raw"}}, prefix)
	if !ok || got.BrowserDownloadURL != "raw" {
		t.Fatalf("selectAsset = %+v, %v; want raw asset", got, ok)
	}
	if _, ok := selectAsset([]Asset{{Name: prefix + ".sha256"}}, prefix); ok {
		t.Fatal("selectAsset matched an unrelated asset")
	}
}

func TestExtractBinaryFromTarXz(t *testing.T) {
	payload := []byte("\x7fELF fake binary")
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(xw)
	if err := tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "vohive", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := extractBinaryFromTarXz(&buf)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("extracted %q, want %q", got, payload)
	}
}
