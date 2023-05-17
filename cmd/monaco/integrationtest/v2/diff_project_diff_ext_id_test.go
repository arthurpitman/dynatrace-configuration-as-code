//go:build integration
// +build integration

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

package v2

import (
	"github.com/dynatrace/dynatrace-configuration-as-code/cmd/monaco/dynatrace"
	"github.com/dynatrace/dynatrace-configuration-as-code/cmd/monaco/integrationtest"
	"github.com/dynatrace/dynatrace-configuration-as-code/cmd/monaco/runner"
	"github.com/dynatrace/dynatrace-configuration-as-code/internal/idutils"
	"github.com/dynatrace/dynatrace-configuration-as-code/pkg/client/dtclient"
	"github.com/dynatrace/dynatrace-configuration-as-code/pkg/project/v2/topologysort"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"testing"
)

var diffProjectDiffExtIDFolder = "test-resources/integration-different-projects-different-extid/"
var diffProjectDiffExtIDFolderManifest = diffProjectDiffExtIDFolder + "manifest.yaml"

// TestSettingsInDifferentProjectsGetDifferentExternalIDs tries to upload a project that contatins two projects with
// the exact same settings 2.0 object and verifies that deploying such a monaco configuration results in
// two different settings objects deployed on the environment
func TestSettingsInDifferentProjectsGetDifferentExternalIDs(t *testing.T) {

	RunIntegrationWithCleanup(t, diffProjectDiffExtIDFolder, diffProjectDiffExtIDFolderManifest, "", "DifferentProjectsGetDifferentExternalID", func(fs afero.Fs, _ TestContext) {

		cmd := runner.BuildCli(fs)
		cmd.SetArgs([]string{"deploy", "--verbose", diffProjectDiffExtIDFolderManifest})
		err := cmd.Execute()

		assert.NoError(t, err)

		var manifestPath = diffProjectDiffExtIDFolderManifest
		loadedManifest := integrationtest.LoadManifest(t, fs, manifestPath, "")
		environment := loadedManifest.Environments["platform_env"]
		projects := integrationtest.LoadProjects(t, fs, manifestPath, loadedManifest)
		sortedConfigs, _ := topologysort.GetSortedConfigsForEnvironments(projects, []string{"platform_env"})

		extIDProject1, _ := idutils.GenerateExternalID(sortedConfigs["platform_env"][0].Coordinate)
		extIDProject2, _ := idutils.GenerateExternalID(sortedConfigs["platform_env"][1].Coordinate)

		c, _ := dynatrace.CreateDTClient(environment.URL.Value, environment.Auth, false)
		settings, _ := c.ListSettings("builtin:anomaly-detection.metric-events", dtclient.ListSettingsOptions{DiscardValue: true, Filter: func(object dtclient.DownloadSettingsObject) bool {
			return object.ExternalId == extIDProject1 || object.ExternalId == extIDProject2
		}})
		assert.Len(t, settings, 2)
	})
}