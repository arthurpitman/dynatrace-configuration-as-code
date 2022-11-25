//go:build unit

// @license
// Copyright 2021 Dynatrace LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deploy

import (
	"errors"
	"fmt"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/rest"
	"github.com/golang/mock/gomock"
	"testing"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	config "github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/coordinate"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/parameter"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/config/v2/template"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/project/v2/topologysort"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/client"
	"github.com/google/uuid"
	"gotest.tools/assert"
)

var dashboardApi = api.NewStandardApi("dashboard", "dashboard", false, "dashboard-v2", false)

func TestResolveParameterValues(t *testing.T) {
	name := "test"
	owner := "hansi"
	ownerParameterName := "owner"
	timeout := 5
	timeoutParameterName := "timeout"
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: name,
			},
		},
		{
			Name: ownerParameterName,
			Parameter: &parameter.DummyParameter{
				Value: owner,
			},
		},
		{
			Name: timeoutParameterName,
			Parameter: &parameter.DummyParameter{
				Value: timeout,
			},
		},
	}

	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	values, errors := ResolveParameterValues(&conf, entities, parameters)

	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
	assert.Equal(t, name, values[config.NameParameter])
	assert.Equal(t, owner, values[ownerParameterName])
	assert.Equal(t, timeout, values[timeoutParameterName])
}

