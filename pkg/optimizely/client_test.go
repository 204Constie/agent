/****************************************************************************
 * Copyright 2019-2020,2023 Optimizely, Inc. and contributors               *
 *                                                                          *
 * Licensed under the Apache License, Version 2.0 (the "License");          *
 * you may not use this file except in compliance with the License.         *
 * You may obtain a copy of the License at                                  *
 *                                                                          *
 *    http://www.apache.org/licenses/LICENSE-2.0                            *
 *                                                                          *
 * Unless required by applicable law or agreed to in writing, software      *
 * distributed under the License is distributed on an "AS IS" BASIS,        *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. *
 * See the License for the specific language governing permissions and      *
 * limitations under the License.                                           *
 ***************************************************************************/

// Package optimizely //
package optimizely

import (
	"context"
	"fmt"
	"testing"

	"github.com/optimizely/go-sdk/pkg/client"
	"github.com/optimizely/go-sdk/pkg/decision"
	"github.com/optimizely/go-sdk/pkg/notification"
	"github.com/stretchr/testify/assert"

	"github.com/optimizely/agent/pkg/optimizely/optimizelytest"

	"github.com/optimizely/go-sdk/pkg/config"
	"github.com/optimizely/go-sdk/pkg/entities"
	"github.com/stretchr/testify/suite"
)

type ClientTestSuite struct {
	suite.Suite
	featureExp  entities.Experiment
	optlyClient *OptlyClient
	userContext entities.UserContext
	testClient  *optimizelytest.TestClient
}

func (suite *ClientTestSuite) SetupTest() {
	testClient := optimizelytest.NewClient()
	suite.testClient = testClient
	feature := entities.Feature{Key: "my_feat"}
	suite.testClient.ProjectConfig.AddMultiVariationFeatureTest(feature, "disabled_var", "enabled_var")
	suite.featureExp = suite.testClient.ProjectConfig.FeatureMap["my_feat"].FeatureExperiments[0]
	suite.optlyClient = &OptlyClient{
		OptimizelyClient: testClient.OptimizelyClient,
		ConfigManager:    &MockConfigManager{config: testClient.ProjectConfig},
		ForcedVariations: testClient.ForcedVariations}
	suite.userContext = entities.UserContext{
		ID:         "userId",
		Attributes: make(map[string]interface{}),
	}
}

func (suite *ClientTestSuite) TearDownTest() {
	suite.optlyClient.Close()
}

func (suite *ClientTestSuite) TestTrackEvent() {
	eventKey := "eventKey"
	suite.testClient.AddEvent(entities.Event{Key: eventKey})
	tags := map[string]interface{}{"tag": "value"}
	actual, err := suite.optlyClient.TrackEvent(context.Background(), eventKey, suite.userContext, tags)
	suite.NoError(err)

	expected := &Track{
		UserID:   "userId",
		EventKey: "eventKey",
	}

	suite.Equal(expected, actual)

	events := suite.testClient.GetProcessedEvents()
	suite.Equal(1, len(events))

	actualEvent := events[0]
	suite.Equal(eventKey, actualEvent.Conversion.Key)
	suite.Equal("userId", actualEvent.VisitorID)
	suite.Equal(tags, actualEvent.Conversion.Tags)
}

