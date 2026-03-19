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
	"strings"
	"text/template"

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
	}
}

// Resolve returns a ready image or a retryable error while an import is still progressing.
func (m *Manager) Resolve(ctx context.Context, request ResolveRequest) (Result, error) {
	if request.Arch == "" {
		request.Arch = "amd64"
	}

	if request.ExistingImageID != 0 {
		imageInfo, err := m.client.GetImage(ctx, request.ExistingImageID)
		if err != nil {
			m.observe("get", "error")
			return Result{}, err
		}

		return m.evaluate(imageInfo, request, request.ProviderManaged)
	}

	imageRef, err := m.client.LookupImageByName(ctx, request.ImageName)
	if err == nil {
		imageInfo, infoErr := m.client.GetImage(ctx, imageRef.ID)
		if infoErr != nil {
			m.observe("lookup", "error")
			return Result{}, infoErr
		}

		m.observe("lookup", "success")
		return m.evaluate(imageInfo, request, request.ProviderManaged)
	}

	if !opennebula.IsNotFoundError(err) {
		m.observe("lookup", "error")
		return Result{}, err
	}

	if !m.config.ImageManagement.ImportOnMiss {
		m.observe("lookup", "error")
		return Result{}, err
	}

	if strings.TrimSpace(m.config.ImageManagement.ArtifactURLTemplate) == "" {
		m.observe("import", "error")
		return Result{}, fmt.Errorf("%w: imageManagement.artifactURLTemplate is required for image import", opennebula.ErrPolicy)
	}

	datastoreName := request.Datastore
	if datastoreName == "" {
		datastoreName = m.config.StoragePolicies.DefaultDatastore
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

	checksum, err := m.resolveChecksum(ctx, sourceURL, request)
	if err != nil {
		m.observe("verify", "error")
		return Result{}, err
	}
	if checksum != "" {
		m.observe("verify", "success")
	}

	imageRef, err = m.client.CreateImage(ctx, opennebula.CreateImageRequest{
		DatastoreID: datastoreRef.ID,
		Name:        request.ImageName,
		SourceURL:   sourceURL,
	})
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

	return m.evaluate(imageInfo, request, true)
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
	case "READY":
		return result, nil
	case "ERROR", "DELETE":
		return Result{}, fmt.Errorf("%w: image %q entered terminal state %s", opennebula.ErrTerminal, imageInfo.Name, imageInfo.State)
	default:
		return result, fmt.Errorf("%w: image %q not ready yet (state=%s)", opennebula.ErrRetryable, imageInfo.Name, imageInfo.State)
	}
}

func (m *Manager) resolveChecksum(ctx context.Context, sourceURL string, request ResolveRequest) (string, error) {
	if request.ExistingChecksum != "" {
		return request.ExistingChecksum, nil
	}

	if !m.config.ImageManagement.RequireChecksum && strings.TrimSpace(m.config.ImageManagement.ChecksumURLTemplate) == "" {
		return "", nil
	}

	if strings.TrimSpace(m.config.ImageManagement.ChecksumURLTemplate) == "" {
		return "", fmt.Errorf("%w: imageManagement.checksumURLTemplate is required when requireChecksum is enabled", opennebula.ErrPolicy)
	}

	checksumURL, err := m.renderURL(m.config.ImageManagement.ChecksumURLTemplate, request)
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

func (m *Manager) renderURL(templateBody string, request ResolveRequest) (string, error) {
	tpl, err := template.New("artifact-url").Parse(templateBody)
	if err != nil {
		return "", fmt.Errorf("parse image artifact template: %w", err)
	}

	var builder strings.Builder
	if err := tpl.Execute(&builder, map[string]string{
		"Arch":         request.Arch,
		"TalosVersion": request.TalosVersion,
		"SchematicID":  request.SchematicID,
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