func TestResolveParameterValuesShouldFailWhenReferencingNonExistingConfig(t *testing.T) {
	nonExistingConfig := coordinate.Coordinate{
		Project:  "non-existing",
		Type:     "management-zone",
		ConfigId: "zone1",
	}
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   nonExistingConfig,
						Property: "name",
					},
				},
			},
		},
	}

	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	_, errors := ResolveParameterValues(&conf, entities, parameters)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestResolveParameterValuesShouldFailWhenReferencingSkippedConfig(t *testing.T) {
	referenceCoordinate := coordinate.Coordinate{
		Project:  "project1",
		Type:     "management-zone",
		ConfigId: "zone1",
	}

	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   referenceCoordinate,
						Property: "name",
					},
				},
			},
		},
	}

	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{
		referenceCoordinate: {
			EntityName: "zone1",
			Coordinate: referenceCoordinate,
			Properties: parameter.Properties{},
			Skip:       true,
		},
	}

	_, errors := ResolveParameterValues(&conf, entities, parameters)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestResolveParameterValuesShouldFailWhenParameterResolveReturnsError(t *testing.T) {
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Err: errors.New("error"),
			},
		},
	}

	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	_, errors := ResolveParameterValues(&conf, entities, parameters)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestValidateParameterReferences(t *testing.T) {
	configCoordinates := coordinate.Coordinate{
		Project:  "project1",
		Type:     "dashboard",
		ConfigId: "dashboard-1",
	}

	referencedConfigCoordinates := coordinate.Coordinate{
		Project:  "project2",
		Type:     "management-zone",
		ConfigId: "zone1",
	}

	param := &parameter.DummyParameter{
		Value: "test",
		References: []parameter.ParameterReference{
			{
				Config:   configCoordinates,
				Property: "name",
			},
			{
				Config:   referencedConfigCoordinates,
				Property: "name",
			},
		},
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{
		referencedConfigCoordinates: {
			EntityName: "zone1",
			Coordinate: referencedConfigCoordinates,
			Properties: parameter.Properties{
				"name": "test",
			},
			Skip: false,
		},
	}

	errors := validateParameterReferences(configCoordinates, "", "", entities, "managementZoneName", param)

	assert.Assert(t, len(errors) == 0, "should not return errors (no errors: %d)", len(errors))
}

func TestValidateParameterReferencesShouldFailWhenReferencingSelf(t *testing.T) {
	paramName := "name"

	configCoordinates := coordinate.Coordinate{
		Project:  "project1",
		Type:     "dashboard",
		ConfigId: "dashboard-1",
	}

	param := &parameter.DummyParameter{
		Value: "test",
		References: []parameter.ParameterReference{
			{
				Config:   configCoordinates,
				Property: paramName,
			},
		},
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	errors := validateParameterReferences(configCoordinates, "", "", entities, paramName, param)

	assert.Assert(t, len(errors) > 0, "should not errors (no errors: %d)", len(errors))
}

func TestValidateParameterReferencesShouldFailWhenReferencingSkippedConfig(t *testing.T) {
	configCoordinates := coordinate.Coordinate{
		Project:  "project1",
		Type:     "dashboard",
		ConfigId: "dashboard-1",
	}

	referencedConfigCoordinates := coordinate.Coordinate{
		Project:  "project2",
		Type:     "management-zone",
		ConfigId: "zone1",
	}

	param := &parameter.DummyParameter{
		Value: "test",
		References: []parameter.ParameterReference{
			{
				Config:   referencedConfigCoordinates,
				Property: "name",
			},
		},
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{
		referencedConfigCoordinates: {
			EntityName: "zone1",
			Coordinate: referencedConfigCoordinates,
			Properties: parameter.Properties{},
			Skip:       true,
		},
	}

	errors := validateParameterReferences(configCoordinates, "", "", entities, "managementZoneName", param)

	assert.Assert(t, len(errors) > 0, "should return errors (no errors: %d)", len(errors))
}

func TestValidateParameterReferencesShouldFailWhenReferencingUnknownConfig(t *testing.T) {
	configCoordinates := coordinate.Coordinate{
		Project:  "project1",
		Type:     "dashboard",
		ConfigId: "dashboard-1",
	}

	referencedConfigCoordinates := coordinate.Coordinate{
		Project:  "project2",
		Type:     "management-zone",
		ConfigId: "zone1",
	}

	param := &parameter.DummyParameter{
		Value: "test",
		References: []parameter.ParameterReference{
			{
				Config:   referencedConfigCoordinates,
				Property: "name",
			},
		},
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	errors := validateParameterReferences(configCoordinates, "", "", entities, "managementZoneName", param)

	assert.Assert(t, len(errors) > 0, "should return errors (no errors: %d)", len(errors))
}

func TestExtractConfigName(t *testing.T) {
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  map[string]parameter.Parameter{},
		References:  []coordinate.Coordinate{},
		Skip:        false,
	}

	name := "test"

	properties := parameter.Properties{
		config.NameParameter: name,
	}

	val, err := ExtractConfigName(&conf, properties)

	assert.NilError(t, err)
	assert.Equal(t, name, val)
}

func TestExtractConfigNameShouldFailOnMissingName(t *testing.T) {
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  map[string]parameter.Parameter{},
		References:  []coordinate.Coordinate{},
		Skip:        false,
	}

	properties := parameter.Properties{}

	_, err := ExtractConfigName(&conf, properties)

	assert.Assert(t, err != nil, "error should not be nil (error val: %s)", err)
}

func TestExtractConfigNameShouldFailOnNameWithNonStringType(t *testing.T) {
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  map[string]parameter.Parameter{},
		References:  []coordinate.Coordinate{},
		Skip:        false,
	}

	properties := parameter.Properties{
		config.NameParameter: 1,
	}

	_, err := ExtractConfigName(&conf, properties)

	assert.Assert(t, err != nil, "error should not be nil (error val: %s)", err)
}

func TestDeployConfig(t *testing.T) {
	name := "test"
	owner := "hansi"
	ownerParameterName := "owner"
	timeout := 5
	timeoutParameterName := "timeout"
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: name,
			},
		},
		{
			Name: ownerParameterName,
			Parameter: &parameter.DummyParameter{
				Value: owner,
			},
		},
		{
			Name: timeoutParameterName,
			Parameter: &parameter.DummyParameter{
				Value: timeout,
			},
		},
	}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	knownEntityNames := knownEntityMap{}

	resolvedEntity, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) == 0, "there should be no errors (no errors: %d, %s)", len(errors), errors)
	assert.Equal(t, name, resolvedEntity.EntityName, "%s == %s")
	assert.Equal(t, conf.Coordinate, resolvedEntity.Coordinate)
	assert.Equal(t, name, resolvedEntity.Properties[config.NameParameter])
	assert.Equal(t, owner, resolvedEntity.Properties[ownerParameterName])
	assert.Equal(t, timeout, resolvedEntity.Properties[timeoutParameterName])
	assert.Equal(t, false, resolvedEntity.Skip)
}

