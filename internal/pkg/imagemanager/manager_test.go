package imagemanager

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	opennebulafake "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula/fake"
)

func TestResolveUsesExistingImage(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	client.Images["talos-image"] = opennebula.ImageRef{ID: 11, Name: "talos-image", Datastore: "fast-ssd"}
	client.ImageInfoByID[11] = opennebula.ImageInfo{
		ID:        11,
		Name:      "talos-image",
		Datastore: "fast-ssd",
		State:     "READY",
		Source:    "https://example.invalid/disk.qcow2",
	}

	manager := New(client, imageConfig("unused", "unused"), nil)

	result, err := manager.Resolve(t.Context(), ResolveRequest{ImageName: "talos-image"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Image.ID != 11 || result.Image.Name != "talos-image" {
		t.Fatalf("unexpected image result: %+v", result)
	}
}

func TestResolveImportsMissingImageWithChecksumVerification(t *testing.T) {
	t.Parallel()

	payload := []byte("talos-image-data")
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/disk.qcow2":
			_, _ = w.Write(payload)
		case "/disk.qcow2.sha256":
			_, _ = w.Write([]byte(checksum + "  disk.qcow2\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := opennebulafake.New()
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}

	manager := New(client, imageConfig(server.URL+"/disk.qcow2", server.URL+"/disk.qcow2.sha256"), nil)

	result, err := manager.Resolve(t.Context(), ResolveRequest{
		ImageName:    "talos-image",
		Datastore:    "fast-ssd",
		TalosVersion: "v1.9.0",
		SchematicID:  "abcd1234",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !result.ProviderManaged {
		t.Fatalf("expected provider-managed image result, got %+v", result)
	}

	if result.Checksum != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, result.Checksum)
	}

	if client.LastCreateImage.Name != "talos-image" {
		t.Fatalf("expected image import request, got %+v", client.LastCreateImage)
	}

	if client.LastCreateImage.SourceURL == "" {
		t.Fatalf("expected direct URL import, got %+v", client.LastCreateImage)
	}
}

func TestResolveStagesCompressedRawArtifactsBeforeImport(t *testing.T) {
	t.Parallel()

	stagingDir := t.TempDir()
	payload := []byte("talos-opennebula-raw-image")
	compressed := compressZstd(t, payload)
	checksum := fmt.Sprintf("%x", sha256.Sum256(compressed))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/opennebula-amd64.raw.zst":
			_, _ = w.Write(compressed)
		case "/opennebula-amd64.raw.zst.sha256":
			_, _ = w.Write([]byte(checksum + "  opennebula-amd64.raw.zst\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := opennebulafake.New()
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}

	manager := New(client, providerconfig.Config{
		ImageManagement: providerconfig.ImageManagementConfig{
			ImportOnMiss:        true,
			RequireChecksum:     true,
			ArtifactURLTemplate: server.URL + "/opennebula-amd64.raw.zst",
			ChecksumURLTemplate: server.URL + "/opennebula-amd64.raw.zst.sha256",
			StagingDir:          stagingDir,
		},
		StoragePolicies: providerconfig.StoragePoliciesConfig{
			DefaultDatastore: "fast-ssd",
		},
	}, nil)

	result, err := manager.Resolve(t.Context(), ResolveRequest{
		ImageName:    "talos-opennebula-image",
		Datastore:    "fast-ssd",
		TalosVersion: "v1.9.0",
		SchematicID:  "abcd1234",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Checksum != checksum {
		t.Fatalf("expected checksum %q, got %q", checksum, result.Checksum)
	}

	if client.LastCreateImage.SourceURL != "" {
		t.Fatalf("expected staged local image import, got %+v", client.LastCreateImage)
	}

	if client.LastCreateImage.SourcePath == "" {
		t.Fatalf("expected staged local image path, got %+v", client.LastCreateImage)
	}

	if client.LastCreateImage.Format != "raw" || client.LastCreateImage.Driver != "raw" {
		t.Fatalf("expected raw import settings, got %+v", client.LastCreateImage)
	}

	staged, err := os.ReadFile(client.LastCreateImage.SourcePath)
	if err != nil {
		t.Fatalf("read staged image: %v", err)
	}

	if !bytes.Equal(staged, payload) {
		t.Fatalf("unexpected staged payload: got %q", staged)
	}

	if filepath.Ext(client.LastCreateImage.SourcePath) == ".zst" {
		t.Fatalf("expected decompressed staged path, got %q", client.LastCreateImage.SourcePath)
	}
}

func TestResolveReturnsRetryWhileImageIsImporting(t *testing.T) {
	t.Parallel()

	client := opennebulafake.New()
	client.ImageInfoByID[21] = opennebula.ImageInfo{
		ID:        21,
		Name:      "talos-image",
		Datastore: "fast-ssd",
		State:     "LOCKED",
		Source:    "https://example.invalid/disk.qcow2",
	}

	manager := New(client, imageConfig("unused", "unused"), nil)

	_, err := manager.Resolve(t.Context(), ResolveRequest{
		ImageName:       "talos-image",
		ExistingImageID: 21,
		ProviderManaged: true,
	})
	if err == nil {
		t.Fatal("expected retryable error while image is importing")
	}
}

func TestResolveRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/disk.qcow2":
			_, _ = w.Write([]byte("actual-image"))
		case "/disk.qcow2.sha256":
			_, _ = w.Write([]byte("deadbeef  disk.qcow2\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := opennebulafake.New()
	client.Datastores["fast-ssd"] = opennebula.DatastoreRef{ID: 31, Name: "fast-ssd"}

	manager := New(client, imageConfig(server.URL+"/disk.qcow2", server.URL+"/disk.qcow2.sha256"), nil)

	_, err := manager.Resolve(t.Context(), ResolveRequest{
		ImageName: "talos-image",
		Datastore: "fast-ssd",
	})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func imageConfig(artifactURL, checksumURL string) providerconfig.Config {
	return providerconfig.Config{
		ImageManagement: providerconfig.ImageManagementConfig{
			ImportOnMiss:        true,
			RequireChecksum:     true,
			ArtifactURLTemplate: artifactURL,
			ChecksumURLTemplate: checksumURL,
			StagingDir:          "/tmp/omni-provider-tests",
		},
		StoragePolicies: providerconfig.StoragePoliciesConfig{
			DefaultDatastore: "fast-ssd",
		},
	}
}

func compressZstd(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer

	encoder, err := zstd.NewWriter(&buffer)
	if err != nil {
		t.Fatalf("create zstd encoder: %v", err)
	}

	if _, err := encoder.Write(payload); err != nil {
		t.Fatalf("write zstd payload: %v", err)
	}

	if err := encoder.Close(); err != nil {
		t.Fatalf("close zstd encoder: %v", err)
	}

	return buffer.Bytes()
}
