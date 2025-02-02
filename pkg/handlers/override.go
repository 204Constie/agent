/****************************************************************************
 * Copyright 2020, Optimizely, Inc. and contributors                        *
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

// Package handlers //
package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/render"

	"github.com/optimizely/agent/pkg/middleware"
)

// OverrideBody defines the request body for an override
type OverrideBody struct {
	UserID        string `json:"userId"`
	ExperimentKey string `json:"experimentKey"`
	VariationKey  string `json:"variationKey"`
}

// Override is used to set forced variations for a given experiment or feature test
func Override(w http.ResponseWriter, r *http.Request) {
	optlyClient, err := middleware.GetOptlyClient(r)
	logger := middleware.GetLogger(r)
	if err != nil {
		RenderError(err, http.StatusInternalServerError, w, r)
		return
	}

	var body OverrideBody
	if parseErr := ParseRequestBody(r, &body); parseErr != nil {
		RenderError(parseErr, http.StatusBadRequest, w, r)
		return
	}

	if body.UserID == "" {
		RenderError(errors.New("userId cannot be empty"), http.StatusBadRequest, w, r)
		return
	}

	experimentKey := body.ExperimentKey
	if experimentKey == "" {
		RenderError(errors.New("experimentKey cannot be empty"), http.StatusBadRequest, w, r)
		return
	}

	// Empty variation means remove
	if body.VariationKey == "" {
		if override, err := optlyClient.RemoveForcedVariation(r.Context(), experimentKey, body.UserID); err != nil {
			RenderError(err, http.StatusInternalServerError, w, r)
		} else {
			render.JSON(w, r, override)
		}
		return
	}

	logger.Debug().Str("experimentKey", experimentKey).Str("variationKey", body.VariationKey).Msg("setting override")
	if override, err := optlyClient.SetForcedVariation(r.Context(), experimentKey, body.UserID, body.VariationKey); err != nil {
		RenderError(err, http.StatusInternalServerError, w, r)
	} else {
		render.JSON(w, r, override)
	}
}
