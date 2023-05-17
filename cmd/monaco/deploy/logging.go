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

package deploy

import (
	"github.com/dynatrace/dynatrace-configuration-as-code/internal/log"
	"github.com/dynatrace/dynatrace-configuration-as-code/pkg/manifest"
	project "github.com/dynatrace/dynatrace-configuration-as-code/pkg/project/v2"
)

func logProjectsInfo(projects []project.Project) {
	log.Info("Projects to be deployed (%d):", len(projects))
	for _, p := range projects {
		log.Info("  - %s", p)
	}

	if log.DebugEnabled() {
		logConfigInfo(projects)
	}
}

func logConfigInfo(projects []project.Project) {
	cfgCount := 0
	for _, p := range projects {
		for _, cfgsPerTypePerEnv := range p.Configs {
			for _, cfgsPerType := range cfgsPerTypePerEnv {
				cfgCount += len(cfgsPerType)
			}
		}
	}
	log.Debug("Deploying %d configurations.", cfgCount)
}

func logEnvironmentsInfo(environments manifest.Environments) {
	log.Info("Environments to deploy to (%d):", len(environments))
	for _, name := range environments.Names() {
		log.Info("  - %s", name)
	}
}
func logDeploymentInfo(dryRun bool, envName string) {
	if dryRun {
		log.Info("Validating configurations for environment `%s`...", envName)
	} else {
		log.Info("Deploying configurations to environment `%s`...", envName)
	}
}

func getOperationNounForLogging(dryRun bool) string {
	if dryRun {
		return "Validation"
	}
	return "Deployment"
}