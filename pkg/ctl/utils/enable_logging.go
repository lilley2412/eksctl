package utils

import (
	"fmt"
	"strings"

	"github.com/kris-nova/logger"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/sets"

	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha5"
	"github.com/weaveworks/eksctl/pkg/ctl/cmdutils"
	"github.com/weaveworks/eksctl/pkg/eks"
	"github.com/weaveworks/eksctl/pkg/printers"
)

func enableLoggingCmd(rc *cmdutils.ResourceCmd) {
	cfg := api.NewClusterConfig()
	rc.ClusterConfig = cfg

	rc.SetDescription("update-cluster-logging", "Update cluster logging configuration", "")

	var typesEnabled *[]string
	rc.SetRunFuncWithNameArg(func() error {
		return doEnableLogging(rc, typesEnabled)
	})

	rc.FlagSetGroup.InFlagSet("General", func(fs *pflag.FlagSet) {
		cmdutils.AddNameFlag(fs, cfg.Metadata)
		cmdutils.AddRegionFlag(fs, rc.ProviderConfig)
		cmdutils.AddConfigFileFlag(fs, &rc.ClusterConfigFile)
		cmdutils.AddApproveFlag(fs, rc)
	})

	rc.FlagSetGroup.InFlagSet("Enable/disable log types", func(fs *pflag.FlagSet) {
		allSupportedTypes := api.SupportedCloudWatchClusterLogTypes()

		typesEnabled = fs.StringArray("enable-types", []string{}, fmt.Sprintf("Log types to be enabled, the rest will be disabled. Supported log types: (all,%s)", strings.Join(allSupportedTypes, ", ")))

	})

	cmdutils.AddCommonFlagsForAWS(rc.FlagSetGroup, rc.ProviderConfig, false)
}

func doEnableLogging(rc *cmdutils.ResourceCmd, logTypesToEnable *[]string) error {
	enableTypes, err := processTypesToEnable(logTypesToEnable)
	if err != nil {
		return err
	}
	rc.ClusterConfig.AppendClusterCloudWatchLogTypes(enableTypes...)

	if err := cmdutils.NewUtilsEnableLoggingLoader(rc).Load(); err != nil {
		return err
	}

	cfg := rc.ClusterConfig
	meta := rc.ClusterConfig.Metadata

	api.SetClusterConfigDefaults(cfg)

	printer := printers.NewJSONPrinter()
	ctl := eks.New(rc.ProviderConfig, cfg)

	if !ctl.IsSupportedRegion() {
		return cmdutils.ErrUnsupportedRegion(rc.ProviderConfig)
	}
	logger.Info("using region %s", meta.Region)

	if err := ctl.CheckAuth(); err != nil {
		return err
	}

	currentlyEnabled, _, err := ctl.GetCurrentClusterConfigForLogging(meta)
	if err != nil {
		return err
	}

	shouldEnable := sets.NewString()

	if cfg.HasClusterCloudWatchLogging() {
		shouldEnable.Insert(cfg.CloudWatch.ClusterLogging.EnableTypes...)
	}

	shouldDisable := sets.NewString(api.SupportedCloudWatchClusterLogTypes()...).Difference(shouldEnable)

	updateRequired := !currentlyEnabled.Equal(shouldEnable)

	if err := printer.LogObj(logger.Debug, "cfg.json = \\\n%s\n", cfg); err != nil {
		return err
	}

	if updateRequired {
		describeTypesToEnable := "no types to enable"
		if len(shouldEnable.List()) > 0 {
			describeTypesToEnable = fmt.Sprintf("enable types: %s", strings.Join(shouldEnable.List(), ", "))
		}

		describeTypesToDisable := "no types to disable"
		if len(shouldDisable.List()) > 0 {
			describeTypesToDisable = fmt.Sprintf("disable types: %s", strings.Join(shouldDisable.List(), ", "))
		}

		cmdutils.LogIntendedAction(rc.Plan, "update CloudWatch logging for cluster %q in %q (%s & %s)",
			meta.Name, meta.Region, describeTypesToEnable, describeTypesToDisable,
		)
		if !rc.Plan {
			if err := ctl.UpdateClusterConfigForLogging(cfg); err != nil {
				return err
			}
		}
	} else {
		logger.Success("CloudWatch logging for cluster %q in %q is already up-to-date", meta.Name, meta.Region)
	}

	cmdutils.LogPlanModeWarning(rc.Plan && updateRequired)

	return nil
}

func processTypesToEnable(typesEnabled *[]string) ([]string, error) {
	if typesEnabled == nil || len(*typesEnabled) == 0 {
		return []string{}, nil
	}
	if len(*typesEnabled) == 1 && (*typesEnabled)[0] == "all" {
		return api.SupportedCloudWatchClusterLogTypes(), nil
	}

	allSupportedTypesSet := sets.NewString(api.SupportedCloudWatchClusterLogTypes()...)
	typesToEnable := sets.NewString()
	for _, logType := range *typesEnabled {

		if !allSupportedTypesSet.Has(logType) {
			return nil, fmt.Errorf("unknown log type %s. Supported log types: %s", logType, strings.Join(api.SupportedCloudWatchClusterLogTypes(), ", "))
		}

		typesToEnable.Insert(logType)
	}
	return typesToEnable.List(), nil
}