func TestDeploySettingShouldFailCyclicParameterDependencies(t *testing.T) {
	ownerParameterName := "owner"
	configCoordinates := coordinate.Coordinate{}

	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   configCoordinates,
						Property: ownerParameterName,
					},
				},
			},
		},
		{
			Name: ownerParameterName,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   configCoordinates,
						Property: config.NameParameter,
					},
				},
			},
		},
	}

	client := &client.DummyClient{}
	entities := make(map[coordinate.Coordinate]parameter.ResolvedEntity)

	conf := &config.Config{
		Template:   generateDummyTemplate(t),
		Parameters: toParameterMap(parameters),
	}
	_, errors := deploySetting(client, entities, conf)
	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeploySettingShouldFailRenderTemplate(t *testing.T) {
	client := &client.DummyClient{}
	entities := make(map[coordinate.Coordinate]parameter.ResolvedEntity)

	conf := &config.Config{
		Template: generateFaultyTemplate(t),
	}

	_, errors := deploySetting(client, entities, conf)
	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeploySettingShouldFailUpsert(t *testing.T) {
	name := "test"
	owner := "hansi"
	ownerParameterName := "owner"
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: name,
			},
		},
		{
			Name: ownerParameterName,
			Parameter: &parameter.DummyParameter{
				Value: owner,
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client := rest.NewMockSettingsClient(mockCtrl)

	client.EXPECT().Upsert(gomock.Any()).Return(api.DynatraceEntity{}, fmt.Errorf("upsert failed"))

	entities := make(map[coordinate.Coordinate]parameter.ResolvedEntity)

	conf := &config.Config{
		Template:   generateDummyTemplate(t),
		Parameters: toParameterMap(parameters),
	}
	_, errors := deploySetting(client, entities, conf)
	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeploySetting(t *testing.T) {
	parameters := []topologysort.ParameterWithName{
		{
			Name: "franz",
			Parameter: &parameter.DummyParameter{
				Value: "foo",
			},
		},
		{
			Name: "hansi",
			Parameter: &parameter.DummyParameter{
				Value: "bar",
			},
		},
	}

	client := &client.DummyClient{}
	entities := make(map[coordinate.Coordinate]parameter.ResolvedEntity)

	conf := &config.Config{
		Template:   generateDummyTemplate(t),
		Parameters: toParameterMap(parameters),
	}
	_, errors := deploySetting(client, entities, conf)
	assert.Assert(t, len(errors) == 0, "there should be no errors (no errors: %d, %s)", len(errors), errors)
}

func TestDeployConfigShouldFailOnAnAlreadyKnownEntityName(t *testing.T) {
	name := "test"
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: name,
			},
		},
	}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	knownEntityNames := knownEntityMap{
		"dashboard": {
			name: struct{}{},
		},
	}

	_, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeployConfigShouldFailCyclicParameterDependencies(t *testing.T) {
	ownerParameterName := "owner"
	configCoordinates := coordinate.Coordinate{
		Project:  "project1",
		Type:     "dashboard",
		ConfigId: "dashboard-1",
	}

	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   configCoordinates,
						Property: ownerParameterName,
					},
				},
			},
		},
		{
			Name: ownerParameterName,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   configCoordinates,
						Property: config.NameParameter,
					},
				},
			},
		},
	}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	knownEntityNames := knownEntityMap{}

	_, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeployConfigShouldFailOnMissingNameParameter(t *testing.T) {
	parameters := []topologysort.ParameterWithName{}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}

	knownEntityNames := knownEntityMap{}

	_, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeployConfigShouldFailOnReferenceOnUnknownConfig(t *testing.T) {
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config: coordinate.Coordinate{
							Project:  "project2",
							Type:     "dashboard",
							ConfigId: "dashboard",
						},
						Property: "managementZoneId",
					},
				},
			},
		},
	}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{}
	knownEntityNames := knownEntityMap{}

	_, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeployConfigShouldFailOnReferenceOnSkipConfig(t *testing.T) {
	referenceCoordinates := coordinate.Coordinate{
		Project:  "project2",
		Type:     "dashboard",
		ConfigId: "dashboard",
	}

	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				References: []parameter.ParameterReference{
					{
						Config:   referenceCoordinates,
						Property: "managementZoneId",
					},
				},
			},
		},
	}

	client := &client.DummyClient{}
	conf := config.Config{
		Template: generateDummyTemplate(t),
		Coordinate: coordinate.Coordinate{
			Project:  "project1",
			Type:     "dashboard",
			ConfigId: "dashboard-1",
		},
		Environment: "development",
		Parameters:  toParameterMap(parameters),
		References:  toReferences(parameters),
		Skip:        false,
	}

	entities := map[coordinate.Coordinate]parameter.ResolvedEntity{
		referenceCoordinates: {
			EntityName: referenceCoordinates.ConfigId,
			Coordinate: referenceCoordinates,
			Properties: parameter.Properties{},
			Skip:       true,
		},
	}

	knownEntityNames := knownEntityMap{}

	_, errors := deployConfig(client, dashboardApi, entities, knownEntityNames, &conf)

	assert.Assert(t, len(errors) > 0, "there should be errors (no errors: %d)", len(errors))
}

