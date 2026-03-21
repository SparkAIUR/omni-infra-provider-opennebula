// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package imagemanager resolves and imports Talos images for the OpenNebula provider.
package imagemanager

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/klauspost/compress/zstd"

	providerconfig "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/observability"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
)

// ResolveRequest describes the image-resolution input for a machine reconcile.
type ResolveRequest struct {
	ImageName        string
	TalosVersion     string
	SchematicID      string
	Arch             string
	Datastore        string
	AllowImport      *bool
	ExistingImageID  int
	ExistingChecksum string
	ExistingSource   string
	ProviderManaged  bool
}

// Result is the normalized image-resolution result used by the provider state machine.
type Result struct {
	Image           opennebula.ImageRef
	SourceURL       string
	Checksum        string
	ProviderManaged bool
	Action          string
	CacheHit        bool
	ChecksumVerified bool
}

// Manager resolves an existing Talos image or imports it on demand.
//
// Example:
//
//	manager := imagemanager.New(client, cfg, metrics)
//	result, err := manager.Resolve(ctx, imagemanager.ResolveRequest{
//	    ImageName:    "talos-opennebula-amd64-v1.10.0-schematic-abc123",
//	    TalosVersion: "v1.10.0",
//	    SchematicID:  "abc123",
//	    Datastore:    "fast-ssd",
//	})
type Manager struct {
	client     opennebula.Client
	config     providerconfig.Config
	httpClient *http.Client
	metrics    *observability.Metrics
	lockMu     sync.Mutex
	locks      map[string]*sync.Mutex
	lockSeenAt map[string]time.Time
}

// New creates a Talos image manager for the OpenNebula provider.
func New(client opennebula.Client, cfg providerconfig.Config, metrics *observability.Metrics) *Manager {
	return &Manager{
		client: client,
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.ImageManagement.ImportTimeout,
		},
		metrics: metrics,
		locks:   map[string]*sync.Mutex{},
		lockSeenAt: map[string]time.Time{},
	}
}

// Resolve returns a ready image or a retryable error while an import is still progressing.
func (m *Manager) Resolve(ctx context.Context, request ResolveRequest) (Result, error) {
	if request.Arch == "" {
		request.Arch = "amd64"
	}

	datastoreName := request.Datastore
	if datastoreName == "" {
		datastoreName = m.config.StoragePolicies.DefaultDatastore
	}

	if request.ExistingImageID != 0 {
		imageInfo, err := m.client.GetImage(ctx, request.ExistingImageID)
		if err != nil {
			m.observe("get", "error")
			return Result{}, err
		}

		result, err := m.evaluate(imageInfo, request, request.ProviderManaged)
		result.Action = "reused"
		result.CacheHit = true
		result.ChecksumVerified = result.Checksum != ""
		return result, err
	}

	imageRef, err := m.lookupImage(ctx, request.ImageName, datastoreName)
	if err == nil {
		imageInfo, infoErr := m.client.GetImage(ctx, imageRef.ID)
		if infoErr != nil {
			m.observe("lookup", "error")
			return Result{}, infoErr
		}

		m.observe("lookup", "success")
		result, err := m.evaluate(imageInfo, request, request.ProviderManaged)
		result.Action = "reused"
		result.CacheHit = true
		result.ChecksumVerified = result.Checksum != ""
		return result, err
	}

	if !opennebula.IsNotFoundError(err) {
		m.observe("lookup", "error")
		return Result{}, err
	}

	allowImport := m.config.ImageManagement.ImportOnMiss
	if request.AllowImport != nil {
		allowImport = *request.AllowImport
	}

	if !allowImport {
		m.observe("lookup", "error")
		return Result{}, err
	}

	if strings.TrimSpace(m.config.ImageManagement.ArtifactURLTemplate) == "" {
		m.observe("import", "error")
		return Result{}, fmt.Errorf("%w: imageManagement.artifactURLTemplate is required for image import", opennebula.ErrPolicy)
	}

	if datastoreName == "" {
		m.observe("import", "error")
		return Result{}, fmt.Errorf("%w: datastore is required to import image %q", opennebula.ErrPolicy, request.ImageName)
	}

	datastoreRef, err := m.client.LookupDatastoreByName(ctx, datastoreName)
	if err != nil {
		m.observe("import", "error")
		return Result{}, err
	}

	sourceURL, err := m.renderURL(m.config.ImageManagement.ArtifactURLTemplate, request)
	if err != nil {
		m.observe("import", "error")
		return Result{}, err
	}

	lockKey := request.ImageName + "::" + sourceURL + "::" + datastoreName
	waited, unlock := m.acquireImportLock(lockKey)
	defer unlock()

	if imageRef, err := m.lookupImage(ctx, request.ImageName, datastoreName); err == nil {
		imageInfo, infoErr := m.client.GetImage(ctx, imageRef.ID)
		if infoErr != nil {
			m.observe("lookup", "error")
			return Result{}, infoErr
		}

		result, evalErr := m.evaluate(imageInfo, request, true)
		if waited {
			result.Action = "waited_for_existing_import"
		} else {
			result.Action = "reused"
		}
		result.CacheHit = true
		result.ChecksumVerified = result.Checksum != ""
		return result, evalErr
	}

	importRequest, checksum, err := m.prepareImport(ctx, sourceURL, request, datastoreRef.ID)
	if err != nil {
		return Result{}, err
	}
	if checksum != "" {
		m.observe("verify", "success")
	}

	imageRef, err = m.client.CreateImage(ctx, importRequest)
	if err != nil {
		m.observe("import", "error")
		return Result{}, err
	}

	imageInfo, err := m.client.GetImage(ctx, imageRef.ID)
	if err != nil {
		m.observe("import", "error")
		return Result{}, err
	}

	m.observe("import", "success")

	request.ExistingChecksum = checksum
	request.ExistingSource = sourceURL

	result, err := m.evaluate(imageInfo, request, true)
	result.Action = "imported"
	result.CacheHit = false
	result.ChecksumVerified = checksum != ""
	return result, err
}

