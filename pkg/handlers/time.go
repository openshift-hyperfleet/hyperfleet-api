/*
Copyright (c) 2018 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"net/http"
	"time"
)

// TimeHandler serves the current server time. It is a public, unauthenticated
// endpoint with no dependencies on the database or service layer.
type TimeHandler struct {
	// now is injectable for testing; defaults to time.Now.
	now func() time.Time
}

func NewTimeHandler() *TimeHandler {
	return &TimeHandler{now: time.Now}
}

type timeResponse struct {
	Time string `json:"time"`
}

// Get sends the current server time as an RFC 3339 (UTC) timestamp.
func (h TimeHandler) Get(w http.ResponseWriter, r *http.Request) {
	nowFn := h.now
	if nowFn == nil {
		nowFn = time.Now
	}

	body := timeResponse{
		Time: nowFn().UTC().Format(time.RFC3339Nano),
	}

	writeJSONResponse(w, r, http.StatusOK, body)
}
