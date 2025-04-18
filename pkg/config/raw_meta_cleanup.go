package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const DefaultKeepImagesBuiltWithinLastNHours uint64 = 2

type rawMetaCleanup struct {
	DisableCleanup                     bool                        `yaml:"disable,omitempty"`
	DisableKubernetesBasedPolicy       bool                        `yaml:"disableKubernetesBasedPolicy,omitempty"`
	DisableGitHistoryBasedPolicy       bool                        `yaml:"disableGitHistoryBasedPolicy,omitempty"`
	DisableBuiltWithinLastNHoursPolicy bool                        `yaml:"disableBuiltWithinLastNHoursPolicy,omitempty"`
	KeepPolicies                       []*rawMetaCleanupKeepPolicy `yaml:"keepPolicies,omitempty"`
	KeepImagesBuiltWithinLastNHours    *uint64                     `yaml:"keepImagesBuiltWithinLastNHours,omitempty"`

	rawMeta               *rawMeta
	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

type rawMetaCleanupKeepPolicy struct {
	References         *rawMetaCleanupKeepPolicyReferences         `yaml:"references,omitempty"`
	ImagesPerReference *rawMetaCleanupKeepPolicyImagesPerReference `yaml:"imagesPerReference,omitempty"`

	rawMetaCleanup        *rawMetaCleanup
	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

type rawMetaCleanupKeepPolicyReferences struct {
	Tag    string                                   `yaml:"tag,omitempty"`
	Branch string                                   `yaml:"branch,omitempty"`
	Limit  *rawMetaCleanupKeepPolicyReferencesLimit `yaml:"limit,omitempty"`

	TagRegexp    *regexp.Regexp `yaml:"-"`
	BranchRegexp *regexp.Regexp `yaml:"-"`

	rawMetaCleanup        *rawMetaCleanup
	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

type rawMetaCleanupKeepPolicyImagesPerReference rawMetaCleanupKeepPolicyReferencesLimit

type rawMetaCleanupKeepPolicyReferencesLimit struct {
	Last     *int           `yaml:"last,omitempty"`
	In       *time.Duration `yaml:"in,omitempty"`
	Operator *Operator      `yaml:"operator,omitempty"`

	rawMetaCleanup        *rawMetaCleanup
	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

func (c *rawMetaCleanup) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if parent, ok := parentStack.Peek().(*rawMeta); ok {
		c.rawMeta = parent
	}

	parentStack.Push(c)
	type plain rawMetaCleanup
	err := unmarshal((*plain)(c))
	parentStack.Pop()
	if err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, c, c.rawMeta.doc); err != nil {
		return err
	}

	return nil
}

func (c *rawMetaCleanupKeepPolicy) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if parent, ok := parentStack.Peek().(*rawMetaCleanup); ok {
		c.rawMetaCleanup = parent
	}

	parentStack.Push(c)
	type plain rawMetaCleanupKeepPolicy
	err := unmarshal((*plain)(c))
	parentStack.Pop()
	if err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, c, c.rawMetaCleanup.rawMeta.doc); err != nil {
		return err
	}

	if c.References == nil {
		return newDetailedConfigError("cleanup keep policy must have references section!", c, c.rawMetaCleanup.rawMeta.doc)
	}

	return nil
}

func (c *rawMetaCleanupKeepPolicyReferences) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if parent, ok := parentStack.Peek().(*rawMetaCleanupKeepPolicy); ok {
		c.rawMetaCleanup = parent.rawMetaCleanup
	}

	parentStack.Push(c)
	type plain rawMetaCleanupKeepPolicyReferences
	err := unmarshal((*plain)(c))
	parentStack.Pop()
	if err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, c, c.rawMetaCleanup.rawMeta.doc); err != nil {
		return err
	}

	if c.Tag == "" && c.Branch == "" {
		return newDetailedConfigError("tag `tag: string|REGEX` or branch `branch: string|REGEX` required for cleanup keep policy!", c, c.rawMetaCleanup.rawMeta.doc)
	} else if c.Tag != "" && c.Branch != "" {
		return newDetailedConfigError("specify only tag `tag: string|REGEX` or branch `branch: string|REGEX` for cleanup keep policy!", c, c.rawMetaCleanup.rawMeta.doc)
	}

	if c.Branch != "" {
		regex, err := c.processRegexpString("branch", c.Branch)
		if err != nil {
			return err
		}

		c.BranchRegexp = regex
	} else {
		regex, err := c.processRegexpString("tag", c.Tag)
		if err != nil {
			return err
		}

		c.TagRegexp = regex
	}

	return nil
}

