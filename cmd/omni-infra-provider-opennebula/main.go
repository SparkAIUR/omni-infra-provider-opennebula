// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main is the root command of the OpenNebula provider service.
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cosi-project/runtime/pkg/resource/protobuf"
	"github.com/siderolabs/omni/client/pkg/client"
	"github.com/siderolabs/omni/client/pkg/infra"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/observability"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider"
	providermeta "github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/meta"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/resources"
)

//go:embed data/schema.json
var schema string

//go:embed data/icon.svg
var icon []byte

var rootCmd = &cobra.Command{
	Use:          "omni-infra-provider-opennebula",
	Short:        "OpenNebula Omni infrastructure provider",
	Long:         `Connects to Omni as an infrastructure provider and manages Talos VMs in OpenNebula.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		runCtx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		loggerConfig := zap.NewProductionConfig()
		logger, err := loggerConfig.Build(zap.AddStacktrace(zapcore.ErrorLevel))
		if err != nil {
			return fmt.Errorf("create logger: %w", err)
		}

		if cfg.omniAPIEndpoint == "" {
			return fmt.Errorf("omni api endpoint is not set")
		}

		configFile, err := os.Open(cfg.configFile)
		if err != nil {
			return fmt.Errorf("open config file %q: %w", cfg.configFile, err)
		}
		defer configFile.Close() //nolint:errcheck

		runtimeConfig, err := config.Load(configFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		authConfig, err := config.ResolveAuth()
		if err != nil {
			return err
		}

		metrics := observability.NewMetrics()
		obsServer := observability.NewServer(runtimeConfig.Observability, metrics)

		baseClient, err := opennebula.NewClient(runtimeConfig, authConfig)
		if err != nil {
			return fmt.Errorf("create opennebula client: %w", err)
		}

		clientOptions := []client.Option{
			client.WithInsecureSkipTLSVerify(cfg.insecureSkipVerify),
		}
		if cfg.serviceAccountKey != "" {
			clientOptions = append(clientOptions, client.WithServiceAccount(cfg.serviceAccountKey))
		}

		omniClient, err := client.New(cfg.omniAPIEndpoint, clientOptions...)
		if err != nil {
			return fmt.Errorf("create omni client: %w", err)
		}
		defer omniClient.Close() //nolint:errcheck

		omniState, err := infra.NewState(omniClient)
		if err != nil {
			return fmt.Errorf("create omni state: %w", err)
		}

		stateHandle := omniState.State()

		opennebulaClient := opennebula.Instrument(baseClient, metrics)
		provisioner := provider.NewProvisioner(opennebulaClient, runtimeConfig, metrics, stateHandle)

		if err := protobuf.RegisterResource(
			resources.NewNameReservation("", "").ResourceDefinition().Type,
			resources.NewNameReservation("", ""),
		); err != nil {
			return err
		}

		ip, err := infra.NewProvider(providermeta.ProviderID, provisioner, infra.ProviderConfig{
			Name:        cfg.providerName,
			Description: cfg.providerDescription,
			Icon:        base64.RawStdEncoding.EncodeToString(icon),
			Schema:      schema,
		})
		if err != nil {
			return fmt.Errorf("create infra provider: %w", err)
		}

		logger.Info(
			"starting opennebula infra provider",
			zap.String("omni_endpoint", cfg.omniAPIEndpoint),
			zap.String("opennebula_endpoint", runtimeConfig.OpenNebula.Endpoint),
			zap.String("resource_pool", runtimeConfig.OpenNebula.ResourcePool),
			zap.String("auth_mode", authConfig.Mode()),
			zap.Any("auth", authConfig.Redacted()),
			zap.String("observability_address", runtimeConfig.Observability.ListenAddress),
		)

		if err := obsServer.Start(runCtx, logger.Named("observability")); err != nil {
			return fmt.Errorf("start observability server: %w", err)
		}

		obsServer.SetReady(true)
		defer obsServer.SetReady(false)

		return ip.Run(
			runCtx,
			logger,
			infra.WithState(stateHandle),
			infra.WithConcurrency(5),
		)
	},
}

var explainCmd = &cobra.Command{
	Use:          "explain",
	Short:        "Resolve provider inputs without mutating OpenNebula",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		runtimeConfig, err := loadRuntimeConfig()
		if err != nil {
			return err
		}
		if !runtimeConfig.Explain.Enabled {
			return fmt.Errorf("explain is disabled by runtime config")
		}

		authConfig, err := config.ResolveAuth()
		if err != nil {
			return err
		}

		client, err := opennebula.NewClient(runtimeConfig, authConfig)
		if err != nil {
			return fmt.Errorf("create opennebula client: %w", err)
		}

		payload, err := os.ReadFile(cfg.providerDataFile)
		if err != nil {
			return fmt.Errorf("read providerData file %q: %w", cfg.providerDataFile, err)
		}

		data, err := provider.ParseProviderData(payload)
		if err != nil {
			return err
		}

		result, err := provider.NewProvisioner(client, runtimeConfig, nil, nil).Explain(cmd.Context(), provider.ExplainInput{
			ProviderData: data,
			TalosVersion: cfg.explainTalosVersion,
			SchematicID:  cfg.explainSchematicID,
			Architecture: cfg.explainArch,
		})
		if err != nil {
			return err
		}

		return writeJSON(os.Stdout, result)
	},
}

var supportBundleCmd = &cobra.Command{
	Use:          "support-bundle",
	Short:        "Generate a non-mutating debug bundle for a providerData payload",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		runtimeConfig, err := loadRuntimeConfig()
		if err != nil {
			return err
		}
		if !runtimeConfig.Support.Bundle.Enabled {
			return fmt.Errorf("support bundle generation is disabled by runtime config")
		}

		authConfig, err := config.ResolveAuth()
		if err != nil {
			return err
		}

		client, err := opennebula.NewClient(runtimeConfig, authConfig)
		if err != nil {
			return fmt.Errorf("create opennebula client: %w", err)
		}

		payload, err := os.ReadFile(cfg.providerDataFile)
		if err != nil {
			return fmt.Errorf("read providerData file %q: %w", cfg.providerDataFile, err)
		}

		data, err := provider.ParseProviderData(payload)
		if err != nil {
			return err
		}

		bundle, err := provider.NewProvisioner(client, runtimeConfig, nil, nil).BuildSupportBundle(cmd.Context(), provider.ExplainInput{
			ProviderData: data,
			TalosVersion: cfg.explainTalosVersion,
			SchematicID:  cfg.explainSchematicID,
			Architecture: cfg.explainArch,
		})
		if err != nil {
			return err
		}

		return writeJSON(os.Stdout, bundle)
	},
}

var cfg struct {
	omniAPIEndpoint     string
	serviceAccountKey   string
	providerName        string
	providerDescription string
	configFile          string
	insecureSkipVerify  bool
	providerDataFile    string
	explainTalosVersion string
	explainSchematicID  string
	explainArch         string
}

func main() {
	if err := app(); err != nil {
		os.Exit(1)
	}
}

func app() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.Flags().StringVar(&cfg.omniAPIEndpoint, "omni-api-endpoint", os.Getenv("OMNI_ENDPOINT"),
		"the endpoint of the Omni API, defaults to OMNI_ENDPOINT")
	rootCmd.Flags().StringVar(&providermeta.ProviderID, "id", providermeta.ProviderID, "the infra provider id")
	rootCmd.Flags().StringVar(&cfg.serviceAccountKey, "omni-service-account-key", os.Getenv("OMNI_SERVICE_ACCOUNT_KEY"),
		"the Omni service account key, defaults to OMNI_SERVICE_ACCOUNT_KEY")
	rootCmd.Flags().StringVar(&cfg.providerName, "provider-name", "opennebula", "provider name as it appears in Omni")
	rootCmd.Flags().StringVar(&cfg.providerDescription, "provider-description", "OpenNebula infrastructure provider", "provider description as it appears in Omni")
	rootCmd.Flags().BoolVar(&cfg.insecureSkipVerify, "insecure-skip-verify", false, "ignore untrusted Omni certificates")
	rootCmd.Flags().StringVar(&cfg.configFile, "config-file", "", "provider config file")
	_ = rootCmd.MarkFlagRequired("config-file")

	explainCmd.Flags().StringVar(&cfg.providerDataFile, "provider-data-file", "", "providerData YAML/JSON file")
	explainCmd.Flags().StringVar(&cfg.explainTalosVersion, "talos-version", "v1.10.0", "Talos version used for image prediction")
	explainCmd.Flags().StringVar(&cfg.explainSchematicID, "schematic-id", "default", "schematic id used for image prediction")
	explainCmd.Flags().StringVar(&cfg.explainArch, "arch", "amd64", "target architecture used for image prediction")
	_ = explainCmd.MarkFlagRequired("provider-data-file")
	rootCmd.AddCommand(explainCmd)

	supportBundleCmd.Flags().StringVar(&cfg.providerDataFile, "provider-data-file", "", "providerData YAML/JSON file")
	supportBundleCmd.Flags().StringVar(&cfg.explainTalosVersion, "talos-version", "v1.10.0", "Talos version used for image prediction")
	supportBundleCmd.Flags().StringVar(&cfg.explainSchematicID, "schematic-id", "default", "schematic id used for image prediction")
	supportBundleCmd.Flags().StringVar(&cfg.explainArch, "arch", "amd64", "target architecture used for image prediction")
	_ = supportBundleCmd.MarkFlagRequired("provider-data-file")
	rootCmd.AddCommand(supportBundleCmd)
}

func loadRuntimeConfig() (config.Config, error) {
	configFile, err := os.Open(cfg.configFile)
	if err != nil {
		return config.Config{}, fmt.Errorf("open config file %q: %w", cfg.configFile, err)
	}
	defer configFile.Close() //nolint:errcheck

	runtimeConfig, err := config.Load(configFile)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	return runtimeConfig, nil
}

func writeJSON(file *os.File, value any) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
