package imagemanager

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
		},
		StoragePolicies: providerconfig.StoragePoliciesConfig{
			DefaultDatastore: "fast-ssd",
		},
	}
}
