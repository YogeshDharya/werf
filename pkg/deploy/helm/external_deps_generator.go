package helm

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/phases/stages"
	"helm.sh/helm/v3/pkg/phases/stages/externaldeps"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
)

func NewStagesExternalDepsGenerator(restClient *action.RESTClientGetter) *StagesExternalDepsGenerator {
	return &StagesExternalDepsGenerator{
		restClient:   restClient,
		metaAccessor: metadataAccessor,
		scheme:       scheme.Scheme,
	}
}

type StagesExternalDepsGenerator struct {
	restClient   *action.RESTClientGetter
	gvkBuilder   externaldeps.GVKBuilder
	metaAccessor meta.MetadataAccessor
	scheme       *runtime.Scheme
	initialized  bool
}

func (s *StagesExternalDepsGenerator) init() error {
	if s.initialized {
		return nil
	}

	mapper, err := (*s.restClient).ToRESTMapper()
	if err != nil {
		return fmt.Errorf("error getting REST mapper: %w", err)
	}

	discoveryClient, err := (*s.restClient).ToDiscoveryClient()
	if err != nil {
		return fmt.Errorf("error getting discovery client: %w", err)
	}

	s.gvkBuilder = NewGVKBuilder(scheme.Scheme, restmapper.NewShortcutExpander(mapper, discoveryClient))

	s.initialized = true

	return nil
}

func (s *StagesExternalDepsGenerator) Generate(stages stages.SortedStageList) error {
	if err := s.init(); err != nil {
		return fmt.Errorf("error initializing external dependencies generator: %w", err)
	}

	for _, stage := range stages {
		if err := stage.DesiredResources.Visit(func(resInfo *resource.Info, err error) error {
			if err != nil {
				return err
			}

			annotations, err := s.metaAccessor.Annotations(resInfo.Object)
			if err != nil {
				return fmt.Errorf("error getting annotations for object: %w", err)
			}

			resExtDeps, err := s.resourceExternalDepsFromAnnotations(annotations)
			if err != nil {
				return fmt.Errorf("error generating external dependencies from resource annotations: %w", err)
			}

			stage.ExternalDependencies = append(stage.ExternalDependencies, resExtDeps...)

			return nil
		}); err != nil {
			return fmt.Errorf("error visiting resources list: %w", err)
		}
	}

	return nil
}

func (s *StagesExternalDepsGenerator) resourceExternalDepsFromAnnotations(annotations map[string]string) (externaldeps.ExternalDependencyList, error) {
	extDepsList, err := NewExternalDepsAnnotationsParser().Parse(annotations)
	if err != nil {
		return nil, fmt.Errorf("error parsing external dependencies annotations: %w", err)
	}

	if len(extDepsList) == 0 {
		return nil, nil
	}

	for _, extDep := range extDepsList {
		if err := extDep.GenerateInfo(s.gvkBuilder, s.scheme, s.metaAccessor); err != nil {
			return nil, fmt.Errorf("error generating Info for external dependency: %w", err)
		}
	}

	return extDepsList, nil
}