func (c *rawMetaCleanupKeepPolicyReferencesLimit) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if parent, ok := parentStack.Peek().(*rawMetaCleanupKeepPolicyReferences); ok {
		c.rawMetaCleanup = parent.rawMetaCleanup
	}

	parentStack.Push(c)
	type plain rawMetaCleanupKeepPolicyReferencesLimit
	err := unmarshal((*plain)(c))
	parentStack.Pop()
	if err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, c, c.rawMetaCleanup.rawMeta.doc); err != nil {
		return err
	}

	if c.Operator != nil {
		if *c.Operator != "Or" && *c.Operator != "And" {
			return newDetailedConfigError(fmt.Sprintf("unsupported value %q for `operator: Or|And`!", *c.Operator), c, c.rawMetaCleanup.rawMeta.doc)
		}
	}

	return nil
}

func (c *rawMetaCleanupKeepPolicyReferences) processRegexpString(name, configValue string) (*regexp.Regexp, error) {
	var value string
	if strings.HasPrefix(configValue, "/") && strings.HasSuffix(configValue, "/") {
		value = strings.TrimPrefix(configValue, "/")
		value = strings.TrimSuffix(value, "/")
	} else {
		value = regexp.QuoteMeta(configValue)
	}

	expr := fmt.Sprintf("^%s$", value)
	regex, err := regexp.Compile(expr)
	if err != nil {
		return nil, newDetailedConfigError(fmt.Sprintf("invalid value %q for %q: %s", configValue, name, err.Error()), c, c.rawMetaCleanup.rawMeta.doc)
	}

	return regex, nil
}

func (c *rawMetaCleanup) toMetaCleanup() MetaCleanup {
	metaCleanup := MetaCleanup{}

	metaCleanup.DisableKubernetesBasedPolicy = c.DisableKubernetesBasedPolicy
	metaCleanup.DisableBuiltWithinLastNHoursPolicy = c.DisableBuiltWithinLastNHoursPolicy
	metaCleanup.DisableGitHistoryBasedPolicy = c.DisableGitHistoryBasedPolicy
	metaCleanup.DisableCleanup = c.DisableCleanup

	for _, policy := range c.KeepPolicies {
		metaCleanup.KeepPolicies = append(metaCleanup.KeepPolicies, policy.toMetaCleanupKeepPolicy())
	}

	if c.KeepImagesBuiltWithinLastNHours != nil {
		metaCleanup.KeepImagesBuiltWithinLastNHours = *c.KeepImagesBuiltWithinLastNHours
	} else {
		metaCleanup.KeepImagesBuiltWithinLastNHours = DefaultKeepImagesBuiltWithinLastNHours
	}

	return metaCleanup
}

func (c *rawMetaCleanupKeepPolicy) toMetaCleanupKeepPolicy() *MetaCleanupKeepPolicy {
	policy := &MetaCleanupKeepPolicy{}

	if c.References != nil {
		policy.References = c.References.toMetaCleanupKeepPolicyReferences()
	}

	if c.ImagesPerReference != nil {
		policy.ImagesPerReference = NewMetaCleanupKeepPolicyImagesPerReference(
			c.ImagesPerReference.Last,
			c.ImagesPerReference.In,
			c.ImagesPerReference.Operator,
		)
	}

	return policy
}

func (c *rawMetaCleanupKeepPolicyReferences) toMetaCleanupKeepPolicyReferences() MetaCleanupKeepPolicyReferences {
	references := MetaCleanupKeepPolicyReferences{}
	references.BranchRegexp = c.BranchRegexp
	references.TagRegexp = c.TagRegexp

	if c.Limit != nil {
		references.Limit = NewMetaCleanupKeepPolicyReferencesLimit(
			c.Limit.Last,
			c.Limit.In,
			c.Limit.Operator,
		)
	}

	return references
}
