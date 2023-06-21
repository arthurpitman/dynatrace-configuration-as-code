/*
 * @license
 * Copyright 2023 Dynatrace LLC
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/internal/featureflags"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/coordinate"
	configError "github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/errors"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/parameter"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/parameter/value"
	"github.com/dynatrace/dynatrace-configuration-as-code/v2/pkg/config/template"
	"github.com/spf13/afero"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	"path/filepath"
	"reflect"
)

type WriterContext struct {
	Fs              afero.Fs
	OutputFolder    string
	ProjectFolder   string
	ParametersSerde map[string]parameter.ParameterSerDe
}

type serializerContext struct {
	*WriterContext
	configFolder string
	config       coordinate.Coordinate
}

type environmentDetails struct {
	group       string
	environment string
}

type detailedSerializerContext struct {
	*serializerContext
	environmentDetails environmentDetails
}

type apiCoordinate struct {
	project string
	api     string
}

type configTemplate struct {
	// absolute path from the monaco project root to the template
	templatePath string

	// content of the template
	content string
}

func WriteConfigs(context *WriterContext, configs []Config) []error {
	definitions, templates, errs := toTopLevelDefinitions(context, configs)

	if len(errs) > 0 {
		return errs
	}

	var writeErrors []error

	for apiCoord, definition := range definitions {
		err := writeTopLevelDefinitionToDisk(context, apiCoord, definition)

		if err != nil {
			writeErrors = append(writeErrors, err)
		}
	}

	writeErrors = append(writeErrors, writeTemplates(context, templates)...)

	if len(writeErrors) > 0 {
		return writeErrors
	}

	return nil
}

func writeTemplates(context *WriterContext, templates []configTemplate) (errors []error) {
	for _, t := range templates {
		fullTemplatePath := filepath.Join(context.OutputFolder, t.templatePath)
		templateDir := filepath.Dir(fullTemplatePath)

		err := context.Fs.MkdirAll(templateDir, 0777)

		if err != nil {
			errors = append(errors, newConfigWriterError(context, err))
			continue
		}

		err = afero.WriteFile(context.Fs, fullTemplatePath, []byte(t.content), 0664)

		if err != nil {
			errors = append(errors, newConfigWriterError(context, err))
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

func toTopLevelDefinitions(context *WriterContext, configs []Config) (map[apiCoordinate]topLevelDefinition, []configTemplate, []error) {
	configsPerCoordinate := groupConfigs(configs)

	var errs []error
	result := map[apiCoordinate]topLevelDefinition{}

	configsPerApi := map[apiCoordinate][]topLevelConfigDefinition{}
	knownTemplates := map[string]struct{}{}
	var configTemplates []configTemplate

	for coord, confs := range configsPerCoordinate {
		sanitizedType := sanitize(coord.Type)
		configContext := &serializerContext{
			WriterContext: context,
			configFolder:  filepath.Join(context.ProjectFolder, sanitizedType),
			config:        coord,
		}

		definition, templates, convertErrs := toTopLevelConfigDefinition(configContext, confs)

		if len(convertErrs) > 0 {
			errs = append(errs, convertErrs...)
			continue
		}

		apiCoord := apiCoordinate{
			project: coord.Project,
			api:     coord.Type,
		}

		configsPerApi[apiCoord] = append(configsPerApi[apiCoord], definition)

		for _, t := range templates {
			if _, found := knownTemplates[t.templatePath]; found {
				continue
			}

			configTemplates = append(configTemplates, t)
			knownTemplates[t.templatePath] = struct{}{}
		}
	}

	if len(errs) > 0 {
		return nil, nil, errs
	}

	for apiCoord, confs := range configsPerApi {
		result[apiCoord] = topLevelDefinition{
			Configs: confs,
		}
	}

	return result, configTemplates, nil
}

func byConfigId(a, b topLevelConfigDefinition) bool {
	return a.Id < b.Id
}

func writeTopLevelDefinitionToDisk(context *WriterContext, apiCoord apiCoordinate, definition topLevelDefinition) error {
	// sort configs so that they are stable within a config file
	slices.SortFunc(definition.Configs, byConfigId)
	definitionYaml, err := yaml.Marshal(definition)

	if err != nil {
		return newConfigWriterError(context, err)
	}

	sanitizedApi := sanitize(apiCoord.api)
	targetConfigFile := filepath.Join(context.OutputFolder, context.ProjectFolder, sanitizedApi, "config.yaml")

	err = context.Fs.MkdirAll(filepath.Dir(targetConfigFile), 0777)

	if err != nil {
		return newConfigWriterError(context, err)
	}

	err = afero.WriteFile(context.Fs, targetConfigFile, definitionYaml, 0664)

	if err != nil {
		return newConfigWriterError(context, err)
	}

	return nil
}

func toTopLevelConfigDefinition(context *serializerContext, configs []Config) (topLevelConfigDefinition, []configTemplate, []error) {
	configDefinitions, templates, errs := toConfigDefinitions(context, configs)

	if len(errs) > 0 {
		return topLevelConfigDefinition{}, nil, errs
	}

	groupedDefinitionsByGroup := groupByGroups(configDefinitions)

	var groupOverrides []extendedConfigDefinition
	var environmentOverrides []extendedConfigDefinition

	for group, definitions := range groupedDefinitionsByGroup {
		base, reduced := extractCommonBase(definitions)

		if base != nil {
			groupOverrides = append(groupOverrides, extendedConfigDefinition{
				configDefinition: *base,
				group:            group,
			})
		}

		environmentOverrides = append(environmentOverrides, reduced...)
	}

	baseConfig, reducedGroupOverrides := extractCommonBase(groupOverrides)

	var config configDefinition
	var groupOverrideConfigs []groupOverride
	var environmentOverrideConfigs []environmentOverride

	if baseConfig != nil {
		config = *baseConfig
	}

	for _, conf := range reducedGroupOverrides {
		groupOverrideConfigs = append(groupOverrideConfigs, groupOverride{
			Group:    conf.group,
			Override: conf.configDefinition,
		})
	}

	for _, conf := range environmentOverrides {
		environmentOverrideConfigs = append(environmentOverrideConfigs, environmentOverride{
			Environment: conf.environment,
			Override:    conf.configDefinition,
		})
	}

	// We need to extract the configType from the original configs.
	// Since they all should have the same configType (they have all the same coordinate), we can take any one.
	ct, err := extractConfigType(context, configs[0])
	if err != nil {
		return topLevelConfigDefinition{}, nil, []error{fmtDetailedConfigWriterError(context, "failed to extract config type: %w", err)}
	}

	return topLevelConfigDefinition{
		Id:                   context.config.ConfigId,
		Config:               config,
		Type:                 ct,
		GroupOverrides:       groupOverrideConfigs,
		EnvironmentOverrides: environmentOverrideConfigs,
	}, templates, nil
}

func extractConfigType(context *serializerContext, config Config) (typeDefinition, error) {

	switch t := config.Type.(type) {
	case SettingsType:
		serializedScope, err := getScope(context, config)
		if err != nil {
			return typeDefinition{}, err
		}

		return typeDefinition{
			Settings: settingsDefinition{
				Schema:        t.SchemaId,
				SchemaVersion: t.SchemaVersion,
				Scope:         serializedScope,
			},
		}, nil

	case ClassicApiType:
		return typeDefinition{
			Api: config.Coordinate.Type,
		}, nil

	case EntityType:
		return typeDefinition{
			Entities: entitiesDefinition{
				EntitiesType: t.EntitiesType,
			},
		}, nil
	case AutomationType:
		if !featureflags.AutomationResources().Enabled() {
			return typeDefinition{}, fmtDetailedConfigWriterError(context, "automation resource feature is not enabled")
		}

		return typeDefinition{
			Automation: automationDefinition{
				Resource: t.Resource,
			},
		}, nil

	default:
		return typeDefinition{}, fmtDetailedConfigWriterError(context, "unknown config-type (ID: %q)", config.Type.ID())
	}
}

func getScope(context *serializerContext, config Config) (configParameter, error) {
	scopeParam, found := config.Parameters[ScopeParameter]
	if !found {
		return nil, fmtDetailedConfigWriterError(context, "scope parameter not found. This is likely a bug")
	}

	serializedScope, err := toParameterDefinition(&detailedSerializerContext{
		serializerContext: context,
	}, ScopeParameter, scopeParam)
	if err != nil {
		return nil, fmtDetailedConfigWriterError(context, "failed to serialize scope-parameter: %w", err)
	}
	return serializedScope, nil
}

func groupByGroups(configs []extendedConfigDefinition) map[string][]extendedConfigDefinition {

	result := make(map[string][]extendedConfigDefinition)

	for _, c := range configs {
		result[c.group] = append(result[c.group], c)
	}

	return result
}

func extractCommonBase(configs []extendedConfigDefinition) (*configDefinition, []extendedConfigDefinition) {
	switch len(configs) {
	case 0:
		return nil, nil
	case 1:
		return &configs[0].configDefinition, nil
	}

	checkResult := testForSameProperties(configs)
	sharedParam := extractSharedParameters(configs)

	// TODO refactor this monstrosity
	if len(sharedParam) == 0 && (!checkResult.foundName || !checkResult.shareName) &&
		(!checkResult.foundTemplate || !checkResult.shareTemplate) &&
		(!checkResult.foundSkip || !checkResult.shareSkip) {
		return nil, configs
	}

	configDefinitionResult := createCommonConfigDefinition(checkResult, sharedParam)
	var definitions []extendedConfigDefinition

	for _, conf := range configs {
		reducedConf := createConfigDefinitionWithoutSharedValues(conf, checkResult, sharedParam)

		if reducedConf != nil {
			definitions = append(definitions, extendedConfigDefinition{
				configDefinition: *reducedConf,
				group:            conf.group,
				environment:      conf.environment,
			})
		}
	}

	return configDefinitionResult, definitions
}

func createConfigDefinitionWithoutSharedValues(toReduce extendedConfigDefinition, checkResult propertyCheckResult,
	sharedParameters map[string]configParameter) *configDefinition {
	allParametersShared := true
	reducedParameters := make(map[string]configParameter)

	for k, v := range toReduce.Parameters {
		if _, found := sharedParameters[k]; !found {
			allParametersShared = false
			reducedParameters[k] = v
		}
	}

	if allParametersShared && checkResult.shareName &&
		checkResult.shareSkip && checkResult.shareTemplate {
		return nil
	}

	result := &configDefinition{
		Parameters: reducedParameters,
	}

	if !checkResult.shareName {
		result.Name = toReduce.Name
	}

	if !checkResult.shareTemplate {
		result.Template = toReduce.Template
	}

	if !checkResult.shareSkip {
		result.Skip = toReduce.Skip
	}

	return result
}

func createCommonConfigDefinition(checkResult propertyCheckResult, sharedParameters map[string]configParameter) *configDefinition {
	result := &configDefinition{}

	if checkResult.foundName || checkResult.shareName {
		result.Name = checkResult.name
	}

	if checkResult.foundTemplate || checkResult.shareTemplate {
		result.Template = checkResult.template
	}

	if checkResult.foundSkip || checkResult.shareSkip {
		result.Skip = checkResult.skip
	}

	if len(sharedParameters) > 0 {
		result.Parameters = sharedParameters
	}

	return result
}

func extractSharedParameters(configs []extendedConfigDefinition) map[string]configParameter {
	result := make(map[string]configParameter)
	startParams := configs[0].Parameters

	for name, val := range startParams {
		if isSharedParameter(configs[1:], name, val) {
			result[name] = val
		}
	}
	return result
}

func isSharedParameter(configs []extendedConfigDefinition, name string, val configParameter) bool {
	for _, conf := range configs {
		paramVal := conf.Parameters[name]

		if !reflect.DeepEqual(val, paramVal) {
			return false
		}
	}
	return true
}

type propertyCheckResult struct {
	shareName bool
	foundName bool
	name      configParameter

	shareTemplate bool
	foundTemplate bool
	template      string

	shareSkip bool
	foundSkip bool
	skip      interface{}
}

func testForSameProperties(configs []extendedConfigDefinition) propertyCheckResult {
	name := configs[0].Name
	templ := configs[0].Template
	skip := configs[0].Skip

	var (
		sameName,
		sameTemplate,
		sameSkip = true, true, true
	)

	for _, c := range configs {
		sameName = sameName && reflect.DeepEqual(name, c.Name)
		sameTemplate = sameTemplate && templ == c.Template
		sameSkip = sameSkip && (reflect.DeepEqual(skip, c.Skip) ||
			(skip == nil && c.Skip == false) ||
			(skip == false && c.Skip == nil))
	}

	if !sameName {
		name = nil
	}

	if !sameTemplate {
		templ = ""
	}

	if !sameSkip {
		skip = nil
	}

	return propertyCheckResult{
		shareName: sameName,
		foundName: name != nil || !sameName,
		name:      name,

		shareTemplate: sameTemplate,
		foundTemplate: templ != "" || !sameTemplate,
		template:      templ,

		shareSkip: sameSkip,
		foundSkip: skip != nil || !sameSkip,
		skip:      skip,
	}
}

type extendedConfigDefinition struct {
	configDefinition
	group       string
	environment string
}

func toConfigDefinitions(context *serializerContext, configs []Config) ([]extendedConfigDefinition, []configTemplate, []error) {
	var errs []error
	result := make([]extendedConfigDefinition, 0, len(configs))

	var templates []configTemplate

	for _, c := range configs {
		definition, templ, convertErrs := toConfigDefinition(context, c)

		if len(convertErrs) > 0 {
			errs = append(errs, convertErrs...)
			continue
		}

		templates = append(templates, templ)

		result = append(result, extendedConfigDefinition{
			configDefinition: definition,
			group:            c.Group,
			environment:      c.Environment,
		})
	}

	if len(errs) > 0 {
		return nil, nil, errs
	}

	return result, templates, nil
}

func toConfigDefinition(context *serializerContext, config Config) (configDefinition, configTemplate, []error) {
	var errs []error
	detailedContext := detailedSerializerContext{
		serializerContext: context,
		environmentDetails: environmentDetails{
			group:       config.Group,
			environment: config.Environment,
		},
	}
	nameParam, err := parseNameParameter(&detailedContext, config)
	if err != nil {
		errs = append(errs, err)
	}

	skipParam, err := parseSkipParameter(&detailedContext, config)
	if err != nil {
		errs = append(errs, err)
	}

	params, convertErrs := convertParameters(&detailedContext, config.Parameters)

	errs = append(errs, convertErrs...)

	configTemplatePath, templ, err := extractTemplate(&detailedContext, config)

	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return configDefinition{}, configTemplate{}, errs
	}

	return configDefinition{
		Name:           nameParam,
		Parameters:     params,
		Template:       filepath.ToSlash(configTemplatePath),
		Skip:           skipParam,
		OriginObjectId: config.OriginObjectId,
	}, templ, nil
}

func parseSkipParameter(d *detailedSerializerContext, config Config) (configParameter, error) {
	if config.SkipForConversion == nil {
		return config.Skip, nil
	}

	skipDefinition, err := toParameterDefinition(d, SkipParameter, config.SkipForConversion)
	if err != nil {
		return nil, fmtDetailedConfigWriterError(d.serializerContext, "failed to serialize skip parameter: %w", err)
	}
	return skipDefinition, nil
}

func extractTemplate(context *detailedSerializerContext, config Config) (string, configTemplate, error) {
	switch templ := config.Template.(type) {
	case template.FileBasedTemplate:
		path, err := filepath.Rel(context.configFolder, filepath.Clean(templ.FilePath()))

		if err != nil {
			return "", configTemplate{}, newDetailedConfigWriterError(context.serializerContext, err)
		}

		return path, configTemplate{
			templatePath: templ.FilePath(),
			content:      templ.Content(),
		}, nil
	case template.Template:
		sanitizedName := sanitize(templ.Id()) + ".json"

		return sanitizedName, configTemplate{
			templatePath: filepath.Join(context.configFolder, sanitizedName),
			content:      templ.Content(),
		}, nil
	}

	// this should never happen
	return "", configTemplate{}, fmtDetailedConfigWriterError(context.serializerContext, "unknown template type")
}

func convertParameters(context *detailedSerializerContext, parameters Parameters) (map[string]configParameter, []error) {
	var errs []error
	result := make(map[string]configParameter)

	for name, param := range parameters {
		// ignore NameParameter and ScopeParameter as it is handled in a special way
		if name == NameParameter || name == ScopeParameter {
			continue
		}

		parsed, err := toParameterDefinition(context, name, param)

		if err != nil {
			errs = append(errs, err)
			continue
		}

		result[name] = parsed
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return result, nil
}

func parseNameParameter(context *detailedSerializerContext, config Config) (configParameter, error) {
	nameParam, found := config.Parameters[NameParameter]

	if !found {
		return nil, fmtDetailedConfigWriterError(context.serializerContext, "%s: `name` parameter missing",
			config.Coordinate)
	}

	return toParameterDefinition(context, NameParameter, nameParam)
}

func toParameterDefinition(context *detailedSerializerContext, parameterName string,
	param parameter.Parameter) (configParameter, error) {

	if isValueParameter(param) {
		return toValueShorthandDefinition(context, parameterName, param)
	}

	serde, found := context.ParametersSerde[param.GetType()]

	if !found {
		return nil, fmtDetailedConfigWriterError(context.serializerContext, "%s:%s: no serde found for type `%s`",
			context.config, parameterName, param.GetType())
	}

	result, err := serde.Serializer(newParameterSerializerContext(context, parameterName, param))

	if err != nil {
		return nil, err
	}

	result["type"] = param.GetType()

	return result, nil
}

func isValueParameter(param parameter.Parameter) bool {
	return param.GetType() == value.ValueParameterType
}

func toValueShorthandDefinition(context *detailedSerializerContext, parameterName string,
	param parameter.Parameter) (configParameter, error) {
	if param.GetType() == value.ValueParameterType {
		valueParam, ok := param.(*value.ValueParameter)

		if !ok {
			return nil, fmtDetailedConfigWriterError(context.serializerContext, "%s:%s: parameter of type `%s` is no value param", context.config, parameterName, param.GetType())
		}

		switch valueParam.Value.(type) {
		// strings can be shorthanded
		case string:
			return valueParam.Value, nil
		default:
			result, err := context.ParametersSerde[param.GetType()].Serializer(newParameterSerializerContext(context, parameterName, param))

			if err != nil {
				return nil, err
			}

			result["type"] = valueParam.GetType()

			return result, nil
		}
	}

	return nil, fmtDetailedConfigWriterError(context.serializerContext, "%s:%s: unknown special type `%s`", context.config, parameterName, param.GetType())
}

func groupConfigs(configs []Config) map[coordinate.Coordinate][]Config {
	result := make(map[coordinate.Coordinate][]Config)

	for _, c := range configs {
		result[c.Coordinate] = append(result[c.Coordinate], c)
	}

	return result
}

func newParameterSerializerContext(context *detailedSerializerContext, name string,
	param parameter.Parameter) parameter.ParameterWriterContext {
	return parameter.ParameterWriterContext{
		Coordinate:    context.config,
		Group:         context.environmentDetails.group,
		Environment:   context.environmentDetails.environment,
		ParameterName: name,
		Parameter:     param,
	}
}

func newConfigWriterError(context *WriterContext, err error) configError.DetailedConfigWriterError {
	return configError.DetailedConfigWriterError{
		Path: filepath.Join(context.OutputFolder, context.ProjectFolder),
		Err:  err,
	}
}

func newDetailedConfigWriterError(context *serializerContext, err error) configError.DetailedConfigWriterError {
	return configError.DetailedConfigWriterError{
		Path:     context.configFolder,
		Location: context.config,
		Err:      err,
	}
}

func fmtDetailedConfigWriterError(context *serializerContext, format string, args ...interface{}) configError.DetailedConfigWriterError {
	return configError.DetailedConfigWriterError{
		Path:     context.configFolder,
		Location: context.config,
		Err:      fmt.Errorf(format, args...),
	}
}