package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventE2E(t *testing.T) {
	t.Run("Create and Get Event", func(t *testing.T) {
		resetTable(context.Background())

		// Create Event
		now := time.Now()
		createReqBody := PostEventsJSONRequestBody{
			Name:                  "Test Event",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationTypes:     []RegistrationType{ByIndividual},
		}
		createBodyBytes, _ := json.Marshal(createReqBody)
		createResp, err := http.Post(testServer.URL+"/events", "application/json", bytes.NewReader(createBodyBytes))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, createResp.StatusCode)

		var createRespBody PostEvents200JSONResponse
		err = json.NewDecoder(createResp.Body).Decode(&createRespBody)
		assert.NoError(t, err)
		assert.NotNil(t, createRespBody.Id)

		// Get Event
		getResp, err := http.Get(testServer.URL + "/events/" + createRespBody.Id.String())
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var getRespBody GetEventsId200JSONResponse
		err = json.NewDecoder(getResp.Body).Decode(&getRespBody)
		assert.NoError(t, err)
		assert.Equal(t, createRespBody.Id, getRespBody.Event.Id)
		assert.Equal(t, createReqBody.Name, getRespBody.Event.Name)
	})

	t.Run("Get All Events", func(t *testing.T) {
		resetTable(context.Background())

		// Get All Events
		getResp, err := http.Get(testServer.URL + "/events")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var getRespBody GetEvents200JSONResponse
		err = json.NewDecoder(getResp.Body).Decode(&getRespBody)
		assert.NoError(t, err)
		assert.NotNil(t, getRespBody.Data)
	})
}
