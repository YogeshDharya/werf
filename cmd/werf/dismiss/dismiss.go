package dismiss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/common-go/pkg/util"
	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/werf/v2/cmd/werf/common"
	"github.com/werf/werf/v2/pkg/config/deploy_params"
	"github.com/werf/werf/v2/pkg/giterminism_manager"
	"github.com/werf/werf/v2/pkg/true_git"
	"github.com/werf/werf/v2/pkg/werf/global_warnings"
)

var cmdData struct {
	WithNamespace bool
	WithHooks     bool
}

var commonCmdData common.CmdData

func NewCmd(ctx context.Context) *cobra.Command {
	ctx = common.NewContextWithCmdData(ctx, &commonCmdData)
	cmd := common.SetCommandContext(ctx, &cobra.Command{
		Use:   "dismiss",
		Short: "Delete werf release from Kubernetes",
		Long:  common.GetLongCommandDescription(GetDismissDocs().Long),
		Example: `  # Dismiss werf release with release name and namespace autogenerated from werf.yaml configuration (Git required):
  $ werf dismiss --env dev

  # Dismiss werf release with explicitly specified release name and namespace (no Git required):
  $ werf dismiss --namespace mynamespace --release myrelease-dev

  # Save the deploy report with the "converge" command and use namespace and release name, saved in this deploy report, in the "dismiss" command (no Git required for dismiss):
  $ werf converge --save-deploy-report --env dev
  $ cp .werf-deploy-report.json /anywhere/
  $ cd /anywhere
  $ werf dismiss --use-deploy-report  # Git not needed anymore, only the deploy report file.

  # Dismiss with namespace:
  $ werf dismiss --env dev --with-namespace`,
		DisableFlagsInUseLine: true,
		Annotations: map[string]string{
			common.DocsLongMD: GetDismissDocs().LongMD,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			defer global_warnings.PrintGlobalWarnings(ctx)

			if err := common.ProcessLogOptions(&commonCmdData); err != nil {
				common.PrintHelp(cmd)
				return err
			}
			common.LogVersion()

			return common.LogRunningTime(func() error {
				return runDismiss(ctx)
			})
		},
	})

	common.SetupTmpDir(&commonCmdData, cmd, common.SetupTmpDirOptions{})
	common.SetupConfigTemplatesDir(&commonCmdData, cmd)
	common.SetupConfigPath(&commonCmdData, cmd)
	common.SetupGiterminismConfigPath(&commonCmdData, cmd)
	common.SetupEnvironment(&commonCmdData, cmd)

	common.SetupGiterminismOptions(&commonCmdData, cmd)

	common.SetupHomeDir(&commonCmdData, cmd, common.SetupHomeDirOptions{})
	common.SetupDir(&commonCmdData, cmd)
	common.SetupGitWorkTree(&commonCmdData, cmd)

	common.SetupSecondaryStagesStorageOptions(&commonCmdData, cmd)
	common.SetupCacheStagesStorageOptions(&commonCmdData, cmd)
	common.SetupRepoOptions(&commonCmdData, cmd, common.RepoDataOptions{})
	common.SetupFinalRepo(&commonCmdData, cmd)
	common.SetupSynchronization(&commonCmdData, cmd)

	common.SetupRelease(&commonCmdData, cmd, true)
	common.SetupNamespace(&commonCmdData, cmd, true)

	common.SetupUseDeployReport(&commonCmdData, cmd)
	common.SetupDeployReportPath(&commonCmdData, cmd)

	common.SetupKubeConfig(&commonCmdData, cmd)
	common.SetupKubeConfigBase64(&commonCmdData, cmd)
	common.SetupKubeContext(&commonCmdData, cmd)
	common.SetupSkipTLSVerifyKube(&commonCmdData, cmd)
	common.SetupKubeApiServer(&commonCmdData, cmd)
	common.SetupKubeCaPath(&commonCmdData, cmd)
	common.SetupKubeTlsServer(&commonCmdData, cmd)
	common.SetupKubeToken(&commonCmdData, cmd)

	common.SetupStatusProgressPeriod(&commonCmdData, cmd)
	common.SetupHooksStatusProgressPeriod(&commonCmdData, cmd)
	common.SetupReleasesHistoryMax(&commonCmdData, cmd)

	common.SetupDockerConfig(&commonCmdData, cmd, "")

	common.SetupLogOptions(&commonCmdData, cmd)
	common.SetupLogProjectDir(&commonCmdData, cmd)

	common.SetupDisableAutoHostCleanup(&commonCmdData, cmd)
	common.SetupAllowedDockerStorageVolumeUsage(&commonCmdData, cmd)
	common.SetupAllowedDockerStorageVolumeUsageMargin(&commonCmdData, cmd)
	common.SetupAllowedLocalCacheVolumeUsage(&commonCmdData, cmd)
	common.SetupAllowedLocalCacheVolumeUsageMargin(&commonCmdData, cmd)
	common.SetupDockerServerStoragePath(&commonCmdData, cmd)

	common.SetupInsecureRegistry(&commonCmdData, cmd)
	common.SetupInsecureHelmDependencies(&commonCmdData, cmd, true)
	common.SetupSkipTlsVerifyRegistry(&commonCmdData, cmd)
	common.SetupContainerRegistryMirror(&commonCmdData, cmd)

	commonCmdData.SetupPlatform(cmd)

	cmd.Flags().BoolVarP(&cmdData.WithNamespace, "with-namespace", "", util.GetBoolEnvironmentDefaultFalse("WERF_WITH_NAMESPACE"), "Delete Kubernetes Namespace after purging Helm Release (default $WERF_WITH_NAMESPACE)")
	cmd.Flags().BoolVarP(&cmdData.WithHooks, "with-hooks", "", util.GetBoolEnvironmentDefaultTrue("WERF_WITH_HOOKS"), "Delete Helm Release hooks getting from existing revisions (default $WERF_WITH_HOOKS or true)")

	return cmd
}

