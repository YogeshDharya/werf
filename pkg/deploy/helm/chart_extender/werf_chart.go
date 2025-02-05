package chart_extender

import (
	"context"

	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/werf/file"
)

var _ chart.ChartExtender = (*WerfChart)(nil)

type WerfChartOptions struct {
	SecretValueFiles           []string
	BuildChartDependenciesOpts chart.BuildChartDependenciesOptions
	DisableDefaultValues       bool
}

func NewWerfChart(
	ctx context.Context,
	chartFileReader file.ChartFileReader,
	chartDir string,
	projectDir string,
	opts WerfChartOptions,
) *WerfChart {
	wc := &WerfChart{
		ChartDir:         chartDir,
		ProjectDir:       projectDir,
		SecretValueFiles: opts.SecretValueFiles,

		ChartFileReader: chartFileReader,

		DisableDefaultValues:       opts.DisableDefaultValues,
		BuildChartDependenciesOpts: opts.BuildChartDependenciesOpts,
	}

	return wc
}

type WerfChart struct {
	ChartDir                   string
	ProjectDir                 string
	SecretValueFiles           []string
	BuildChartDependenciesOpts chart.BuildChartDependenciesOptions
	DisableDefaultValues       bool

	ChartFileReader file.ChartFileReader
}

func (wc *WerfChart) Type() string {
	return "chart"
}

func (wc *WerfChart) GetChartFileReader() file.ChartFileReader {
	return wc.ChartFileReader
}

func (wc *WerfChart) GetDisableDefaultValues() bool {
	return wc.DisableDefaultValues
}

func (wc *WerfChart) GetSecretValueFiles() []string {
	return wc.SecretValueFiles
}

func (wc *WerfChart) GetProjectDir() string {
	return wc.ProjectDir
}

func (wc *WerfChart) GetChartDir() string {
	return wc.ChartDir
}

func (wc *WerfChart) SetChartDir(dir string) {
	wc.ChartDir = dir
}

func (wc *WerfChart) GetBuildChartDependenciesOpts() chart.BuildChartDependenciesOptions {
	return wc.BuildChartDependenciesOpts
}