func (m *Manager) lookupImage(ctx context.Context, imageName string, datastoreName string) (opennebula.ImageRef, error) {
	if strings.TrimSpace(datastoreName) != "" {
		return m.client.LookupImageByNameInDatastore(ctx, imageName, datastoreName)
	}

	return m.client.LookupImageByName(ctx, imageName)
}

func (m *Manager) acquireImportLock(key string) (bool, func()) {
	if !m.config.ImageManagement.ImportLock.Enabled {
		return false, func() {}
	}

	m.lockMu.Lock()
	now := time.Now()
	for seenKey, seenAt := range m.lockSeenAt {
		if now.Sub(seenAt) > m.config.ImageManagement.ImportLock.StaleAfter {
			delete(m.lockSeenAt, seenKey)
			delete(m.locks, seenKey)
		}
	}

	lock, ok := m.locks[key]
	waited := ok
	if !ok {
		lock = &sync.Mutex{}
		m.locks[key] = lock
	}
	m.lockSeenAt[key] = now
	m.lockMu.Unlock()

	lock.Lock()

	return waited, func() {
		lock.Unlock()
		m.lockMu.Lock()
		m.lockSeenAt[key] = time.Now()
		m.lockMu.Unlock()
	}
}

func (m *Manager) prepareImport(
	ctx context.Context,
	sourceURL string,
	request ResolveRequest,
	datastoreID int,
) (opennebula.CreateImageRequest, string, error) {
	if !needsLocalStaging(sourceURL) {
		checksum, err := m.resolveChecksum(ctx, sourceURL, request)
		if err != nil {
			m.observe("verify", "error")
			return opennebula.CreateImageRequest{}, "", err
		}

		return opennebula.CreateImageRequest{
			DatastoreID: datastoreID,
			Name:        request.ImageName,
			SourceURL:   sourceURL,
		}, checksum, nil
	}

	stagedPath, checksum, err := m.stageCompressedArtifact(ctx, sourceURL, request)
	if err != nil {
		m.observe("verify", "error")
		return opennebula.CreateImageRequest{}, "", err
	}

	return opennebula.CreateImageRequest{
		DatastoreID: datastoreID,
		Name:        request.ImageName,
		SourcePath:  stagedPath,
		Driver:      "raw",
		Format:      "raw",
	}, checksum, nil
}