func runDismiss(ctx context.Context) error {
	_, ctx, err := common.InitCommonComponents(ctx, common.InitCommonComponentsOptions{
		Cmd: &commonCmdData,
		InitTrueGitWithOptions: &common.InitTrueGitOptions{
			Options: true_git.Options{LiveGitOutput: *commonCmdData.LogDebug},
		},
		InitProcessContainerBackend: true,
		InitWerf:                    true,
		InitGitDataManager:          true,
		InitManifestCache:           true,
		InitLRUImagesCache:          true,
	})
	if err != nil {
		return fmt.Errorf("component init error: %w", err)
	}

	common.LogKubeContext(kube.Context)

	giterminismManager, err := common.GetGiterminismManager(ctx, &commonCmdData)
	var gitNotFoundErr *common.GitWorktreeNotFoundError
	if err != nil {
		if !errors.As(err, &gitNotFoundErr) {
			return fmt.Errorf("get giterminism manager: %w", err)
		}
	}

	releaseNamespace, releaseName, err := getNamespaceAndRelease(
		ctx,
		gitNotFoundErr == nil,
		giterminismManager,
	)
	if err != nil {
		return fmt.Errorf("get release name and namespace: %w", err)
	}

	if err := action.Uninstall(ctx, action.UninstallOptions{
		DeleteHooks:                cmdData.WithHooks,
		DeleteReleaseNamespace:     cmdData.WithNamespace,
		KubeAPIServerName:          *commonCmdData.KubeApiServer,
		KubeCAPath:                 *commonCmdData.KubeCaPath,
		KubeConfigBase64:           *commonCmdData.KubeConfigBase64,
		KubeConfigPaths:            append([]string{*commonCmdData.KubeConfig}, *commonCmdData.KubeConfigPathMergeList...),
		KubeContext:                *commonCmdData.KubeContext,
		KubeSkipTLSVerify:          *commonCmdData.SkipTlsVerifyKube,
		KubeTLSServerName:          *commonCmdData.KubeTlsServer,
		KubeToken:                  *commonCmdData.KubeToken,
		LogDebug:                   *commonCmdData.LogDebug,
		ProgressTablePrintInterval: time.Duration(*commonCmdData.StatusProgressPeriodSeconds) * time.Second,
		ReleaseHistoryLimit:        *commonCmdData.ReleasesHistoryMax,
		ReleaseName:                releaseName,
		ReleaseNamespace:           releaseNamespace,
		ReleaseStorageDriver:       action.ReleaseStorageDriver(os.Getenv("HELM_DRIVER")),
	}); err != nil {
		return fmt.Errorf("release uninstall: %w", err)
	}

	return nil
}

