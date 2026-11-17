package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatesHandler_ComponentPosition(t *testing.T) {
	a := newPlanAPI(t)
	code, p := a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, a.lunch, a.chili))
	require.Equal(t, http.StatusCreated, code, p)
	url := fmt.Sprintf("/api/plates/%d/components", int64(p["id"].(float64)))

	comps := p["components"].([]any)
	assert.Equal(t, "main", comps[0].(map[string]any)["position"])

	code, got := a.do(http.MethodPost, url, fmt.Sprintf(`{"food_id":%d,"portions":1,"position":"main"}`, a.curry))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "error.plate.second_main", got["message_key"], got)

	code, _ = a.do(http.MethodPost, url, fmt.Sprintf(`{"food_id":%d,"portions":1,"position":"bogus"}`, a.curry))
	assert.Equal(t, http.StatusBadRequest, code)

	code, got = a.do(http.MethodPost, url, fmt.Sprintf(`{"food_id":%d,"portions":1}`, a.curry))
	require.Equal(t, http.StatusCreated, code, got)
	comps = got["components"].([]any)
	assert.Equal(t, "side", comps[len(comps)-1].(map[string]any)["position"], "derived and echoed")
}
