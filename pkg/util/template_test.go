//go:build unit
// +build unit

/**
 * @license
 * Copyright 2020 Dynatrace LLC
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

package util

import (
	"gotest.tools/assert"
	"testing"
)

const testMatrixTemplateWithEnvVar = "Follow the {{.color}} {{ .Env.ANIMAL }}"
const testMatrixTemplateWithProperty = "Follow the {{.color}} {{ .ANIMAL }}"

func TestGetStringWithEnvVar(t *testing.T) {

	template, err := NewTemplateFromString("template_test", testMatrixTemplateWithEnvVar)
	assert.NilError(t, err)

	SetEnv(t, "ANIMAL", "cow")
	result, err := template.ExecuteTemplate(getTemplateTestProperties())
	UnsetEnv(t, "ANIMAL")

	assert.NilError(t, err)
	assert.Equal(t, "Follow the white cow", result)
}

func TestGetStringWithEnvVarLeadsToErrorIfEnvVarNotPresent(t *testing.T) {

	template, err := NewTemplateFromString("template_test", testMatrixTemplateWithEnvVar)
	assert.NilError(t, err)

	UnsetEnv(t, "ANIMAL")
	_, err = template.ExecuteTemplate(getTemplateTestProperties())

	assert.ErrorContains(t, err, "map has no entry for key \"ANIMAL\"")
}

func TestGetStringLeadsToErrorIfPropertyNotPresent(t *testing.T) {

	template, err := NewTemplateFromString("template_test", testMatrixTemplateWithEnvVar)
	assert.NilError(t, err)

	SetEnv(t, "ANIMAL", "cow")
	_, err = template.ExecuteTemplate(make(map[string]string)) // empty map
	UnsetEnv(t, "ANIMAL")

	assert.ErrorContains(t, err, "map has no entry for key \"color\"")
}

func TestGetStringWithEnvVarAndProperty(t *testing.T) {

	template, err := NewTemplateFromString("template_test", testMatrixTemplateWithProperty)
	assert.NilError(t, err)

	SetEnv(t, "ANIMAL", "cow")
	result, err := template.ExecuteTemplate(getTemplateTestPropertiesClashingWithEnvVars())
	UnsetEnv(t, "ANIMAL")

	assert.NilError(t, err)
	assert.Equal(t, "Follow the white rabbit", result)
}

func TestGetStringWithEnvVarIncludingEqualSigns(t *testing.T) {

	template, err := NewTemplateFromString("template_test", testMatrixTemplateWithEnvVar)
	assert.NilError(t, err)

	SetEnv(t, "ANIMAL", "cow=rabbit=chicken")
	result, err := template.ExecuteTemplate(getTemplateTestProperties())
	UnsetEnv(t, "ANIMAL")

	assert.NilError(t, err)
	assert.Equal(t, "Follow the white cow=rabbit=chicken", result)
}

func getTemplateTestProperties() map[string]string {

	m := make(map[string]string)

	m["color"] = "white"
	m["animalType"] = "rabbit"

	return m
}

func getTemplateTestPropertiesClashingWithEnvVars() map[string]string {

	m := make(map[string]string)

	m["color"] = "white"
	m["ANIMAL"] = "rabbit"

	return m
}

func TestTemplatesWithSpecialCharactersProduceValidJson(t *testing.T) {
	tests := []struct {
		name           string
		templateString string
		properties     map[string]string
		want           string
	}{
		{
			"empty test should work",
			`{}`,
			map[string]string{},
			`{}`,
		},
		{
			"newlines are escaped",
			`{ "key": "{{ .value }}", "object": { "o_key": "{{ .object_value}}" } }`,
			map[string]string{
				"value":        "A string\nwith several lines\n\n - here's one\n\n - and another",
				"object_value": "and\r\none\r\nmore",
			},
			`{ "key": "A string\nwith several lines\n\n - here's one\n\n - and another", "object": { "o_key": "and\r\none\r\nmore" } }`,
		},
		{
			"strings with random double-quotes are escaped",
			`{ "key": "{{ .value }}" }`,
			map[string]string{
				"value": `A "String" - that contains random "quotes"`,
			},
			`{ "key": "A \"String\" - that contains random \"quotes\"" }`,
		},
		{
			"regular slashes are not escaped",
			`{ "userAgent": "{{ .value }}" }`,
			map[string]string{
				"value": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36",
			},
			`{ "userAgent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36" }`,
		},
		{
			"a v1 list definition does not get quotes escaped",
			`{ "list": [ {{ .entries }} ] }`,
			map[string]string{
				"entries": `"element a", "element b", "element c"`,
			},
			`{ "list": [ "element a", "element b", "element c" ] }`,
		},
		{
			"a v1 list definition can still contain newlines",
			`{ "list": [ {{ .entries }} ] }`,
			map[string]string{
				"entries": `"element a",
"element b",
"element c"`,
			},
			`{ "list": [ "element a",
"element b",
"element c" ] }`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template, err := NewTemplateFromString("template_test", tt.templateString)
			assert.NilError(t, err)

			result, err := template.ExecuteTemplate(tt.properties)
			assert.NilError(t, err)
			assert.Equal(t, result, tt.want)

			err = ValidateJson(result, Location{})
			assert.NilError(t, err)
		})
	}
}
