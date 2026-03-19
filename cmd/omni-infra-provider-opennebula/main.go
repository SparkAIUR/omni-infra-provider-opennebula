// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main is the root command of the OpenNebula provider service.
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/siderolabs/omni/client/pkg/client"
	"github.com/siderolabs/omni/client/pkg/infra"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/config"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/observability"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/opennebula"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider"
	"github.com/SparkAIUR/omni-infra-provider-opennebula/internal/pkg/provider/meta"
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

		opennebulaClient := opennebula.Instrument(baseClient, metrics)
		provisioner := provider.NewProvisioner(opennebulaClient, runtimeConfig, metrics)

		ip, err := infra.NewProvider(meta.ProviderID, provisioner, infra.ProviderConfig{
			Name:        cfg.providerName,
			Description: cfg.providerDescription,
			Icon:        base64.RawStdEncoding.EncodeToString(icon),
			Schema:      schema,
		})
		if err != nil {
			return fmt.Errorf("create infra provider: %w", err)
		}

		clientOptions := []client.Option{
			client.WithInsecureSkipTLSVerify(cfg.insecureSkipVerify),
		}
		if cfg.serviceAccountKey != "" {
			clientOptions = append(clientOptions, client.WithServiceAccount(cfg.serviceAccountKey))
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
			infra.WithOmniEndpoint(cfg.omniAPIEndpoint),
			infra.WithClientOptions(clientOptions...),
			infra.WithConcurrency(5),
		)
	},
}

var cfg struct {
	omniAPIEndpoint     string
	serviceAccountKey   string
	providerName        string
	providerDescription string
	configFile          string
	insecureSkipVerify  bool
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
	rootCmd.Flags().StringVar(&meta.ProviderID, "id", meta.ProviderID, "the infra provider id")
	rootCmd.Flags().StringVar(&cfg.serviceAccountKey, "omni-service-account-key", os.Getenv("OMNI_SERVICE_ACCOUNT_KEY"),
		"the Omni service account key, defaults to OMNI_SERVICE_ACCOUNT_KEY")
	rootCmd.Flags().StringVar(&cfg.providerName, "provider-name", "opennebula", "provider name as it appears in Omni")
	rootCmd.Flags().StringVar(&cfg.providerDescription, "provider-description", "OpenNebula infrastructure provider", "provider description as it appears in Omni")
	rootCmd.Flags().BoolVar(&cfg.insecureSkipVerify, "insecure-skip-verify", false, "ignore untrusted Omni certificates")
	rootCmd.Flags().StringVar(&cfg.configFile, "config-file", "", "provider config file")
	_ = rootCmd.MarkFlagRequired("config-file")
}