func getNamespaceAndRelease(
	ctx context.Context,
	gitFound bool,
	giterminismMgr giterminism_manager.Interface,
) (string, string, error) {
	namespaceSpecified := *commonCmdData.Namespace != ""
	releaseSpecified := *commonCmdData.Release != ""

	var namespace string
	var release string
	if common.GetUseDeployReport(&commonCmdData) {
		if namespaceSpecified || releaseSpecified {
			return "", "", fmt.Errorf("--namespace or --release can't be used together with --use-deploy-report")
		}

		deployReportPath, err := common.GetDeployReportPath(&commonCmdData)
		if err != nil {
			return "", "", fmt.Errorf("unable to get deploy report path: %w", err)
		}

		deployReportByte, err := os.ReadFile(deployReportPath)
		if err != nil {
			return "", "", fmt.Errorf("unable to read deploy report file %q: %w", deployReportPath, err)
		}

		var deployReport helmrelease.DeployReport
		if err := json.Unmarshal(deployReportByte, &deployReport); err != nil {
			return "", "", fmt.Errorf("unable to unmarshal deploy report file %q: %w", deployReportPath, err)
		}

		if deployReport.Namespace == "" {
			return "", "", fmt.Errorf("unable to get namespace from deploy report file %q", deployReportPath)
		}

		if deployReport.Release == "" {
			return "", "", fmt.Errorf("unable to get release from deploy report file %q", deployReportPath)
		}

		namespace = deployReport.Namespace
		release = deployReport.Release
	} else if gitFound {
		common.ProcessLogProjectDir(&commonCmdData, giterminismMgr.ProjectDir())

		_, werfConfig, err := common.GetRequiredWerfConfig(ctx, &commonCmdData, giterminismMgr, common.GetWerfConfigOptions(&commonCmdData, true))
		if err != nil {
			return "", "", fmt.Errorf("unable to load werf config: %w", err)
		}
		logboek.LogOptionalLn()

		namespace, err = deploy_params.GetKubernetesNamespace(*commonCmdData.Namespace, *commonCmdData.Environment, werfConfig)
		if err != nil {
			return "", "", err
		}

		release, err = deploy_params.GetHelmRelease(*commonCmdData.Release, *commonCmdData.Environment, namespace, werfConfig)
		if err != nil {
			return "", "", err
		}
	} else if !gitFound {
		if !namespaceSpecified && !releaseSpecified {
			return "", "", fmt.Errorf("no git with werf project found: dismiss should either be executed in a git repository, or with --namespace and --release specified, or with --use-deploy-report")
		} else if namespaceSpecified && !releaseSpecified {
			return "", "", fmt.Errorf("--namespace specified, but not --release, while should be specified both or none")
		} else if !namespaceSpecified && releaseSpecified {
			return "", "", fmt.Errorf("--release specified, but not --namespace, while should be specified both or none")
		}

		namespace = *commonCmdData.Namespace
		release = *commonCmdData.Release
	}

	return namespace, release, nil
}