func (m *Manager) evaluate(imageInfo opennebula.ImageInfo, request ResolveRequest, providerManaged bool) (Result, error) {
	result := Result{
		Image: opennebula.ImageRef{
			ID:        imageInfo.ID,
			Name:      imageInfo.Name,
			Datastore: imageInfo.Datastore,
			SizeMiB:   imageInfo.SizeMiB,
		},
		SourceURL:       firstNonEmpty(request.ExistingSource, imageInfo.Source),
		Checksum:        request.ExistingChecksum,
		ProviderManaged: providerManaged,
	}

	switch strings.ToUpper(imageInfo.State) {
	case "READY", "USED", "USED_PERS":
		return result, nil
	case "ERROR", "DELETE":
		return Result{}, fmt.Errorf("%w: image %q entered terminal state %s", opennebula.ErrTerminal, imageInfo.Name, imageInfo.State)
	default:
		return result, fmt.Errorf("%w: image %q not ready yet (state=%s)", opennebula.ErrRetryable, imageInfo.Name, imageInfo.State)
	}
}

func (m *Manager) stageCompressedArtifact(ctx context.Context, sourceURL string, request ResolveRequest) (string, string, error) {
	if err := os.MkdirAll(m.config.ImageManagement.StagingDir, 0o755); err != nil {
		return "", "", fmt.Errorf("%w: create staging dir: %w", opennebula.ErrTerminal, err)
	}

	stagedCompressedPath := filepath.Join(
		m.config.ImageManagement.StagingDir,
		fmt.Sprintf("%s.raw.zst", sanitizeName(request.ImageName)),
	)
	stagedRawPath := strings.TrimSuffix(stagedCompressedPath, ".zst")

	if err := m.downloadSource(ctx, sourceURL, stagedCompressedPath); err != nil {
		return "", "", err
	}
	defer os.Remove(stagedCompressedPath) //nolint:errcheck

	checksum, err := m.validateDownloadedChecksum(ctx, sourceURL, stagedCompressedPath, request)
	if err != nil {
		_ = os.Remove(stagedRawPath)
		return "", "", err
	}

	if err := decompressZstdFile(stagedCompressedPath, stagedRawPath); err != nil {
		_ = os.Remove(stagedRawPath)
		return "", "", fmt.Errorf("%w: decompress %q: %w", opennebula.ErrTerminal, sourceURL, err)
	}

	return stagedRawPath, checksum, nil
}

func (m *Manager) resolveChecksum(ctx context.Context, sourceURL string, request ResolveRequest) (string, error) {
	if request.ExistingChecksum != "" {
		return request.ExistingChecksum, nil
	}

	checksumTemplate := strings.TrimSpace(m.config.ImageManagement.ChecksumURLTemplate)
	if !m.config.ImageManagement.RequireChecksum && checksumTemplate == "" {
		return "", nil
	}

	if checksumTemplate == "" {
		actual, err := m.hashSource(ctx, sourceURL)
		if err != nil {
			return "", err
		}

		return actual, nil
	}

	checksumURL, err := m.renderURL(checksumTemplate, request)
	if err != nil {
		return "", err
	}

	expected, err := m.fetchChecksum(ctx, checksumURL)
	if err != nil {
		return "", err
	}

	if !m.config.ImageManagement.RequireChecksum {
		return expected, nil
	}

	actual, err := m.hashSource(ctx, sourceURL)
	if err != nil {
		return "", err
	}

	if !strings.EqualFold(expected, actual) {
		return "", fmt.Errorf("%w: checksum mismatch for %q", opennebula.ErrTerminal, sourceURL)
	}

	return expected, nil
}

func (m *Manager) validateDownloadedChecksum(ctx context.Context, sourceURL, localPath string, request ResolveRequest) (string, error) {
	if request.ExistingChecksum != "" {
		return request.ExistingChecksum, nil
	}

	checksumTemplate := strings.TrimSpace(m.config.ImageManagement.ChecksumURLTemplate)
	if !m.config.ImageManagement.RequireChecksum && checksumTemplate == "" {
		return "", nil
	}

	if checksumTemplate == "" {
		actual, err := hashFile(localPath)
		if err != nil {
			return "", fmt.Errorf("%w: hash staged image %q: %w", opennebula.ErrTerminal, localPath, err)
		}

		return actual, nil
	}

	checksumURL, err := m.renderURL(checksumTemplate, request)
	if err != nil {
		return "", err
	}

	expected, err := m.fetchChecksum(ctx, checksumURL)
	if err != nil {
		return "", err
	}

	if !m.config.ImageManagement.RequireChecksum {
		return expected, nil
	}

	actual, err := hashFile(localPath)
	if err != nil {
		return "", fmt.Errorf("%w: hash staged image %q: %w", opennebula.ErrTerminal, localPath, err)
	}

	if !strings.EqualFold(expected, actual) {
		return "", fmt.Errorf("%w: checksum mismatch for %q", opennebula.ErrTerminal, sourceURL)
	}

	return expected, nil
}