func TestDeployConfigsWithNoConfigs(t *testing.T) {
	client := &client.DummyClient{}
	var apis map[string]api.Api
	var sortedConfigs []config.Config

	errors := DeployConfigs(client, apis, sortedConfigs, false, false)
	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
}

func TestDeployConfigsWithOneConfigToSkip(t *testing.T) {
	client := &client.DummyClient{}
	var apis map[string]api.Api
	sortedConfigs := []config.Config{
		{Skip: true},
	}
	errors := DeployConfigs(client, apis, sortedConfigs, false, false)
	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
}

func TestDeployConfigsTargetingSettings(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	client := rest.NewMockDynatraceClient(mockCtrl)
	var apis map[string]api.Api
	sortedConfigs := []config.Config{
		{
			Template: generateDummyTemplate(t),
			Type: config.Type{
				Schema:        "schema",
				SchemaVersion: "schemaversion",
				Scope:         "scope",
			},
		},
	}
	client.EXPECT().Upsert(gomock.Any()).Times(1)
	errors := DeployConfigs(client, apis, sortedConfigs, false, false)
	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
}

func TestDeployConfigsTargetingClassicConfigUnique(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	theConfigName := "theConfigName"
	theApiName := "theApiName"

	theApi := api.NewMockApi(gomock.NewController(t))
	theApi.EXPECT().GetId().AnyTimes().Return(theApiName)
	theApi.EXPECT().IsDeprecatedApi().Return(false)
	theApi.EXPECT().IsNonUniqueNameApi().Return(false)

	client := rest.NewMockDynatraceClient(mockCtrl)
	client.EXPECT().UpsertByName(gomock.Any(), theConfigName, gomock.Any()).Times(1)

	apis := map[string]api.Api{theApiName: theApi}
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: theConfigName,
			},
		},
	}
	sortedConfigs := []config.Config{
		{
			Parameters: toParameterMap(parameters),
			Coordinate: coordinate.Coordinate{Type: theApiName},
			Template:   generateDummyTemplate(t),
			Type: config.Type{
				Api: theApiName,
			},
		},
	}

	errors := DeployConfigs(client, apis, sortedConfigs, false, false)
	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
}

func TestDeployConfigsTargetingClassicConfigNonUnique(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	theConfigName := "theConfigName"
	theApiName := "theApiName"

	theApi := api.NewMockApi(gomock.NewController(t))
	theApi.EXPECT().GetId().AnyTimes().Return(theApiName)
	theApi.EXPECT().IsDeprecatedApi().Return(false)
	theApi.EXPECT().IsNonUniqueNameApi().Return(true)

	client := rest.NewMockDynatraceClient(mockCtrl)
	client.EXPECT().UpsertByEntityId(gomock.Any(), gomock.Any(), theConfigName, gomock.Any())

	apis := map[string]api.Api{theApiName: theApi}
	parameters := []topologysort.ParameterWithName{
		{
			Name: config.NameParameter,
			Parameter: &parameter.DummyParameter{
				Value: theConfigName,
			},
		},
	}
	sortedConfigs := []config.Config{
		{
			Parameters: toParameterMap(parameters),
			Coordinate: coordinate.Coordinate{Type: theApiName},
			Template:   generateDummyTemplate(t),
			Type: config.Type{
				Api: theApiName,
			},
		},
	}

	errors := DeployConfigs(client, apis, sortedConfigs, false, false)
	assert.Assert(t, len(errors) == 0, "there should be no errors (errors: %s)", errors)
}

func toParameterMap(params []topologysort.ParameterWithName) map[string]parameter.Parameter {
	result := make(map[string]parameter.Parameter)

	for _, p := range params {
		result[p.Name] = p.Parameter
	}

	return result
}

func toReferences(params []topologysort.ParameterWithName) []coordinate.Coordinate {
	var result []coordinate.Coordinate

	for _, p := range params {
		refs := p.Parameter.GetReferences()

		if refs == nil {
			continue
		}

		for _, ref := range refs {
			result = append(result, ref.Config)
		}
	}

	return result
}

func generateDummyTemplate(t *testing.T) template.Template {
	uuid, err := uuid.NewUUID()
	assert.NilError(t, err)
	templ := template.CreateTemplateFromString("deploy_test-"+uuid.String(), "{}")
	return templ
}

func generateFaultyTemplate(t *testing.T) template.Template {
	uuid, err := uuid.NewUUID()
	assert.NilError(t, err)
	templ := template.CreateTemplateFromString("deploy_test-"+uuid.String(), "{")
	return templ
}