func (suite *ClientTestSuite) TestValidSetForcedVariations() {
	scenarios := []struct {
		experimentKey string
		variationKey  string
		previousKey   string
		messages      []string
	}{
		{
			experimentKey: suite.featureExp.Key,
			variationKey:  "enabled_var",
			previousKey:   "",
			messages:      nil,
		},
		{
			experimentKey: suite.featureExp.Key,
			variationKey:  "disabled_var",
			previousKey:   "enabled_var",
			messages:      []string{"updating previous override"},
		},
		{
			experimentKey: suite.featureExp.Key,
			variationKey:  "dne-var",
			previousKey:   "disabled_var",
			messages: []string{
				"variationKey not found in configuration",
				"updating previous override",
			},
		},
		{
			experimentKey: "dne-exp",
			variationKey:  "dne-var",
			previousKey:   "",
			messages:      []string{"experimentKey not found in configuration"},
		},
	}

	userID := "testUser"
	for _, scenario := range scenarios {
		actual, err := suite.optlyClient.SetForcedVariation(context.Background(), scenario.experimentKey, userID, scenario.variationKey)
		suite.NoError(err)

		expected := &Override{
			UserID:           userID,
			ExperimentKey:    scenario.experimentKey,
			VariationKey:     scenario.variationKey,
			PrevVariationKey: scenario.previousKey,
			Messages:         scenario.messages,
		}

		suite.Equal(expected, actual)

		key := decision.ExperimentOverrideKey{
			ExperimentKey: scenario.experimentKey,
			UserID:        "testUser",
		}

		actVar, _ := suite.testClient.ForcedVariations.GetVariation(key)
		suite.Equal(scenario.variationKey, actVar)
	}
}

func (suite *ClientTestSuite) TestRemoveForcedVariation() {
	scenarios := []struct {
		previousKey string
		messages    []string
	}{
		{
			previousKey: "enabled_var",
			messages:    []string{"removing previous override"},
		},
		{
			previousKey: "",
			messages:    []string{"no pre-existing override"},
		},
	}

	userID := "testUser"
	_, _ = suite.optlyClient.SetForcedVariation(context.Background(), suite.featureExp.Key, userID, "enabled_var")

	for _, scenario := range scenarios {
		actual, err := suite.optlyClient.RemoveForcedVariation(context.Background(), suite.featureExp.Key, userID)
		suite.NoError(err)

		expected := &Override{
			UserID:           userID,
			ExperimentKey:    suite.featureExp.Key,
			VariationKey:     "",
			PrevVariationKey: scenario.previousKey,
			Messages:         scenario.messages,
		}

		suite.Equal(expected, actual)
		isEnabled, _ := suite.optlyClient.IsFeatureEnabled("my_feat", suite.userContext)
		suite.False(isEnabled)
	}

}

func (suite *ClientTestSuite) TestActivateFeature() {
	var1 := entities.Variable{Key: "var1", DefaultValue: "val1"}
	var2 := entities.Variable{Key: "var2", DefaultValue: "val2"}
	advancedFeature := entities.Feature{
		Key: "advanced",
		VariableMap: map[string]entities.Variable{
			"var1": var1,
			"var2": var2,
		},
	}

	suite.testClient.AddFeatureTest(advancedFeature)
	feature := suite.testClient.OptimizelyClient.GetOptimizelyConfig().FeaturesMap["advanced"]

	expected := &Decision{
		UserID:     "testUser",
		FeatureKey: "advanced",
		Type:       "feature",
		Variables: map[string]interface{}{
			"var1": "val1",
			"var2": "val2",
		},
		Enabled:       true,
		ExperimentKey: "5",
		VariationKey:  "6",
	}

	// Response should be the same regardless of the flag
	for _, flag := range []bool{true, false} {
		actual, err := suite.optlyClient.ActivateFeature(context.Background(), feature.Key, entities.UserContext{ID: "testUser"}, flag)
		suite.NoError(err)
		suite.Equal(expected, actual)
	}

	// Only one event should have been triggered
	suite.Equal(1, len(suite.testClient.GetProcessedEvents()))
}