func (m *Manager) renderURL(templateBody string, request ResolveRequest) (string, error) {
	tpl, err := template.New("artifact-url").Parse(templateBody)
	if err != nil {
		return "", fmt.Errorf("parse image artifact template: %w", err)
	}

	talosVersionNoV := strings.TrimPrefix(strings.TrimSpace(request.TalosVersion), "v")

	var builder strings.Builder
	if err := tpl.Execute(&builder, map[string]string{
		"Arch":             request.Arch,
		"TalosVersion":     request.TalosVersion,
		"TalosVersionNoV":  talosVersionNoV,
		"SchematicID":      request.SchematicID,
	}); err != nil {
		return "", fmt.Errorf("render image artifact template: %w", err)
	}

	return builder.String(), nil
}

func (m *Manager) fetchChecksum(ctx context.Context, checksumURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create checksum request: %w", opennebula.ErrTerminal, err)
	}

	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", classifyHTTPError("fetch checksum", err)
	}
	defer response.Body.Close() //nolint:errcheck

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTPStatus("fetch checksum", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return "", classifyHTTPError("read checksum", err)
	}

	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", fmt.Errorf("%w: checksum payload at %q is empty", opennebula.ErrTerminal, checksumURL)
	}

	return strings.TrimSpace(fields[0]), nil
}

func (m *Manager) hashSource(ctx context.Context, sourceURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create source request: %w", opennebula.ErrTerminal, err)
	}

	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", classifyHTTPError("download source image", err)
	}
	defer response.Body.Close() //nolint:errcheck

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTPStatus("download source image", response.StatusCode)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, response.Body); err != nil {
		return "", classifyHTTPError("hash source image", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (m *Manager) downloadSource(ctx context.Context, sourceURL, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("%w: create source request: %w", opennebula.ErrTerminal, err)
	}

	response, err := m.httpClient.Do(request)
	if err != nil {
		return classifyHTTPError("download source image", err)
	}
	defer response.Body.Close() //nolint:errcheck

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyHTTPStatus("download source image", response.StatusCode)
	}

	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("%w: create staged image %q: %w", opennebula.ErrTerminal, destination, err)
	}
	defer file.Close() //nolint:errcheck

	if _, err := io.Copy(file, response.Body); err != nil {
		return classifyHTTPError("stage source image", err)
	}

	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func decompressZstdFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close() //nolint:errcheck

	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}

	decoder, err := zstd.NewReader(source)
	if err != nil {
		destination.Close() //nolint:errcheck
		return err
	}
	defer decoder.Close()

	if _, err := io.Copy(destination, decoder); err != nil {
		destination.Close() //nolint:errcheck
		return err
	}

	return destination.Close()
}

func needsLocalStaging(sourceURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(sourceURL))
	return strings.HasSuffix(lower, ".raw.zst") || strings.HasSuffix(lower, ".zst")
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, string(os.PathSeparator), "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "image"
	}

	return value
}

func (m *Manager) observe(action, outcome string) {
	if m.metrics == nil {
		return
	}

	m.metrics.ObserveImageOperation(action, outcome)
}

func classifyHTTPError(action string, err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %s: %w", opennebula.ErrRetryable, action, err)
	}

	return fmt.Errorf("%w: %s: %w", opennebula.ErrRetryable, action, err)
}

func classifyHTTPStatus(action string, statusCode int) error {
	if statusCode >= 500 {
		return fmt.Errorf("%w: %s returned status %d", opennebula.ErrRetryable, action, statusCode)
	}

	return fmt.Errorf("%w: %s returned status %d", opennebula.ErrTerminal, action, statusCode)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
