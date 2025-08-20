package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrationE2E(t *testing.T) {
	t.Run("Register for Event", func(t *testing.T) {
		// Create Event
		now := time.Now()
		createReqBody := PostEventsJSONRequestBody{
			Name:                  "Test Event for Registration",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now.Add(time.Hour),
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

		// Register for Event
		reg := &Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		require.NoError(t, reg.FromIndividualRegistration(indivReg))

		regBodyBytes, _ := json.Marshal(reg)
		regResp, err := http.Post(testServer.URL+"/events/"+createRespBody.Id.String()+"/register", "application/json", bytes.NewReader(regBodyBytes))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, regResp.StatusCode)

		var regRespBody PostEventsEventIdRegister200JSONResponse
		err = json.NewDecoder(regResp.Body).Decode(&regRespBody)
		assert.NoError(t, err)
		assert.NotNil(t, regRespBody.Registration)

		// Get Registrations for Event
		getRegResp, err := http.Get(testServer.URL + "/events/" + createRespBody.Id.String() + "/registrations")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, getRegResp.StatusCode)

		var getRegRespBody GetEventsEventIdRegistrations200JSONResponse
		err = json.NewDecoder(getRegResp.Body).Decode(&getRegRespBody)
		assert.NoError(t, err)
		assert.Len(t, getRegRespBody.Data, 1)
	})
}