func (suite *ClientTestSuite) TestActivateExperiment() {
	testExperimentKey := "testExperiment1"
	testVariation := suite.testClient.ProjectConfig.CreateVariation("variationA")
	suite.testClient.AddExperiment(testExperimentKey, []entities.Variation{testVariation})
	experiment := suite.testClient.OptimizelyClient.GetOptimizelyConfig().ExperimentsMap["testExperiment1"]

	expected := &Decision{
		UserID:        "testUser",
		ExperimentKey: "testExperiment1",
		VariationKey:  "variationA",
		Type:          "experiment",
		Variables:     map[string]interface{}{},
		Enabled:       true,
	}

	// Response should be the same regardless of the flag
	for _, flag := range []bool{true, false} {
		actual, err := suite.optlyClient.ActivateExperiment(context.Background(), experiment.Key, entities.UserContext{ID: "testUser"}, flag)
		suite.NoError(err)
		suite.Equal(expected, actual)
	}

	// Only one event should have been triggered
	suite.Equal(1, len(suite.testClient.GetProcessedEvents()))
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}

type ErrorConfigManager struct {
	error string
}

func NewErrorConfigManager(message string) ErrorConfigManager {
	return ErrorConfigManager{error: message}
}

func (e ErrorConfigManager) RemoveOnProjectConfigUpdate(id int) error {
	panic("implement me")
}

func (e ErrorConfigManager) OnProjectConfigUpdate(callback func(notification.ProjectConfigUpdateNotification)) (int, error) {
	return 0, fmt.Errorf("config update error")
}

func (e ErrorConfigManager) GetConfig() (config.ProjectConfig, error) {
	return nil, fmt.Errorf("config error")
}

func (e ErrorConfigManager) GetOptimizelyConfig() *config.OptimizelyConfig {
	panic("implement me")
}

func (e ErrorConfigManager) SyncConfig() {
	panic("implement me")
}

type MockConfigManager struct {
	config config.ProjectConfig
	sdkKey string
}

func (m MockConfigManager) RemoveOnProjectConfigUpdate(int) error {
	panic("implement me")
}

func (m MockConfigManager) OnProjectConfigUpdate(callback func(notification.ProjectConfigUpdateNotification)) (int, error) {
	return 0, fmt.Errorf("method OnProjectConfigUpdate does not have any effect on MockConfigManager")
}

func (m MockConfigManager) GetConfig() (config.ProjectConfig, error) {
	return m.config, nil
}

func (m MockConfigManager) GetOptimizelyConfig() *config.OptimizelyConfig {
	panic("implement me")
}

func (m MockConfigManager) SyncConfig() {
	panic("implement me")
}

func TestTrackErrorConfigManager(t *testing.T) {
	testClient := optimizelytest.NewClient()
	optlyClient := &OptlyClient{
		OptimizelyClient: testClient.OptimizelyClient,
		ConfigManager:    NewErrorConfigManager("config error"),
		ForcedVariations: testClient.ForcedVariations,
	}

	uc := entities.UserContext{ID: "userId"}
	actual, err := optlyClient.TrackEvent(context.Background(), "something", uc, map[string]interface{}{})
	assert.EqualError(t, err, "config error")

	expected := &Track{}
	assert.Equal(t, expected, actual)
}

func TestTrackErrorClient(t *testing.T) {
	// Construct an OptimizelyClient with an erroring config manager
	factory := client.OptimizelyFactory{}
	oClient, _ := factory.Client(
		client.WithConfigManager(NewErrorConfigManager("track error")),
	)

	// Construct a valid config manager as part of the OptlyClient wrapper
	testConfig := optimizelytest.NewConfig()
	eventKey := "test-event"
	event := entities.Event{Key: eventKey}
	testConfig.AddEvent(event)

	optlyClient := &OptlyClient{
		OptimizelyClient: oClient,
		ConfigManager:    &MockConfigManager{config: testConfig},
		ForcedVariations: nil,
	}

	uc := entities.UserContext{ID: "userId"}
	actual, err := optlyClient.TrackEvent(context.Background(), "something", uc, map[string]interface{}{})
	assert.NoError(t, err)

	expected := &Track{
		UserID:   "userId",
		EventKey: "something",
		Error:    "Event with key something not found",
	}

	assert.Equal(t, expected, actual)
}
